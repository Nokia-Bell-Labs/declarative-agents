// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"encoding/base64"
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
	// The live applier tier reuses the smoke release and namespace so the
	// runtime helpers (prepareCodingHelmCluster, verifyCodingHelmRollouts,
	// startCodingHelmForwards) apply unchanged; it stands its own dedicated
	// cluster apart from the smoke one so a cluster failure here reads as a
	// cluster failure, not a smoke regression.
	codingApplierLiveCluster = "da-coding-agent-applier"
	// The shared applier image (GH-1368): agent-core plus helm and kubectl, no
	// baked chart. One repo serves every application's applier because the image
	// content is application-agnostic; the per-run tag is the tested commit.
	codingApplierLiveImageRepo = "declarative-agents/applier"

	// The applier's exec declarations are written for helm 3, which the shared
	// agent-core/applier.Dockerfile pins (ARG HELM_VERSION). A helm-4 image would
	// reject the --atomic/--dry-run spellings outright.
	codingApplierDeclaredHelmMajor = "3"

	// The live apply legs run a real helm upgrade, a real 120s kubectl rollout
	// verify, and (on the rollback leg) a helm rollback, so the client waits
	// well past the fake-tracer's 130s bound.
	codingApplierLiveRequestTimeout = 4 * time.Minute
	codingApplierLiveReadyTimeout   = 3 * time.Minute
)

// ApplierLive proves the applier against a real cluster, which the fake-CLI
// tracer (integration:applier) cannot: that target drives recording stand-ins
// whose exit codes come from the scenario, so it is evidence about the machine
// and the arguments it builds, not about helm and kubectl behaving as the
// declarations assume (srd006 R5.3). This one builds the applier image, installs
// the packaged chart with the applier enabled, and drives a values patch through
// the running applier so a real helm upgrade moves the release revision, a
// verify stall triggers a real helm rollback, and a non-conforming patch is
// rejected against the real chart schema.
//
// It is a separate target from integration:applier on purpose. That one runs
// anywhere in seconds; this one needs docker and kind, builds images, and stands
// up a cluster.
func (Integration) ApplierLive() error {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	if reason := applierLiveSkipReason(roots, codingSmokeEnvironment{}.run); reason != "" {
		fmt.Printf("SKIP applierLive: %s\n", reason)
		return nil
	}
	return runCodingApplierLive(roots)
}

// applierLiveSkipReason reports why the live tier cannot run, or "" when every
// dependency is present. The live prerequisites are the smoke prerequisites --
// docker, kind, helm, kubectl, the agent-core checkout, a reachable Docker
// engine, and the local Go base image -- so it delegates to the smoke gate. A
// recorded skip keeps a checkout without docker or kind runnable; it is never
// silent.
func applierLiveSkipReason(roots integrationRoots, run codingSmokeRunner) string {
	return codingHelmSmokeSkipReason(roots, run)
}

