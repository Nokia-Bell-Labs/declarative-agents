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
	executorLiveImage   = "declarative-agents/chatbot-mesh-executor:live"
	executorLiveCluster = "da-chatbot-mesh-executor"
	executorLiveRelease = "live"

	executorReadyWait = 3 * time.Minute

	executorLiveControlURL = "http://127.0.0.1:18091/api/lifecycle/health"
	executorLiveRolloutURL = "http://127.0.0.1:18090/provisioning/api/rollout"
	executorLiveApplyURL   = "http://127.0.0.1:18090/provisioning/api/apply"
)

// executorLiveRollbackHook is test-only chart instrumentation. A post-upgrade
// hook runs after Helm has waited for the ordinary resources and before the
// upgrade command returns. For the reserved fixture value it uses the real
// kubectl in the executor image to regress the chatbot Deployment. That makes
// the following declared kubectl rollout status fail deterministically, without
// racing an out-of-band patch against Helm's own --wait.
//
// The extra Role is installed by the host-side initial install. The executor
// therefore already holds these test-only permissions when an in-cluster upgrade
// re-applies the chart; no production chart or production RBAC is widened.
const executorLiveRollbackHook = `{{- if .Values.executor.enabled }}
{{- $fullname := include "chatbot-mesh.fullname" . -}}
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: {{ $fullname }}-executor-live-rollback-trigger
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
  name: {{ $fullname }}-executor-live-rollback-trigger
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: {{ $fullname }}-executor-live-rollback-trigger
subjects:
  - kind: ServiceAccount
    name: {{ $fullname }}-executor
{{- if eq (int .Values.executor.params.nResults) 751 }}
---
apiVersion: v1
kind: Pod
metadata:
  name: {{ $fullname }}-executor-live-rollback-trigger
  annotations:
    helm.sh/hook: post-upgrade
    helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded
spec:
  serviceAccountName: {{ $fullname }}-executor
  restartPolicy: Never
  containers:
    - name: regress-chatbot
      image: "{{ .Values.executor.image.repository }}:{{ .Values.executor.image.tag }}"
      imagePullPolicy: {{ .Values.executor.image.pullPolicy }}
      command: [kubectl]
      args:
        - patch
        - deployment/{{ $fullname }}-chatbot
        - --type=strategic
        - -p
        - '{"spec":{"template":{"spec":{"containers":[{"name":"chatbot","image":"invalid.local/executor-live-rollback:missing"}]}}}}'
{{- end }}
{{- end }}
`

// ExecutorLive proves the executor against a real cluster, which the fake-CLI
// tracer cannot: integration:executor drives recording stand-ins that take their
// exit codes from the scenario, so it is evidence about the machine and the
// arguments it constructs, not about helm and kubectl behaving as the
// declarations assume (srd006 R5.3, GH-735).
//
// It is a separate target from integration:executor on purpose. That one runs
// anywhere in seconds; this one needs docker and kind, builds an image, and
// stands up a cluster. Keeping them apart means a cluster failure reads as a
// cluster failure rather than as a tracer failure.
func (Integration) ExecutorLive() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(profilesRoot, "agent-core"))
	if reason := executorLiveSkipReason(coreRoot); reason != "" {
		fmt.Printf("SKIP executorLive: %s\n", reason)
		return nil
	}
	return runExecutorLive(coreRoot, profilesRoot)
}

// executorLiveSkipReason reports why the live tier cannot run, or "" when every
// dependency is present. A recorded skip keeps a checkout without docker or kind
// runnable; it is never silent.
func executorLiveSkipReason(coreRoot string) string {
	for _, bin := range []string{"docker", "kind", "kubectl", "helm"} {
		if _, err := exec.LookPath(bin); err != nil {
			return fmt.Sprintf("%s not found on PATH", bin)
		}
	}
	if !agentCoreAvailable(coreRoot) {
		return fmt.Sprintf("agent-core checkout not found at %s (set %s)", coreRoot, agentCoreRootEnv)
	}
	return ""
}

