// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const (
	// The shared applier image (GH-1368): agent-core plus helm and kubectl, no
	// baked chart. One repo serves every application's applier because the image
	// content is application-agnostic; the per-run tag is the tested commit.
	applierLiveImageRepository = "declarative-agents/applier"
	applierLiveCluster         = "da-chatbot-mesh-applier"
	applierLiveRelease         = "live"

	applierReadyWait = 3 * time.Minute

	applierLiveControlURL = "http://127.0.0.1:18091/api/lifecycle/health"
	applierLiveRolloutURL = "http://127.0.0.1:18090/provisioning/api/rollout"
	applierLiveApplyURL   = "http://127.0.0.1:18090/provisioning/api/apply"
)

// applierLiveRollbackHook is test-only chart instrumentation. A post-upgrade
// hook runs after Helm has waited for the ordinary resources and before the
// upgrade command returns. For the reserved fixture value it uses the real
// kubectl in the applier image to regress the chatbot Deployment. That makes
// the following declared kubectl rollout status fail deterministically, without
// racing an out-of-band patch against Helm's own --wait.
//
// The strategic patch also drops the chatbot's progressDeadlineSeconds so the
// unpullable image trips ProgressDeadlineExceeded in seconds rather than after
// kubectl rollout status --timeout 120s expires. Without it the verify would
// consume nearly the whole apply request budget (rest.yaml apply timeout 130s),
// leaving no headroom for helm_upgrade and helm_rollback, so the request machine
// was cancelled mid-verify and the endpoint answered 504 machine_timeout instead
// of the RolledBack -> 500 the scenario proves (a race lost on a slow node). The
// short deadline makes the RolledBack path fit the budget deterministically. The
// underlying production concern -- a real stalled apply's 120s verify racing the
// 130s budget -- is tracked separately from image consolidation.
//
// The extra Role is installed by the host-side initial install. The applier
// therefore already holds these test-only permissions when an in-cluster upgrade
// re-applies the chart; no production chart or production RBAC is widened.
const applierLiveRollbackHook = `{{- if .Values.applier.enabled }}
{{- $fullname := include "chatbot-mesh.fullname" . -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ $fullname }}-applier-live-rollback-trigger
rules:
  - apiGroups: [""]
    resources: [pods]
    verbs: [get, list, watch, create, delete]
  - apiGroups: [apps]
    resources: [deployments]
    verbs: [get, patch]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: {{ $fullname }}-applier-live-rollback-trigger
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ $fullname }}-applier-live-rollback-trigger
subjects:
  - kind: ServiceAccount
    name: {{ $fullname }}-applier
{{- if eq (int .Values.applier.params.nResults) 751 }}
---
apiVersion: v1
kind: Pod
metadata:
  name: {{ $fullname }}-applier-live-rollback-trigger
  annotations:
    helm.sh/hook: post-upgrade
    helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded
spec:
  serviceAccountName: {{ $fullname }}-applier
  restartPolicy: Never
  containers:
    - name: regress-chatbot
      image: "{{ .Values.applier.image.repository }}:{{ .Values.applier.image.tag }}"
      imagePullPolicy: {{ .Values.applier.image.pullPolicy }}
      command: [kubectl]
      args:
        - patch
        - deployment/{{ $fullname }}-chatbot
        - --type=strategic
        - -p
        - '{"spec":{"progressDeadlineSeconds":20,"template":{"spec":{"containers":[{"name":"chatbot","image":"invalid.local/applier-live-rollback:missing"}]}}}}'
{{- end }}
{{- end }}
`

// ApplierLive proves the applier against a real cluster, which the fake-CLI
// tracer cannot: integration:applier drives recording stand-ins that take their
// exit codes from the scenario, so it is evidence about the machine and the
// arguments it constructs, not about helm and kubectl behaving as the
// declarations assume (srd006 R5.3, GH-735).
//
// It is a separate target from integration:applier on purpose. That one runs
// anywhere in seconds; this one needs docker and kind, builds an image, and
// stands up a cluster. Keeping them apart means a cluster failure reads as a
// cluster failure rather than as a tracer failure.
func (Integration) ApplierLive() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(profilesRoot)
	if reason := applierLiveSkipReason(coreRoot); reason != "" {
		fmt.Printf("SKIP applierLive: %s\n", reason)
		return nil
	}
	return runApplierLive(coreRoot, profilesRoot)
}