func runCodingApplierLive(roots integrationRoots) (result error) {
	images, err := resolveCodingHelmImages(roots.Application)
	if err != nil {
		return err
	}
	applierImage, err := resolveCodingApplierImage(roots.Application)
	if err != nil {
		return err
	}

	// One instrumented chart directory serves both the host-side install and the
	// applier's mounted /chart, so Helm records and rolls back one coherent chart.
	// Package() must run first so the profile closure the chart mounts exists on
	// disk. The chart is delivered to the applier pod as a volume, not baked into
	// the image (GH-1368): it is packaged to a tarball, base64-encoded, and passed
	// as applier.chartArchive so the chart bytes travel with the Helm release.
	if err := Package(); err != nil {
		return &codingHelmSemanticError{Step: "profile package", Cause: err}
	}
	chartDir, cleanupChart, err := stageCodingApplierLiveChart(roots.Application)
	if err != nil {
		return err
	}
	defer cleanupChart()
	chartArchive, cleanupArchive, err := packageCodingApplierChart(chartDir)
	if err != nil {
		return &codingHelmSemanticError{Step: "applier chart package", Cause: err}
	}
	defer cleanupArchive()
	if err := assertCodingApplierChartArchiveCarriesProfiles(chartArchive); err != nil {
		return &codingHelmSemanticError{Step: "applier chart verification", Cause: err}
	}

	kindConfig := filepath.Join(roots.Application, "helm", "ci", "kind-config.yaml")
	kindRun := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), codingHelmClusterTimeout)
		defer cancel()
		return codingSmokeEnvironment{}.run(ctx, "kind", args...)
	}
	cluster, err := kindrig.EnsureClusterWithOptions(
		kindRun, codingApplierLiveCluster, kindConfig, 120*time.Second,
		codingApplierLiveEnsureOptions())
	if err != nil {
		return &codingHelmInfrastructureError{Step: "kind cluster acquisition", Cause: err}
	}
	kubeconfig, cleanupKubeconfig, err := codingKindKubeconfig(codingApplierLiveCluster)
	if err != nil {
		cluster.ReleaseAfter(kindRun, true, kindrig.FailureEvidence{
			Directory: codingHelmEvidenceDir(roots.Application, images.Revision),
		})
		return &codingHelmInfrastructureError{Step: "kind kubeconfig", Cause: err}
	}
	defer cleanupKubeconfig()
	environment := codingSmokeEnvironment{kubeconfig: kubeconfig}
	defer func() {
		cleanupCodingHelmSmoke(
			environment, cluster, kindRun, result != nil,
			codingHelmEvidenceDir(roots.Application, images.Revision))
	}()

	if err := checkCodingHelmInfrastructure(environment.run); err != nil {
		return err
	}
	// Reuse the smoke cluster preparation verbatim: it builds and loads the
	// runtime, model, and collector images, deploys the deterministic model, and
	// seeds the workspace PVC the serving roles mount.
	if err := prepareCodingHelmCluster(environment, cluster.Name, roots, images); err != nil {
		return classifyCodingHelmFailure(environment.run, "cluster preparation", err, true)
	}

	// The shared applier image is FROM agent-core (GH-1368): agent-core plus helm
	// and kubectl, no baked chart. prepareCodingHelmCluster already built and
	// loaded the agent-core image the collector runs on; the applier layers on it,
	// and the chart reaches the pod through the mounted applier.chartArchive.
	if err := buildCodingApplierImage(roots.Core, codingHelmCollectorImage, applierImage); err != nil {
		return &codingHelmSemanticError{Step: "applier image build", Cause: err}
	}
	if err := assertCodingApplierImageCarriesItsTools(applierImage); err != nil {
		return &codingHelmSemanticError{Step: "applier image verification", Cause: err}
	}
	if err := loadCodingDependencyImage(cluster.Name, applierImage); err != nil {
		return &codingHelmInfrastructureError{Step: "applier image load", Cause: err}
	}

	if err := installCodingApplierLiveChart(environment, chartDir, chartArchive, roots.Application, images.Agent, applierImage); err != nil {
		return classifyCodingHelmFailure(environment.run, "Helm install", err, true)
	}
	if err := verifyCodingHelmRollouts(environment, "applier"); err != nil {
		return classifyCodingHelmFailure(environment.run, "role readiness", err, true)
	}

	forwards, err := startCodingHelmForwards(environment, true)
	if err != nil {
		return classifyCodingHelmFailure(environment.run, "port-forward", err, true)
	}
	defer forwards.stop()

	if err := assertCodingApplierServesItsSurface(environment, roots.Application); err != nil {
		return err
	}
	fmt.Printf("integration:applierLive PASS - revision %s the applier runs on kind from an image built on the "+
		"runtime under test, reads a real Deployment's rollout, applies a values patch that moves the release to a "+
		"new revision, compensates a post-verify stall with a real helm rollback, and rejects a non-conforming patch "+
		"against the real chart schema without touching it\n", images.Revision)
	return nil
}

func codingApplierLiveEnsureOptions() kindrig.EnsureOptions {
	return kindrig.EnsureOptions{
		ReusePolicy: kindrig.RecreateUnhealthyOwnedCluster,
		HealthRun: func(name string, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(context.Background(), codingHelmProbeTimeout)
			defer cancel()
			return codingSmokeEnvironment{}.run(ctx, name, args...)
		},
	}
}