func runExecutorLive(coreRoot, profilesRoot string) error {
	fmt.Printf("executorLive: building runtime image %s from %s\n", helmImage, coreRoot)
	if err := buildSmokeRuntimeImage(coreRoot, helmImage); err != nil {
		return err
	}
	chartDir := exampleChartDir(profilesRoot)
	staged, cleanupChart, err := stageExecutorLiveChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupChart()

	fmt.Printf("executorLive: building executor image %s on %s\n", executorLiveImage, helmImage)
	if err := buildExecutorImage(profilesRoot, staged, helmImage, executorLiveImage); err != nil {
		return err
	}
	if err := assertExecutorImageCarriesItsTools(executorLiveImage); err != nil {
		return err
	}

	cluster, err := kindrig.EnsureCluster(kindrig.DefaultRun, executorLiveCluster,
		helmKindConfig(exampleChartDir(profilesRoot)), helmClusterWait)
	if err != nil {
		return err
	}
	defer cluster.Release(kindrig.DefaultRun)

	if err := kindrig.LoadImage(executorLiveCluster, executorLiveImage); err != nil {
		return err
	}
	if err := kindrig.LoadImage(executorLiveCluster, helmImage); err != nil {
		return err
	}

	if err := helmInstallExecutorLive(staged); err != nil {
		return err
	}
	if err := waitExecutorDeploymentReady(); err != nil {
		return err
	}
	if err := assertExecutorServesItsSurface(profilesRoot); err != nil {
		return err
	}
	fmt.Println("integration:executorLive PASS - the executor runs on kind from an image built on the runtime " +
		"under test, reads a real Deployment's rollout, applies a values patch that moves the release to a new " +
		"revision, compensates a post-upgrade verification failure with a real Helm rollback, and rejects a " +
		"non-conforming patch against the real chart schema without touching it")
	return nil
}

// stageExecutorLiveChart gives only this live tier a deterministic post-upgrade
// regression hook. Both the host-side install and /chart in the executor image
// use this same staged directory, so Helm records and rolls back one coherent
// instrumented chart.
func stageExecutorLiveChart(chartDir, profilesRoot string) (string, func(), error) {
	staged, cleanup, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return "", nil, err
	}
	hook := filepath.Join(staged, "templates", "executor-live-rollback-trigger.yaml")
	if err := os.WriteFile(hook, []byte(executorLiveRollbackHook), 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage executor live rollback trigger: %w", err)
	}
	return staged, cleanup, nil
}

// helmInstallExecutorLive installs the mesh with the executor enabled. The two
// values files layer: the kind footprint every cluster test shares, then the
// executor the others deliberately disable.
func helmInstallExecutorLive(chartPath string) error {
	repo, tag := splitImageRef(helmImage)
	execRepo, execTag := splitImageRef(executorLiveImage)
	cmd := exec.Command("helm", "install", executorLiveRelease, chartPath,
		"--values", filepath.Join(chartPath, "ci", "kind-values.yaml"),
		"--values", filepath.Join(chartPath, "ci", "kind-executor-values.yaml"),
		"--set", "image.repository="+repo,
		"--set", "image.tag="+tag,
		"--set", "image.pullPolicy=Never",
		"--set", "executor.image.repository="+execRepo,
		"--set", "executor.image.tag="+execTag,
		"--set", "llm.externalURL=http://host.docker.internal:11434",
		"--timeout", helmInstallTimeout.String(),
	)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("helm install %s: %w", executorLiveRelease, err)
	}
	return nil
}

// waitExecutorDeploymentReady waits for the executor alone. The install does not
// use --wait: the chatbot needs an LLM this tier does not require, so blocking on
// the whole mesh would make the executor's own readiness depend on something
// unrelated to it.
func waitExecutorDeploymentReady() error {
	deployment := "deployment/" + executorLiveRelease + "-chatbot-mesh-executor"
	cmd := exec.Command("kubectl", "rollout", "status", deployment,
		"--timeout", executorReadyWait.String())
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("the executor Deployment never became ready: %w", err)
	}
	return nil
}