// applierLiveSkipReason reports why the live tier cannot run, or "" when every
// dependency is present. A recorded skip keeps a checkout without docker or kind
// runnable; it is never silent.
func applierLiveSkipReason(coreRoot string) string {
	for _, bin := range []string{"docker", "kind", "kubectl", "helm"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Sprintf("%s not found on PATH", bin)
		}
	}
	if !agentCoreAvailable(coreRoot) {
		return fmt.Sprintf("agent-core checkout not found at %s (set core_root in demo.yaml)", coreRoot)
	}
	return ""
}

func runApplierLive(coreRoot, profilesRoot string) error {
	images, err := resolveChatbotIntegrationImages(profilesRoot)
	if err != nil {
		return err
	}
	fmt.Printf("applierLive: building runtime image %s from %s\n", images.Runtime, coreRoot)
	if err := buildSmokeRuntimeImage(coreRoot, images.Runtime); err != nil {
		return err
	}
	chartDir := applicationChartDir(profilesRoot)
	staged, cleanupChart, err := stageApplierLiveChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupChart()

	// The chart reaches the applier pod as a mounted volume, not baked into the
	// shared applier image (GH-1368): the staged chart is packaged to a tarball and
	// carried by a ConfigMap provisioned out-of-release (GH-1407), referenced by the
	// chart's applier.chartArchiveConfigMap value. One coherent instrumented chart is
	// both installed host-side and mounted at /chart, so a values change re-renders
	// the same chart.
	chartArchive, cleanupArchive, err := packageApplierChart(staged)
	if err != nil {
		return err
	}
	defer cleanupArchive()
	if err := assertApplierChartArchiveCarriesProfiles(chartArchive); err != nil {
		return err
	}

	// The shared applier image is FROM the agent-core runtime built above (GH-1368):
	// agent-core plus helm and kubectl, no baked chart.
	fmt.Printf("applierLive: building applier image %s on %s\n", images.Applier, images.Runtime)
	if err := buildApplierImage(coreRoot, images.Runtime, images.Applier); err != nil {
		return err
	}
	if err := assertApplierImageCarriesItsTools(images.Applier); err != nil {
		return err
	}

	// The applier tier stands up the full mesh (chatbot, rag, chroma, dolt,
	// collector, observer), so the kind node needs every external dependency
	// image. Preload them from the host the same way helmSmoke and demo do
	// (GH-1368): the host docker pulls (and caches) them, then they are imported
	// into the kind node, so the node never pulls from a registry itself. A node
	// that pulls directly fails behind a TLS-intercepting proxy it does not trust.
	// The applier's helm_upgrade runs --atomic --wait, so unlike helmSmoke it
	// gates on EVERY release pod reaching Ready. The shared smoke set includes
	// the observer proxy and utility init image as of GH-1321, so no release pod
	// can stall in ImagePullBackOff on a node that cannot reach the registry.
	dependencyImages, err := applierLiveDependencyImages(chartDir)
	if err != nil {
		return err
	}
	for _, image := range dependencyImages {
		fmt.Printf("applierLive: preloading dependency image %s\n", image)
		if out, pullErr := exec.Command("docker", "pull", "--platform", "linux/"+runtime.GOARCH, image).CombinedOutput(); pullErr != nil {
			return fmt.Errorf("pull applier dependency %s: %w: %s", image, pullErr, strings.TrimSpace(string(out)))
		}
	}

	cluster, err := kindrig.EnsureCluster(kindrig.DefaultRun, applierLiveCluster,
		helmKindConfig(applicationChartDir(profilesRoot)), helmClusterWait)
	if err != nil {
		return err
	}
	defer cluster.Release(kindrig.DefaultRun)

	if err := loadKindImage(applierLiveCluster, images.Applier); err != nil {
		return err
	}
	if err := loadKindImage(applierLiveCluster, images.Runtime); err != nil {
		return err
	}
	for _, image := range dependencyImages {
		if err := loadSmokeDependencyImage(applierLiveCluster, image); err != nil {
			return err
		}
	}

	if err := helmInstallApplierLive(staged, chartArchive, images.Runtime, images.Applier); err != nil {
		return err
	}
	if err := waitApplierDeploymentReady(); err != nil {
		return err
	}
	if err := assertApplierServesItsSurface(profilesRoot); err != nil {
		return err
	}
	fmt.Printf("integration:applierLive PASS - revision %s the applier runs on kind from an image built on the runtime "+
		"under test, reads a real Deployment's rollout, applies a values patch that moves the release to a new "+
		"revision, compensates a post-upgrade verification failure with a real Helm rollback, and rejects a "+
		"non-conforming patch against the real chart schema without touching it\n", images.Revision)
	return nil
}

