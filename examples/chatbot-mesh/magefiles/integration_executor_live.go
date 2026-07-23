// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
)

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
	fmt.Printf("executorLive: building executor image %s on %s\n", executorLiveImage, helmImage)
	if err := buildExecutorImage(profilesRoot, helmImage, executorLiveImage); err != nil {
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

	chartDir := exampleChartDir(profilesRoot)
	staged, cleanupChart, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return err
	}
	defer cleanupChart()
	if err := helmInstallExecutorLive(staged); err != nil {
		return err
	}
	if err := waitExecutorDeploymentReady(); err != nil {
		return err
	}
	if err := assertExecutorServesItsSurface(); err != nil {
		return err
	}
	fmt.Println("integration:executorLive PASS - the executor image builds on the runtime under test, " +
		"carries the helm and kubectl its declarations name, ships a chart that renders every profile, " +
		"and runs on kind reading a real Deployment's rollout")
	return nil
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
// image rather than a published tag, so the live tier runs the code under test
// the way the smoke does.
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
func buildExecutorImage(profilesRoot, runtimeImage, image string) error {
	chartDir := exampleChartDir(profilesRoot)
	staged, cleanupChart, err := stageSmokeChart(chartDir, profilesRoot)
	if err != nil {
		return fmt.Errorf("stage the chart the image ships: %w", err)
	}
	defer cleanupChart()

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
func assertExecutorServesItsSurface() error {
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