// buildExecutorImage builds the executor image from the locally built runtime
// image and the supplied staged chart rather than published artifacts, so the
// live tier runs the code under test the way the smoke does.
//
// The build context carries the *staged* chart, not the one in the repo. The
// Dockerfile's `COPY helm /chart` takes whatever the context has, and the chart
// in the repo has no agent profiles -- the packaging step stages them
// (chartProfilePrograms). An image built from the unstaged chart renders a
// profiles ConfigMap with only the four co-generated keys, so an in-cluster
// upgrade would replace the live ConfigMap with a nearly empty one and no agent
// would survive its next restart (GH-748). Re-rendering the whole chart is the
// contract -- a values change re-renders the co-generated topology (srd006 R2.2,
// srd003 R2) -- which is exactly why what /chart contains matters.
//
// TARGETARCH is passed explicitly. The Dockerfile defaults it to amd64, and a
// plain `docker build` on an arm64 host does not set it -- the result is an
// arm64 image carrying amd64 helm and kubectl, which crash the first time an
// exec word runs one. The kind nodes are the host's architecture, so the image
// has to be too.
func buildExecutorImage(profilesRoot, staged, runtimeImage, image string) error {
	buildCtx, err := os.MkdirTemp("", "executor-image-ctx-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(buildCtx) }()
	if err := copyDirContents(staged, filepath.Join(buildCtx, "helm")); err != nil {
		return fmt.Errorf("place the staged chart in the build context: %w", err)
	}

	cmd := exec.Command("docker", "build",
		"-f", filepath.Join(profilesRoot, "executor.Dockerfile"),
		"--build-arg", "RUNTIME_IMAGE="+runtimeImage,
		"--build-arg", "TARGETARCH="+runtime.GOARCH,
		"-t", image, buildCtx)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build %s: %w", image, err)
	}
	return nil
}

// executorImageProbe is one thing the image must carry for the executor's
// declarations to work, and the command that proves it is there and runnable.
type executorImageProbe struct {
	what string
	args []string
	want string
}

// executorImageProbes are the assumptions the exec declarations make about their
// own container. Each runs the binary rather than testing for the file, because
// a wrong-architecture binary is present and unrunnable -- which is the failure
// an unqualified build produces.
func executorImageProbes() []executorImageProbe {
	return []executorImageProbe{
		{what: "helm", args: []string{"helm", "version", "--short"}, want: "v"},
		{what: "kubectl", args: []string{"kubectl", "version", "--client"}, want: "Client Version"},
		// The chart the helm words install, at the path their args name.
		{what: "the chart at /chart", args: []string{"cat", "/chart/Chart.yaml"}, want: "chatbot-mesh"},
		// The runtime the profile runs; the executor is an agent before it is a
		// pair of CLIs.
		{what: "the agent binary", args: []string{"agent", "--help"}, want: "profile"},
	}
}

// assertExecutorImageCarriesItsTools runs each probe inside the built image. An
// image missing any of them fails at runtime inside a pod, where the error names
// a tool that is not there rather than an image that was built wrong.
func assertExecutorImageCarriesItsTools(image string) error {
	for _, probe := range executorImageProbes() {
		args := append([]string{"run", "--rm", "--entrypoint", probe.args[0], image}, probe.args[1:]...)
		out, err := exec.Command("docker", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("executor image does not carry %s: docker %s: %w\n%s",
				probe.what, strings.Join(probe.args, " "), err, out)
		}
		if !strings.Contains(string(out), probe.want) {
			return fmt.Errorf("executor image %s check did not report %q:\n%s", probe.what, probe.want, out)
		}
		fmt.Printf("executorLive: image carries %s\n", probe.what)
	}
	if err := assertExecutorImageHelmMajor(image); err != nil {
		return err
	}
	return assertExecutorImageChartCarriesProfiles(image)
}