// applierLiveDependencyImages is the full external image set for the applier
// tier. Its kind values disable Ollama, so this is the same collector, Chroma,
// Dolt, observer-proxy, and utility set used by the smoke topology.
func applierLiveDependencyImages(chartDir string) ([]string, error) {
	return smokeDependencyImages(chartDir)
}

// stageApplierLiveChart gives only this live tier a deterministic post-upgrade
// regression hook. Both the host-side install and /chart in the applier image
// use this same staged directory, so Helm records and rolls back one coherent
// instrumented chart.
func stageApplierLiveChart(chartDir, profilesRoot string) (string, func(), error) {
	staged, cleanup, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return "", nil, err
	}
	hook := filepath.Join(staged, "templates", "applier-live-rollback-trigger.yaml")
	if err := os.WriteFile(hook, []byte(applierLiveRollbackHook), 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage applier live rollback trigger: %w", err)
	}
	return staged, cleanup, nil
}

// applierLiveChartConfigMap names the ConfigMap that carries the packaged chart
// to the applier pod. It is provisioned out-of-release (see helmInstallApplierLive)
// and referenced by the chart's applier.chartArchiveConfigMap value.
const applierLiveChartConfigMap = applierLiveRelease + "-applier-chart"

// helmInstallApplierLive installs the mesh with the applier enabled. The two
// values files layer: the kind footprint every cluster test shares, then the
// applier the others deliberately disable. The chart the applier mounts at /chart
// is delivered as data (GH-1368): the packaged tarball is carried by a ConfigMap
// this tooling provisions OUT of the Helm release (kubectl apply) and the chart
// references by name via applier.chartArchiveConfigMap. Keeping it out of the
// release is what holds the release Secret under the 1 MiB apiserver limit -- the
// chatbot-mesh chart embeds the collector UI, so carrying the archive in-release
// (values plus a rendered binaryData ConfigMap) stored it twice and overflowed
// (GH-1407). This mirrors the GH-1402 curator-UI shard provisioning.
func helmInstallApplierLive(chartPath, chartArchive, runtimeImage, applierImage string) error {
	repo, tag := splitImageRef(runtimeImage)
	applierRepo, applierTag := splitImageRef(applierImage)
	if err := provisionApplierChartConfigMap(chartArchive); err != nil {
		return err
	}
	cmd := exec.Command("helm", "install", applierLiveRelease, chartPath,
		"--values", filepath.Join(chartPath, "ci", "kind-values.yaml"),
		"--values", filepath.Join(chartPath, "ci", "kind-applier-values.yaml"),
		"--set", "applier.chartArchiveConfigMap="+applierLiveChartConfigMap,
		"--set", "image.repository="+repo,
		"--set-string", "image.tag="+tag,
		"--set", "image.pullPolicy=Never",
		"--set", "applier.image.repository="+applierRepo,
		"--set-string", "applier.image.tag="+applierTag,
		"--set", "llm.externalURL=http://host.docker.internal:11434",
		"--timeout", helmInstallTimeout.String(),
	)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install %s: %w", applierLiveRelease, err)
	}
	return nil
}