// resolveCodingApplierImage names the applier image by the tested checkout's
// commit, the same revision tag resolveCodingHelmImages uses, so the image built
// here is the one loaded and installed.
func resolveCodingApplierImage(applicationRoot string) (string, error) {
	repositoryRoot := filepath.Clean(filepath.Join(applicationRoot, "..", ".."))
	commit, err := gitOutput(repositoryRoot, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("resolve applier image revision: %w", err)
	}
	image, _, err := kindrig.CommitImage(codingApplierLiveImageRepo, commit)
	if err != nil {
		return "", err
	}
	return image, nil
}

// stageCodingApplierLiveChart assembles one chart directory carrying the
// manifest-derived closure and a test-only post-upgrade
// rollback trigger. The host installs this directory, and packageCodingApplierChart
// packages it to the tarball the applier mounts at /chart, so a values change
// re-renders one coherent instrumented chart (srd006 R2.2). It mirrors
// packageHelmChart's staging.
func stageCodingApplierLiveChart(applicationRoot string) (string, func(), error) {
	profiles := filepath.Join(applicationRoot, filepath.FromSlash(defaultProfileOutput))
	stage, err := os.MkdirTemp("", "coding-agent-applier-live-chart-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	chart := filepath.Join(stage, "coding-agent")
	if err := stageChartSource(filepath.Join(applicationRoot, "helm"), chart); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage source chart: %w", err)
	}
	if err := prepareHelmProfiles(profiles, chart); err != nil {
		cleanup()
		return "", nil, err
	}
	hook := filepath.Join(chart, "templates", "applier-live-rollback-trigger.yaml")
	if err := os.WriteFile(hook, []byte(codingApplierLiveRollbackHook), 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage applier live rollback trigger: %w", err)
	}
	return chart, cleanup, nil
}

// codingApplierLiveRollbackHook is test-only chart instrumentation. A
// post-upgrade hook runs as part of the upgrade, before the upgrade command
// returns; Helm waits for the hook Pod to complete even though the applier's own
// helm_upgrade no longer waits for the release resources. For the reserved
// sentinel value the hook uses the real kubectl in the applier image to regress
// the planner Deployment. That makes the applier's own kubectl rollout status
// verify fail deterministically, without racing an out-of-band patch.
//
// The strategic patch also drops progressDeadlineSeconds so the unpullable
// image trips ProgressDeadlineExceeded in seconds rather than consuming the
// 120s verify window. The 130s machine_request budget must retain headroom for
// helm_rollback and the RolledBack -> 500 response.
//
// The extra Role is installed by the host-side initial install, so the applier
// already holds these test-only permissions when an in-cluster upgrade re-applies
// the chart: helm creates the hook Pod under the applier ServiceAccount, and the
// applier's production Role grants pods only get/list/watch. No production chart
// or production RBAC is widened.
const codingApplierLiveRollbackHook = `{{- if .Values.applier.enabled }}
{{- $fullname := include "coding-agent.fullname" . -}}
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
{{- if eq (dig "executor" "resources" "requests" "memory" "" .Values.roles) "751Mi" }}
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
    - name: regress-planner
      image: "{{ .Values.applier.image.repository }}:{{ .Values.applier.image.tag }}"
      imagePullPolicy: {{ .Values.applier.image.pullPolicy }}
      command: [kubectl]
      args:
        - patch
        - deployment/{{ $fullname }}-planner
        - --namespace
        - {{ .Release.Namespace }}
        - --type=strategic
        - -p
        - '{"spec":{"progressDeadlineSeconds":5,"template":{"spec":{"containers":[{"name":"planner","image":"invalid.local/applier-live-rollback:missing"}]}}}}'
{{- end }}
{{- end }}
`