// assertExecutorImageChartCarriesProfiles renders the chart the image ships,
// using the image's own helm, and requires every agent profile an enabled
// Deployment mounts to appear in the profiles ConfigMap.
//
// This is what an apply actually does. The executor runs
// `helm upgrade chatbot-mesh /chart`, which re-renders the co-generated topology
// (srd006 R2.2), so the ConfigMap that render produces replaces the live one. If
// /chart carries an unstaged chart the render is nearly empty, the replacement
// strips every agent profile, and no agent survives its next restart -- with the
// apply reporting success, because helm did exactly what it was asked (GH-748).
func assertExecutorImageChartCarriesProfiles(image string) error {
	out, err := exec.Command("docker", "run", "--rm", "--entrypoint", "helm", image,
		"template", "chatbot-mesh", "/chart").CombinedOutput()
	if err != nil {
		return fmt.Errorf("render /chart with the image's own helm: %w\n%s", err, out)
	}
	render := string(out)
	for _, agent := range []string{"chatbot", "rag-server", "coordinator", "creator", "executor"} {
		key := "agents__" + agent + "__profile.yaml"
		if !strings.Contains(render, key) {
			return fmt.Errorf("the chart at /chart renders no %s; an apply would replace the live profiles "+
				"ConfigMap with one missing it, and that agent would not come back from a restart", key)
		}
	}
	fmt.Println("executorLive: the chart at /chart renders every agent profile")
	return nil
}

// assertExecutorImageHelmMajor proves the helm inside the image is the major the
// exec declarations are written for. GH-739 binds the declared flags to the
// pinned HELM_VERSION at the source; this checks the binary that actually ships,
// since a build arg override or a changed base could put a different one there.
func assertExecutorImageHelmMajor(image string) error {
	out, err := exec.Command("docker", "run", "--rm", "--entrypoint", "helm", image,
		"version", "--template", "{{.Version}}").CombinedOutput()
	if err != nil {
		return fmt.Errorf("read helm version from %s: %w\n%s", image, err, out)
	}
	version := strings.TrimSpace(string(out))
	major := strings.TrimPrefix(strings.SplitN(version, ".", 2)[0], "v")
	if major != executorDeclaredHelmMajor {
		return fmt.Errorf("the executor image ships helm %s, but its exec declarations are written for helm %s; "+
			"the flag spellings differ between majors and helm rejects an unknown flag (GH-739)",
			version, executorDeclaredHelmMajor)
	}
	fmt.Printf("executorLive: image ships helm %s, matching the declared flags\n", version)
	return nil
}

// executorDeclaredHelmMajor is the helm major exec-declarations.yaml is written
// for. TestExecutorHelmFlagsMatchTheShippedHelm holds it to the Dockerfile pin;
// this constant is what the running image is checked against.
const executorDeclaredHelmMajor = "3"

// assertExecutorServesItsSurface proves the executor is an agent that started,
// not just a container that is running, and that its rollout read reaches a real
// Deployment.
//
// Readiness alone is the weaker claim: the probe hits the control server, which
// the runtime serves once the profile loads, so it already means more than a
// live process. What it cannot show is that the request machine dispatches --
// the rollout read runs two kubectl words against the cluster's own API, using
// the ServiceAccount the chart binds, so a working read is evidence about RBAC,
// the kubeconfig the pod gets, and the counts word's go-template all at once.
func assertExecutorServesItsSurface(profilesRoot string) error {
	stop, err := kubectlPortForward("svc/"+executorLiveRelease+"-chatbot-mesh-executor", 18090, 18091)
	if err != nil {
		return err
	}
	defer stop()

	if err := waitHTTPStatus(executorLiveControlURL, http.StatusOK, executorReadyWait); err != nil {
		return fmt.Errorf("the executor control health never answered: %w", err)
	}
	fmt.Println("executorLive: the executor answers its control health")

	body, status, err := requestInference(http.MethodGet, executorLiveRolloutURL, "", "executor live rollout read")
	if err != nil {
		return fmt.Errorf("rollout read failed: %w", err)
	}
	if err := assertLiveRolloutBody(body, status); err != nil {
		return err
	}

	// The apply path, which is what the fake-CLI tracer cannot reach: a real
	// helm upgrade against a real release (GH-747).
	if err := assertLiveApplyChangesTheRelease(profilesRoot); err != nil {
		return err
	}
	if err := assertLiveRollbackRestoresTheRelease(profilesRoot); err != nil {
		return err
	}
	if err := assertLiveSchemaRejection(profilesRoot); err != nil {
		return err
	}

	// After a real apply, the rollout read must still answer off the cluster.
	body, status, err = requestInference(http.MethodGet, executorLiveRolloutURL, "", "executor live rollout recheck")
	if err != nil {
		return fmt.Errorf("rollout read after apply failed: %w", err)
	}
	return assertLiveRolloutBody(body, status)
}