// provisionApplierChartConfigMap creates the out-of-release ConfigMap carrying the
// packaged chart. kubectl stores the tarball under binaryData because it is not
// valid UTF-8, and the chart's stage-chart init container unpacks it at /chart.
//
// `kubectl create` is used rather than `kubectl apply`: apply records the object
// in a last-applied-configuration annotation, and the base64 chart tarball
// (~0.5 MiB) blows past the 256 KiB annotation limit. A pre-delete keeps it
// idempotent when the kind cluster is reused.
func provisionApplierChartConfigMap(chartArchive string) error {
	del := exec.Command("kubectl", "delete", "configmap", applierLiveChartConfigMap, "--ignore-not-found")
	del.Stdout, del.Stderr = os.Stderr, os.Stderr
	if err := del.Run(); err != nil {
		return fmt.Errorf("clear stale applier chart ConfigMap %s: %w", applierLiveChartConfigMap, err)
	}
	create := exec.Command("kubectl", "create", "configmap", applierLiveChartConfigMap,
		"--from-file=chart.tgz="+chartArchive)
	create.Stdout, create.Stderr = os.Stderr, os.Stderr
	if err := create.Run(); err != nil {
		return fmt.Errorf("create applier chart ConfigMap %s: %w", applierLiveChartConfigMap, err)
	}
	return nil
}

// waitApplierDeploymentReady waits for the applier alone. The install does not
// use --wait: the chatbot needs an LLM this tier does not require, so blocking on
// the whole mesh would make the applier's own readiness depend on something
// unrelated to it.
func waitApplierDeploymentReady() error {
	deployment := "deployment/" + applierLiveRelease + "-chatbot-mesh-applier"
	cmd := exec.Command("kubectl", "rollout", "status", deployment,
		"--timeout", applierReadyWait.String())
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("the applier Deployment never became ready: %w", err)
	}
	return nil
}

// buildApplierImage builds the shared applier image from the locally built
// agent-core runtime rather than published artifacts, so the live tier runs the
// code under test the way the smoke does (GH-1368). The image is agent-core plus
// helm and kubectl and bakes no chart; the chart reaches the running pod through
// the mounted out-of-release chart ConfigMap, so the build context carries nothing
// per-app. It builds from the shared agent-core/applier.Dockerfile with the
// agent-core tree as the context.
//
// TARGETARCH is passed explicitly. The Dockerfile defaults it to amd64, and a
// plain `docker build` on an arm64 host does not set it -- the result is an
// arm64 image carrying amd64 helm and kubectl, which crash the first time an
// exec word runs one. The kind nodes are the host's architecture, so the image
// has to be too.
func buildApplierImage(coreRoot, runtimeImage, image string) error {
	cmd := exec.Command("docker", "build",
		"-f", filepath.Join(coreRoot, "applier.Dockerfile"),
		"--build-arg", "RUNTIME_IMAGE="+runtimeImage,
		"--build-arg", "TARGETARCH="+runtime.GOARCH,
		"-t", image, coreRoot)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build %s: %w", image, err)
	}
	return nil
}

// packageApplierChart packages the staged chart directory into a gzipped tarball
// and returns its path. This is the chart the applier's `helm upgrade
// chatbot-mesh /chart` word installs, delivered to the pod as data through the
// out-of-release chart ConfigMap rather than baked into the image (GH-1368). It is
// the same instrumented chart the host installs, so an in-cluster upgrade
// re-renders one coherent chart.
func packageApplierChart(chartDir string) (string, func(), error) {
	dest, err := os.MkdirTemp("", "chatbot-mesh-applier-chart-tgz-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dest) }
	output, err := exec.Command("helm", "package", chartDir, "--destination", dest).CombinedOutput()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("helm package applier chart: %w: %s", err, strings.TrimSpace(string(output)))
	}
	archive := filepath.Join(dest, "chatbot-mesh-0.1.0.tgz")
	if _, err := os.Stat(archive); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("packaged applier chart %s: %w", archive, err)
	}
	return archive, cleanup, nil
}