// buildCodingApplierImage builds the shared applier image from the locally built
// agent-core runtime rather than published artifacts, so the live tier runs the
// code under test the way the smoke does (GH-1368). The image is agent-core plus
// helm and kubectl and bakes no chart; the chart reaches the running pod through
// the mounted applier.chartArchive, so the build context carries nothing.
//
// TARGETARCH is passed explicitly. The Dockerfile defaults it to amd64, and a
// plain docker build on an arm64 host does not set it -- the result is an arm64
// image carrying amd64 helm and kubectl, which crash the first time an exec word
// runs one. The kind nodes are the host's architecture, so the image has to be
// too.
func buildCodingApplierImage(coreRoot, runtimeImage, image string) error {
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmClusterTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "docker", "build",
		"-f", filepath.Join(coreRoot, "applier.Dockerfile"),
		"--build-arg", "RUNTIME_IMAGE="+runtimeImage,
		"--build-arg", "TARGETARCH="+runtime.GOARCH,
		"-t", image, coreRoot)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("docker build %s: %w: %s", image, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// packageCodingApplierChart packages the staged chart directory into a gzipped
// tarball and returns its path. This is the chart the applier's `helm upgrade
// coding-agent /chart` word installs, delivered to the pod as data through the
// applier.chartArchive value rather than baked into the image (GH-1368). It is
// the same instrumented chart the host installs, so an in-cluster upgrade
// re-renders one coherent chart.
func packageCodingApplierChart(chartDir string) (string, func(), error) {
	dest, err := os.MkdirTemp("", "coding-applier-chart-tgz-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dest) }
	output, err := exec.Command("helm", "package", chartDir, "--destination", dest).CombinedOutput()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("helm package applier chart: %w: %s", err, strings.TrimSpace(string(output)))
	}
	archive := filepath.Join(dest, "coding-agent-0.1.0.tgz")
	if _, err := os.Stat(archive); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("packaged applier chart %s: %w", archive, err)
	}
	return archive, cleanup, nil
}

// codingApplierChartArchiveB64 base64-encodes the packaged chart into a temporary
// file so it can be passed to helm as `--set-file applier.chartArchive=<file>`;
// the chart template puts the value straight into the applier-chart ConfigMap's
// binaryData, which the kubelet decodes to the raw tarball the init container
// unpacks at /chart.
func codingApplierChartArchiveB64(archive string) (string, func(), error) {
	raw, err := os.ReadFile(archive)
	if err != nil {
		return "", nil, fmt.Errorf("read packaged applier chart: %w", err)
	}
	dir, err := os.MkdirTemp("", "coding-applier-chart-b64-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "chart.tgz.b64")
	if err := os.WriteFile(path, []byte(base64.StdEncoding.EncodeToString(raw)), 0o600); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write applier chart archive base64: %w", err)
	}
	return path, cleanup, nil
}

// assertCodingApplierChartArchiveCarriesProfiles renders the packaged chart the
// applier will mount at /chart, using the host helm, and requires every serving
// role's profile to appear. This is what an apply actually does: the applier runs
// helm upgrade coding-agent /chart, which re-renders the co-generated topology
// (srd006 R2.2), so the ConfigMap that render produces replaces the live one. If
// the mounted chart carried no profiles the render would be empty, the
// replacement would strip every profile, and no agent would survive its next
// restart.
func assertCodingApplierChartArchiveCarriesProfiles(archive string) error {
	out, err := exec.Command("helm", "template", "coding-agent", archive,
		"--set", "applier.enabled=true").CombinedOutput()
	if err != nil {
		return fmt.Errorf("render packaged applier chart: %w\n%s", err, out)
	}
	render := string(out)
	for _, role := range []string{"planner", "executor", "critic"} {
		key := role + "__profile.yaml"
		if !strings.Contains(render, key) {
			return fmt.Errorf("the packaged applier chart renders no %s; an apply would replace the live "+
				"profiles ConfigMap with one missing it, and that agent would not come back from a restart", key)
		}
	}
	fmt.Println("applierLive: the chart the applier mounts at /chart renders every serving-role profile")
	return nil
}