// assertLiveRolloutBody checks a rollout read against a real Deployment. The
// phase is deliberately not pinned: whether the chatbot has rolled out depends on
// an LLM this tier does not stand up, and both complete and progressing are
// honest answers about a real cluster. What must hold is that the counts are the
// Deployment's own -- a 502 would mean the executor could not reach it at all,
// and a zero desired would mean it read something that is not there (srd006
// R3.3, GH-686).
func assertLiveRolloutBody(body []byte, status int) error {
	if status != http.StatusOK {
		return fmt.Errorf("rollout read status = %d, want 200; the executor could not read the Deployment: %s",
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
	fmt.Printf("executorLive: rollout read reports phase %s, %d/%d ready, revision %d from the live Deployment\n",
		rollout.Phase, rollout.Ready, rollout.Desired, rollout.Revision)
	return nil
}

// assertLiveApplyChangesTheRelease drives a real values patch through the apply
// endpoint and proves the release actually changed.
//
// A 200 is not evidence here: the fake-CLI tracer already returns one, and that
// is exactly what it cannot distinguish. What makes this different is the helm
// revision -- the executor's helm_upgrade ran in-cluster against the release,
// re-rendering the co-generated topology (srd006 R2.2), so a revision that did
// not move means nothing was applied whatever the response said.
func assertLiveApplyChangesTheRelease(profilesRoot string) error {
	before, err := helmReleaseRevision(executorLiveRelease)
	if err != nil {
		return err
	}
	fmt.Printf("executorLive: release at revision %d before the apply\n", before)

	patch, err := executorValuesPatch(profilesRoot, "conforming.yaml")
	if err != nil {
		return err
	}
	body, status, err := requestInference(http.MethodPost, executorLiveApplyURL, patch, "executor live apply")
	if err != nil {
		return fmt.Errorf("apply request failed: %w", err)
	}
	if status != http.StatusOK {
		return fmt.Errorf("apply status = %d, want 200: %s\n%s", status, body, executorPodDiagnostics())
	}
	if !strings.Contains(string(body), `"status":"applied"`) {
		return fmt.Errorf("apply did not report applied: %s", body)
	}

	after, err := helmReleaseRevision(executorLiveRelease)
	if err != nil {
		return err
	}
	if after <= before {
		return fmt.Errorf("the release is still at revision %d after an apply reported success; "+
			"helm_upgrade did not reach the release, so the 200 proved nothing", after)
	}
	fmt.Printf("executorLive: the apply moved the release from revision %d to %d\n", before, after)
	return nil
}

// assertLiveRollbackRestoresTheRelease proves the compensating action with real
// Helm and kubectl (srd006 R3.2, GH-751). The staged post-upgrade hook regresses
// the chatbot only after helm upgrade --wait has succeeded. The executor must
// therefore reach Verifying, observe kubectl's real timeout, run helm rollback,
// and map RolledBack to the distinct 500 response.
//
// Helm rollback creates a new release revision; it does not move the revision
// number backwards. Restoration is proved by comparing the computed release
// values and by waiting for the chatbot Deployment to become ready again.
func assertLiveRollbackRestoresTheRelease(profilesRoot string) error {
	beforeRevision, err := helmReleaseRevision(executorLiveRelease)
	if err != nil {
		return err
	}
	beforeValues, err := helmReleaseValues(executorLiveRelease)
	if err != nil {
		return err
	}
	patch, err := executorValuesPatch(profilesRoot, "rollback-trigger.yaml")
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: executorReadyWait}
	body, status, err := requestHTTPWithClient(client, http.MethodPost, executorLiveApplyURL, patch)
	if err != nil {
		return fmt.Errorf("rollback-triggering apply request failed: %w", err)
	}
	if status != http.StatusInternalServerError {
		return fmt.Errorf("rollback-triggering apply status = %d, want 500: %s\n%s",
			status, body, executorPodDiagnostics())
	}
	for _, want := range []string{`"error":"rolled_back"`, `"status":"rolled_back"`} {
		if !strings.Contains(string(body), want) {
			return fmt.Errorf("rollback response does not contain %s: %s", want, body)
		}
	}

	afterRevision, err := helmReleaseRevision(executorLiveRelease)
	if err != nil {
		return err
	}
	if afterRevision < beforeRevision+2 {
		return fmt.Errorf("release revision moved from %d to %d, want an upgrade and a rollback revision",
			beforeRevision, afterRevision)
	}
	afterValues, err := helmReleaseValues(executorLiveRelease)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(afterValues, beforeValues) {
		beforeJSON, _ := json.Marshal(beforeValues)
		afterJSON, _ := json.Marshal(afterValues)
		return fmt.Errorf("helm rollback did not restore the prior computed values:\nbefore: %s\nafter:  %s",
			beforeJSON, afterJSON)
	}

	deployment := "deployment/" + executorLiveRelease + "-chatbot-mesh-chatbot"
	cmd := exec.Command("kubectl", "rollout", "status", deployment, "--timeout", executorReadyWait.String())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chatbot Deployment did not recover after rollback: %w\n%s", err, out)
	}
	fmt.Printf("executorLive: real helm rollback restored revision %d values in new revision %d and recovered the chatbot\n",
		beforeRevision, afterRevision)
	return nil
}