// applierImageProbe is one thing the image must carry for the applier's
// declarations to work, and the command that proves it is there and runnable.
type applierImageProbe struct {
	what string
	args []string
	want string
}

// applierImageProbes are the assumptions the exec declarations make about their
// own container. Each runs the binary rather than testing for the file, because
// a wrong-architecture binary is present and unrunnable -- which is the failure
// an unqualified build produces.
func applierImageProbes() []applierImageProbe {
	return []applierImageProbe{
		{what: "helm", args: []string{"helm", "version", "--short"}, want: "v"},
		{what: "kubectl", args: []string{"kubectl", "version", "--client"}, want: "Client Version"},
		// The runtime the profile runs; the applier is an agent before it is a
		// pair of CLIs. The chart is not baked into the image (GH-1368); it reaches
		// the pod through the mounted out-of-release chart ConfigMap, verified
		// host-side by assertApplierChartArchiveCarriesProfiles.
		{what: "the agent binary", args: []string{"agent", "--help"}, want: "profile"},
	}
}

// assertApplierImageCarriesItsTools runs each probe inside the built image. An
// image missing any of them fails at runtime inside a pod, where the error names
// a tool that is not there rather than an image that was built wrong.
func assertApplierImageCarriesItsTools(image string) error {
	for _, probe := range applierImageProbes() {
		args := append([]string{"run", "--rm", "--entrypoint", probe.args[0], image}, probe.args[1:]...)
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("applier image does not carry %s: docker %s: %w\n%s",
				probe.what, strings.Join(probe.args, " "), err, out)
		}
		if !strings.Contains(string(out), probe.want) {
			return fmt.Errorf("applier image %s check did not report %q:\n%s", probe.what, probe.want, out)
		}
		fmt.Printf("applierLive: image carries %s\n", probe.what)
	}
	return assertApplierImageHelmMajor(image)
}

// assertApplierChartArchiveCarriesProfiles renders the packaged chart the applier
// will mount at /chart, using the host helm, and requires every agent profile an
// enabled Deployment mounts to appear in the profiles ConfigMap.
//
// This is what an apply actually does. The applier runs
// `helm upgrade chatbot-mesh /chart`, which re-renders the co-generated topology
// (srd006 R2.2), so the ConfigMap that render produces replaces the live one. If
// the mounted chart carried no profiles the render would be nearly empty, the
// replacement would strip every agent profile, and no agent would survive its
// next restart -- with the apply reporting success, because helm did exactly what
// it was asked (GH-748). The chart is delivered as data now (GH-1368), so this
// asserts the archive rather than the image.
func assertApplierChartArchiveCarriesProfiles(archive string) error {
	out, err := exec.Command("helm", "template", "chatbot-mesh", archive,
		"--set", "applier.enabled=true").CombinedOutput()
	if err != nil {
		return fmt.Errorf("render packaged applier chart: %w\n%s", err, out)
	}
	render := string(out)
	for _, agent := range []string{"chatbot", "rag-server", "provisioning-workflow-orchestrator", "creator", "applier"} {
		key := "agents__" + agent + "__profile.yaml"
		if !strings.Contains(render, key) {
			return fmt.Errorf("the chart the applier mounts at /chart renders no %s; an apply would replace the "+
				"live profiles ConfigMap with one missing it, and that agent would not come back from a restart", key)
		}
	}
	fmt.Println("applierLive: the chart the applier mounts at /chart renders every agent profile")
	return nil
}

