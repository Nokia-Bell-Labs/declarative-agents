// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const (
	executorLiveImage   = "declarative-agents/chatbot-mesh-executor:live"
	executorLiveCluster = "da-chatbot-mesh-executor"
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
	fmt.Println("integration:executorLive PASS - the executor image builds on the runtime under test, " +
		"carries the helm and kubectl its declarations name, ships the chart at /chart, and loads into kind")
	return nil
}

// buildExecutorImage builds the executor image from the locally built runtime
// image rather than a published tag, so the live tier runs the code under test
// the way the smoke does.
//
// TARGETARCH is passed explicitly. The Dockerfile defaults it to amd64, and a
// plain `docker build` on an arm64 host does not set it -- the result is an
// arm64 image carrying amd64 helm and kubectl, which fail with an exec format
// error the first time an exec word runs one. The kind nodes are the host's
// architecture, so the image has to be too.
func buildExecutorImage(profilesRoot, runtimeImage, image string) error {
	cmd := exec.Command("docker", "build",
		"-f", "executor.Dockerfile",
		"--build-arg", "RUNTIME_IMAGE="+runtimeImage,
		"--build-arg", "TARGETARCH="+runtime.GOARCH,
		"-t", image, ".")
	cmd.Dir = profilesRoot
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
	return assertExecutorImageHelmMajor(image)
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