// assertCodingApplierImageCarriesItsTools runs each assumption the exec
// declarations make about their own container inside the built image, so a
// missing or wrong-architecture tool fails here with a name rather than at
// runtime inside a pod. Each probe runs the binary rather than testing for the
// file, because a wrong-architecture binary is present and unrunnable.
func assertCodingApplierImageCarriesItsTools(image string) error {
	probes := []struct {
		what string
		args []string
		want string
	}{
		{"helm", []string{"helm", "version", "--short"}, "v"},
		{"kubectl", []string{"kubectl", "version", "--client"}, "Client Version"},
		{"the agent binary", []string{"agent", "--help"}, "profile"},
	}
	for _, probe := range probes {
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
	return assertCodingApplierImageHelmMajor(image)
}

// assertCodingApplierImageHelmMajor proves the helm inside the image is the major
// the exec declarations are written for. A build-arg override or a changed base
// could ship a different one, and helm rejects an unknown flag outright.
func assertCodingApplierImageHelmMajor(image string) error {
	out, err := exec.Command("docker", "run", "--rm", "--entrypoint", "helm", image,
		"version", "--template", "{{.Version}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("read helm version from %s: %w\n%s", image, err, out)
	}
	version := strings.TrimSpace(string(out))
	major := strings.TrimPrefix(strings.SplitN(version, ".", 2)[0], "v")
	if major != codingApplierDeclaredHelmMajor {
		return fmt.Errorf("the applier image ships helm %s, but its exec declarations are written for helm %s; "+
			"the flag spellings differ between majors and helm rejects an unknown flag",
			version, codingApplierDeclaredHelmMajor)
	}
	fmt.Printf("applierLive: image ships helm %s, matching the declared flags\n", version)
	return nil
}

// installCodingApplierLiveChart installs the instrumented chart directory with the
// applier enabled. It layers the kind footprint every cluster test shares, then
// the applier the others deliberately disable, and pins the locally built and
// loaded images. The chart the applier mounts at /chart is delivered as data:
// applier.chartArchive carries the base64 tarball the init container unpacks
// (GH-1368), so the shared applier image bakes no chart.
func installCodingApplierLiveChart(
	environment codingSmokeEnvironment, chartDir, chartArchive, applicationRoot, runtimeImage, applierImage string,
) error {
	repository, tag := splitCodingImageRef(runtimeImage)
	collectorRepository, collectorTag := splitCodingImageRef(codingHelmCollectorImage)
	applierRepository, applierTag := splitCodingImageRef(applierImage)
	archiveB64, cleanupB64, err := codingApplierChartArchiveB64(chartArchive)
	if err != nil {
		return err
	}
	defer cleanupB64()
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmInstallTimeout)
	defer cancel()
	output, err := environment.run(ctx, "helm",
		"install", codingHelmRelease, chartDir,
		"--set-file", "applier.chartArchive="+archiveB64,
		"--namespace", codingHelmNamespace,
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-values.yaml"),
		"--values", filepath.Join(applicationRoot, "helm", "ci", "kind-applier-values.yaml"),
		"--set", "image.repository="+repository,
		"--set-string", "image.tag="+tag,
		"--set", "collector.image.repository="+collectorRepository,
		"--set-string", "collector.image.tag="+collectorTag,
		"--set", "applier.image.repository="+applierRepository,
		"--set-string", "applier.image.tag="+applierTag,
		"--wait", "--timeout", codingHelmInstallTimeout.String(),
	)
	if err != nil {
		return fmt.Errorf("helm install: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// assertCodingApplierServesItsSurface proves the applier is an agent that
// started, not just a container that is running, and then drives the apply path
// the fake-CLI tracer cannot reach: a real helm upgrade, a real rollback, and a
// real schema rejection against a real release.
func assertCodingApplierServesItsSurface(environment codingSmokeEnvironment, applicationRoot string) error {
	if err := waitServingHTTP(applierControlHealthURL, codingApplierLiveReadyTimeout); err != nil {
		return classifyCodingHelmFailure(environment.run, "applier control health",
			fmt.Errorf("the applier control health never answered: %w", err), true)
	}
	fmt.Println("applierLive: the applier answers its control health")

	if err := runApplierLiveApplyStep(environment.run, "rollout read", func() error {
		return assertCodingLiveRolloutReads()
	}); err != nil {
		return err
	}
	if err := runApplierLiveApplyStep(environment.run, "upgrade", func() error {
		return assertCodingLiveApplyChangesTheRelease(environment, applicationRoot)
	}); err != nil {
		return err
	}
	if err := runApplierLiveApplyStep(environment.run, "rollback", func() error {
		return assertCodingLiveRollbackRestoresTheRelease(environment, applicationRoot)
	}); err != nil {
		return err
	}
	if err := runApplierLiveApplyStep(environment.run, "schema rejection", func() error {
		return assertCodingLiveSchemaRejection(environment, applicationRoot)
	}); err != nil {
		return err
	}

	// After a real apply-and-rollback, the rollout read must still answer off the
	// cluster.
	if err := runApplierLiveApplyStep(environment.run, "rollout recheck", func() error {
		return assertCodingLiveRolloutReads()
	}); err != nil {
		return err
	}
	return nil
}

// assertCodingLiveRolloutReads checks a rollout read against a real Deployment.
// The phase is not pinned -- both complete and progressing are honest answers
// about a real cluster -- but a 502 would mean the applier could not reach the
// Deployment at all, and a zero desired or revision would mean it read something
// that is not there (srd006 R3.3).
func assertCodingLiveRolloutReads() error {
	body, status, err := codingApplierLiveGet(applierRolloutURL)
	if err != nil {
		return fmt.Errorf("rollout read failed: %w", err)
	}
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
	if err := json.Unmarshal([]byte(body), &rollout); err != nil {
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

// assertCodingLiveApplyChangesTheRelease drives a conforming values patch through
// the apply endpoint and proves the release actually changed. A 200 is not
// evidence here: the fake-CLI tracer already returns one. What makes this
// different is the helm revision -- the applier's helm_upgrade ran in-cluster
// against the release, so a revision that did not move means nothing was applied
// whatever the response said.
func assertCodingLiveApplyChangesTheRelease(environment codingSmokeEnvironment, applicationRoot string) error {
	before, err := codingHelmReleaseRevision(environment)
	if err != nil {
		return err
	}
	fmt.Printf("applierLive: release at revision %d before the apply\n", before)

	patch, err := codingApplierValuesPatch(applicationRoot, "conforming.yaml")
	if err != nil {
		return err
	}
	body, status, err := codingApplierLivePost(applierApplyURL, patch)
	if err != nil {
		return fmt.Errorf("apply request failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("apply status = %d, want 200: %s", status, body)
	}
	if !strings.Contains(body, `"status":"applied"`) {
		return fmt.Errorf("apply did not report applied: %s", body)
	}

	after, err := codingHelmReleaseRevision(environment)
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

// assertCodingLiveRollbackRestoresTheRelease proves the compensating action with
// real Helm and kubectl (srd006 R3.2). The applier's helm_upgrade applies and
// returns without waiting, and the staged post-upgrade hook regresses the planner
// as part of that upgrade, so the applier reaches its kubectl rollout status
// verify, observes the real 120s timeout, runs helm rollback, and maps RolledBack
// to the distinct 500 response. A waited --atomic upgrade would instead block on
// the never-ready planner until helm's own timeout and answer 504.
//
// Helm rollback creates a new release revision; it does not move the revision
// number backwards. Restoration is proved by comparing the computed release
// values and by waiting for the planner Deployment to become ready again.
func assertCodingLiveRollbackRestoresTheRelease(environment codingSmokeEnvironment, applicationRoot string) error {
	beforeRevision, err := codingHelmReleaseRevision(environment)
	if err != nil {
		return err
	}
	beforeValues, err := codingHelmReleaseValues(environment)
	if err != nil {
		return err
	}
	patch, err := codingApplierValuesPatch(applicationRoot, "rollback-trigger.yaml")
	if err != nil {
		return err
	}
	body, status, err := codingApplierLivePost(applierApplyURL, patch)
	if err != nil {
		return fmt.Errorf("rollback-triggering apply request failed: %w", err)
	}
	if status != http.StatusInternalServerError {
		return fmt.Errorf("rollback-triggering apply status = %d, want 500: %s", status, body)
	}
	for _, want := range []string{`"error":"rolled_back"`, `"status":"rolled_back"`} {
		if !strings.Contains(body, want) {
			return fmt.Errorf("rollback response does not contain %s: %s", want, body)
		}
	}

	afterRevision, err := codingHelmReleaseRevision(environment)
	if err != nil {
		return err
	}
	if afterRevision < beforeRevision+2 {
		return fmt.Errorf("release revision moved from %d to %d, want an upgrade and a rollback revision",
			beforeRevision, afterRevision)
	}
	afterValues, err := codingHelmReleaseValues(environment)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(afterValues, beforeValues) {
		beforeJSON, _ := json.Marshal(beforeValues)
		afterJSON, _ := json.Marshal(afterValues)
		return fmt.Errorf("helm rollback did not restore the prior computed values:\nbefore: %s\nafter:  %s",
			beforeJSON, afterJSON)
	}

	if err := runCodingSmokeCommand(environment, codingApplierLiveReadyTimeout,
		"kubectl", "rollout", "status",
		"deployment/"+codingHelmRelease+"-coding-agent-planner",
		"-n", codingHelmNamespace, "--timeout", codingApplierLiveReadyTimeout.String()); err != nil {
		return fmt.Errorf("planner Deployment did not recover after rollback: %w", err)
	}
	fmt.Printf("applierLive: real helm rollback restored revision %d values in new revision %d and recovered the planner\n",
		beforeRevision, afterRevision)
	return nil
}

// assertCodingLiveSchemaRejection closes the loop the local dry-run opened: the
// same non-conforming document, now against a real release on a cluster. The
// release must not move -- a rejected patch applies nothing (srd006 R2.1).
func assertCodingLiveSchemaRejection(environment codingSmokeEnvironment, applicationRoot string) error {
	before, err := codingHelmReleaseRevision(environment)
	if err != nil {
		return err
	}
	patch, err := codingApplierValuesPatch(applicationRoot, "non-conforming.yaml")
	if err != nil {
		return err
	}
	body, status, err := codingApplierLivePost(applierApplyURL, patch)
	if err != nil {
		return fmt.Errorf("apply request failed: %w", err)
	}
	if status != http.StatusBadRequest {
		return fmt.Errorf("a non-conforming patch returned %d, want 400: %s", status, body)
	}
	if !strings.Contains(body, "validate_rejected") {
		return fmt.Errorf("the rejection did not report validate_rejected: %s", body)
	}
	after, err := codingHelmReleaseRevision(environment)
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

// codingApplierValuesPatch reads a shared fixture and wraps it as the apply
// request the creator would send (srd006 R1.4). The fixtures are the ones the
// local dry-run tier validates, so both tiers exercise the same documents.
func codingApplierValuesPatch(applicationRoot, fixture string) (string, error) {
	path := filepath.Join(applicationRoot, "testdata", "integration", "applier-values", fixture)
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

// codingApplierLivePost drives the apply endpoint with a timeout that clears a
// real helm upgrade, a 120s kubectl rollout verify, and a helm rollback.
func codingApplierLivePost(url, body string) (string, int, error) {
	status, responseBody, err := applierHTTPWithTimeout(http.MethodPost, url, body, codingApplierLiveRequestTimeout)
	return responseBody, status, err
}

func codingApplierLiveGet(url string) (string, int, error) {
	status, responseBody, err := applierHTTPWithTimeout(http.MethodGet, url, "", codingApplierLiveRequestTimeout)
	return responseBody, status, err
}

// codingHelmReleaseRevision reads the release's current revision, which is what a
// real upgrade moves and a rejected patch leaves alone.
func codingHelmReleaseRevision(environment codingSmokeEnvironment) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmProbeTimeout)
	defer cancel()
	out, err := environment.run(ctx, "helm", "get", "metadata", codingHelmRelease,
		"-n", codingHelmNamespace, "-o", "json")
	if err != nil {
		return 0, fmt.Errorf("helm get metadata %s: %w\n%s", codingHelmRelease, err, out)
	}
	var metadata struct {
		Revision int `json:"revision"`
	}
	if err := json.Unmarshal(out, &metadata); err != nil {
		return 0, fmt.Errorf("decode helm metadata: %w: %s", err, out)
	}
	return metadata.Revision, nil
}

// codingHelmReleaseValues reads the fully computed values so a rollback is
// compared by released state, not by its ever-increasing numeric revision.
func codingHelmReleaseValues(environment codingSmokeEnvironment) (map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmProbeTimeout)
	defer cancel()
	out, err := environment.run(ctx, "helm", "get", "values", codingHelmRelease,
		"-n", codingHelmNamespace, "--all", "-o", "json")
	if err != nil {
		return nil, fmt.Errorf("helm get values %s: %w\n%s", codingHelmRelease, err, out)
	}
	var values map[string]any
	if err := json.Unmarshal(out, &values); err != nil {
		return nil, fmt.Errorf("decode helm values: %w: %s", err, out)
	}
	return values, nil
}