// assertApplierImageHelmMajor proves the helm inside the image is the major the
// exec declarations are written for. GH-739 binds the declared flags to the
// pinned HELM_VERSION at the source; this checks the binary that actually ships,
// since a build arg override or a changed base could put a different one there.
func assertApplierImageHelmMajor(image string) error {
	out, err := exec.Command("docker", "run", "--rm", "--entrypoint", "helm", image,
		"version", "--template", "{{.Version}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("read helm version from %s: %w\n%s", image, err, out)
	}
	version := strings.TrimSpace(string(out))
	major := strings.TrimPrefix(strings.SplitN(version, ".", 2)[0], "v")
	if major != applierDeclaredHelmMajor {
		return fmt.Errorf("the applier image ships helm %s, but its exec declarations are written for helm %s; "+
			"the flag spellings differ between majors and helm rejects an unknown flag (GH-739)",
			version, applierDeclaredHelmMajor)
	}
	fmt.Printf("applierLive: image ships helm %s, matching the declared flags\n", version)
	return nil
}

// applierDeclaredHelmMajor is the helm major exec-declarations.yaml is written
// for. TestApplierHelmFlagsMatchTheShippedHelm holds it to the Dockerfile pin;
// this constant is what the running image is checked against.
const applierDeclaredHelmMajor = "3"

// assertApplierServesItsSurface proves the applier is an agent that started,
// not just a container that is running, and that its rollout read reaches a real
// Deployment.
//
// Readiness alone is the weaker claim: the probe hits the control server, which
// the runtime serves once the profile loads, so it already means more than a
// live process. What it cannot show is that the request machine dispatches --
// the rollout read runs two kubectl words against the cluster's own API, using
// the ServiceAccount the chart binds, so a working read is evidence about RBAC,
// the kubeconfig the pod gets, and the counts word's go-template all at once.
func assertApplierServesItsSurface(profilesRoot string) error {
	stop, err := kubectlPortForward("svc/"+applierLiveRelease+"-chatbot-mesh-applier", 18090, 18091)
	if err != nil {
		return err
	}
	defer stop()

	if err := waitHTTPStatus(applierLiveControlURL, http.StatusOK, applierReadyWait); err != nil {
		return fmt.Errorf("the applier control health never answered: %w", err)
	}
	fmt.Println("applierLive: the applier answers its control health")

	body, status, err := requestInference(http.MethodGet, applierLiveRolloutURL, "", "applier live rollout read")
	if err != nil {
		return fmt.Errorf("rollout read failed: %w", err)
	}
	if err := assertLiveRolloutBody(body, status); err != nil {
		return err
	}

	// The apply path, which is what the fake-CLI tracer cannot reach: a real
	// helm upgrade against a real release (GH-747).
	if err := runApplierLiveApplyStep(runApplierLiveCommand, "upgrade",
		func() error { return assertLiveApplyChangesTheRelease(profilesRoot) }); err != nil {
		return err
	}
	if err := runApplierLiveApplyStep(runApplierLiveCommand, "rollback",
		func() error { return assertLiveRollbackRestoresTheRelease(profilesRoot) }); err != nil {
		return err
	}
	if err := runApplierLiveApplyStep(runApplierLiveCommand, "schema rejection",
		func() error { return assertLiveSchemaRejection(profilesRoot) }); err != nil {
		return err
	}

	// After a real apply, the rollout read must still answer off the cluster.
	body, status, err = requestInference(http.MethodGet, applierLiveRolloutURL, "", "applier live rollout recheck")
	if err != nil {
		return fmt.Errorf("rollout read after apply failed: %w", err)
	}
	return assertLiveRolloutBody(body, status)
}