// assertLiveSchemaRejection closes the loop GH-732 opened with a local dry-run:
// the same non-conforming document, now against a real release on a cluster.
// The release must not move -- a rejected patch applies nothing (srd006 R2.1).
func assertLiveSchemaRejection(profilesRoot string) error {
	before, err := helmReleaseRevision(executorLiveRelease)
	if err != nil {
		return err
	}
	patch, err := executorValuesPatch(profilesRoot, "non-conforming.yaml")
	if err != nil {
		return err
	}
	body, status, err := requestInference(http.MethodPost, executorLiveApplyURL, patch, "executor live reject")
	if err != nil {
		return fmt.Errorf("apply request failed: %w", err)
	}
	if status != http.StatusBadRequest {
		return fmt.Errorf("a non-conforming patch returned %d, want 400: %s", status, body)
	}
	if !strings.Contains(string(body), "validate_rejected") {
		return fmt.Errorf("the rejection did not report validate_rejected: %s", body)
	}
	after, err := helmReleaseRevision(executorLiveRelease)
	if err != nil {
		return err
	}
	if after != before {
		return fmt.Errorf("the release moved from revision %d to %d on a rejected patch; "+
			"a schema rejection must apply nothing", before, after)
	}
	fmt.Printf("executorLive: the non-conforming patch was rejected and left the release at revision %d\n", after)
	return nil
}

// executorPodDiagnostics returns the executor's own log tail. A live apply that
// fails reports only the terminal the machine reached; what helm actually said
// is in the pod, and without it a failure here is a mystery rather than a
// finding.
func executorPodDiagnostics() string {
	out, err := exec.Command("kubectl", "logs",
		"-l", "app.kubernetes.io/component=executor", "--tail", "60").CombinedOutput()
	if err != nil {
		return fmt.Sprintf("(could not read executor logs: %v)", err)
	}
	return "executor log tail:\n" + string(out)
}

// executorValuesPatch reads a shared fixture and wraps it as the apply request
// the creator would send (srd006 R1.4). The fixtures are GH-732's, so the local
// dry-run tier and this one validate the same documents.
func executorValuesPatch(profilesRoot, fixture string) (string, error) {
	path := filepath.Join(profilesRoot, "testdata", "integration", "executor-values", fixture)
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