// assertLiveRolloutBody checks a rollout read against a real Deployment. The
// phase is deliberately not pinned: whether the chatbot has rolled out depends on
// an LLM this tier does not stand up, and both complete and progressing are
// honest answers about a real cluster. What must hold is that the counts are the
// Deployment's own -- a 502 would mean the applier could not reach it at all,
// and a zero desired would mean it read something that is not there (srd006
// R3.3, GH-686).
func assertLiveRolloutBody(body []byte, status int) error {
	if status != http.StatusOK {
		return fmt.Errorf("rollout read status = %d, want 200; the applier could not read the Deployment: %s",
			status, body)
	}
	var rollout struct {
		Phase    string `json:"phase"`
		Ready    int    `json:"ready"`
		Desired  int    `json:"desired"`
		Revision int    `json:"revision"`
	}
	if err := json.Unmarshal(body, &rollout); err != nil {
		return fmt.Errorf("decode rollout response: %w: %s", err, body)
	}
	if rollout.Phase != "complete" && rollout.Phase != "progressing" {
		return fmt.Errorf("rollout phase = %q, want complete or progressing: %s", rollout.Phase, body)
	}
	if rollout.Desired < 1 {
		return fmt.Errorf("rollout desired = %d; the counts did not come from a real Deployment: %s",
			rollout.Desired, body)
	}
	if rollout.Revision < 1 {
		return fmt.Errorf("rollout revision = %d; a deployed release has at least revision 1: %s",
			rollout.Revision, body)
	}
	fmt.Printf("applierLive: rollout read reports phase %s, %d/%d ready, revision %d from the live Deployment\n",
		rollout.Phase, rollout.Ready, rollout.Desired, rollout.Revision)
	return nil
}

// assertLiveApplyChangesTheRelease drives a real values patch through the apply
// endpoint and proves the release actually changed.
//
// A 200 is not evidence here: the fake-CLI tracer already returns one, and that
// is exactly what it cannot distinguish. What makes this different is the helm
// revision -- the applier's helm_upgrade ran in-cluster against the release,
// re-rendering the co-generated topology (srd006 R2.2), so a revision that did
// not move means nothing was applied whatever the response said.
func assertLiveApplyChangesTheRelease(profilesRoot string) error {
	before, err := helmReleaseRevision(applierLiveRelease)
	if err != nil {
		return err
	}
	fmt.Printf("applierLive: release at revision %d before the apply\n", before)

	patch, err := applierValuesPatch(profilesRoot, "conforming.yaml")
	if err != nil {
		return err
	}
	body, status, err := requestInference(http.MethodPost, applierLiveApplyURL, patch, "applier live apply")
	if err != nil {
		return fmt.Errorf("apply request failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("apply status = %d, want 200: %s", status, body)
	}
	if !strings.Contains(string(body), `"status":"applied"`) {
		return fmt.Errorf("apply did not report applied: %s", body)
	}

	after, err := helmReleaseRevision(applierLiveRelease)
	if err != nil {
		return err
	}
	if after <= before {
		return fmt.Errorf("the release is still at revision %d after an apply reported success; "+
			"helm_upgrade did not reach the release, so the 200 proved nothing", after)
	}
	fmt.Printf("applierLive: the apply moved the release from revision %d to %d\n", before, after)
	return nil
}

// assertLiveRollbackRestoresTheRelease proves the compensating action with real
// Helm and kubectl (srd006 R3.2, GH-751). The staged post-upgrade hook regresses
// the chatbot only after helm upgrade --wait has succeeded. The applier must
// therefore reach Verifying, observe kubectl's real timeout, run helm rollback,
// and map RolledBack to the distinct 500 response.
//
// Helm rollback creates a new release revision; it does not move the revision
// number backwards. Restoration is proved by comparing the computed release
// values and by waiting for the chatbot Deployment to become ready again.
func assertLiveRollbackRestoresTheRelease(profilesRoot string) error {
	beforeRevision, err := helmReleaseRevision(applierLiveRelease)
	if err != nil {
		return err
	}
	beforeValues, err := helmReleaseValues(applierLiveRelease)
	if err != nil {
		return err
	}
	patch, err := applierValuesPatch(profilesRoot, "rollback-trigger.yaml")
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: applierReadyWait}
	body, status, err := requestHTTPWithClient(client, http.MethodPost, applierLiveApplyURL, patch)
	if err != nil {
		return fmt.Errorf("rollback-triggering apply request failed: %w", err)
	}
	if status != http.StatusInternalServerError {
		return fmt.Errorf("rollback-triggering apply status = %d, want 500: %s", status, body)
	}
	for _, want := range []string{`"error":"rolled_back"`, `"status":"rolled_back"`} {
		if !strings.Contains(string(body), want) {
			return fmt.Errorf("rollback response does not contain %s: %s", want, body)
		}
	}

	afterRevision, err := helmReleaseRevision(applierLiveRelease)
	if err != nil {
		return err
	}
	if afterRevision < beforeRevision+2 {
		return fmt.Errorf("release revision moved from %d to %d, want an upgrade and a rollback revision",
			beforeRevision, afterRevision)
	}
	afterValues, err := helmReleaseValues(applierLiveRelease)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(afterValues, beforeValues) {
		beforeJSON, _ := json.Marshal(beforeValues)
		afterJSON, _ := json.Marshal(afterValues)
		return fmt.Errorf("helm rollback did not restore the prior computed values:\nbefore: %s\nafter:  %s",
			beforeJSON, afterJSON)
	}

	deployment := "deployment/" + applierLiveRelease + "-chatbot-mesh-chatbot"
	cmd := exec.Command("kubectl", "rollout", "status", deployment, "--timeout", applierReadyWait.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chatbot Deployment did not recover after rollback: %w\n%s", err, out)
	}
	fmt.Printf("applierLive: real helm rollback restored revision %d values in new revision %d and recovered the chatbot\n",
		beforeRevision, afterRevision)
	return nil
}

// assertLiveSchemaRejection closes the loop GH-732 opened with a local dry-run:
// the same non-conforming document, now against a real release on a cluster.
// The release must not move -- a rejected patch applies nothing (srd006 R2.1).
func assertLiveSchemaRejection(profilesRoot string) error {
	before, err := helmReleaseRevision(applierLiveRelease)
	if err != nil {
		return err
	}
	patch, err := applierValuesPatch(profilesRoot, "non-conforming.yaml")
	if err != nil {
		return err
	}
	body, status, err := requestInference(http.MethodPost, applierLiveApplyURL, patch, "applier live reject")
	if err != nil {
		return fmt.Errorf("apply request failed: %w", err)
	}
	if status != http.StatusBadRequest {
		return fmt.Errorf("a non-conforming patch returned %d, want 400: %s", status, body)
	}
	if !strings.Contains(string(body), "validate_rejected") {
		return fmt.Errorf("the rejection did not report validate_rejected: %s", body)
	}
	after, err := helmReleaseRevision(applierLiveRelease)
	if err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("the release moved from revision %d to %d on a rejected patch; "+
			"a schema rejection must apply nothing", before, after)
	}
	fmt.Printf("applierLive: the non-conforming patch was rejected and left the release at revision %d\n", after)
	return nil
}

// applierValuesPatch reads a shared fixture and wraps it as the apply request
// the creator would send (srd006 R1.4). The fixtures are GH-732's, so the local
// dry-run tier and this one validate the same documents.
func applierValuesPatch(profilesRoot, fixture string) (string, error) {
	path := filepath.Join(profilesRoot, "testdata", "integration", "applier-values", fixture)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read values fixture %s: %w", fixture, err)
	}
	request := map[string]string{"schema_version": "1", "content": string(content)}
	encoded, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

// helmReleaseRevision reads the release's current revision, which is what a real
// upgrade moves and a rejected patch leaves alone.
func helmReleaseRevision(release string) (int, error) {
	out, err := exec.Command("helm", "get", "metadata", release, "-o", "json").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("helm get metadata %s: %w\n%s", release, err, out)
	}
	var metadata struct {
		Revision int `json:"revision"`
	}
	if err := json.Unmarshal(out, &metadata); err != nil {
		return 0, fmt.Errorf("decode helm metadata: %w: %s", err, out)
	}
	return metadata.Revision, nil
}

// helmReleaseValues reads the fully computed values so a rollback is compared
// by released state, not by its ever-increasing numeric revision.
func helmReleaseValues(release string) (map[string]any, error) {
	out, err := exec.Command("helm", "get", "values", release, "--all", "-o", "json").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("helm get values %s: %w\n%s", release, err, out)
	}
	var values map[string]any
	if err := json.Unmarshal(out, &values); err != nil {
		return nil, fmt.Errorf("decode helm values: %w: %s", err, out)
	}
	return values, nil
}
