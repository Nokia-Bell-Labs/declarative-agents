// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestContainerEngineUsesOverride(t *testing.T) {
	got, err := containerEngine("docker", func(name string) (string, error) {
		t.Fatalf("lookPath called for override %q", name)
		return "", nil
	})
	if err != nil {
		t.Fatalf("containerEngine returned error: %v", err)
	}
	if got != "docker" {
		t.Fatalf("containerEngine = %q, want docker", got)
	}
}

func TestContainerEnginePrefersDocker(t *testing.T) {
	got, err := containerEngine("", func(name string) (string, error) {
		if name == "docker" {
			return "/usr/bin/docker", nil
		}
		if name == "podman" {
			return "/usr/bin/podman", nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatalf("containerEngine returned error: %v", err)
	}
	if got != "docker" {
		t.Fatalf("containerEngine = %q, want docker", got)
	}
}

func TestContainerEngineFallsBackToPodman(t *testing.T) {
	got, err := containerEngine("", func(name string) (string, error) {
		if name == "podman" {
			return "/usr/bin/podman", nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatalf("containerEngine returned error: %v", err)
	}
	if got != "podman" {
		t.Fatalf("containerEngine = %q, want podman", got)
	}
}

func TestContainerEngineErrorsWhenMissing(t *testing.T) {
	_, err := containerEngine("", func(name string) (string, error) {
		return "", errors.New("missing")
	})
	if err == nil {
		t.Fatal("containerEngine returned nil error for missing engines")
	}
	if !strings.Contains(err.Error(), envContainerEngine) {
		t.Fatalf("error = %q, want mention %s", err, envContainerEngine)
	}
}

func TestContainerBuildArgsForPodman(t *testing.T) {
	got := containerBuildArgs(dockerBuildOptions{
		Engine:    "podman",
		Image:     "registry.example/agent-core:test",
		Ref:       "v0.20260612.2",
		Repo:      "https://example.invalid/agent-core.git",
		NetRC:     "/home/user/.netrc",
		TLSVerify: "false",
	})
	want := []string{
		"build",
		"--tls-verify=false",
		"--secret", "id=git_credentials,src=/home/user/.netrc",
		"--build-arg", "AGENT_CORE_REF=v0.20260612.2",
		"--build-arg", "AGENT_CORE_REPO=https://example.invalid/agent-core.git",
		"-t", "registry.example/agent-core:test",
		".",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("containerBuildArgs = %#v, want %#v", got, want)
	}
}

func TestContainerBuildArgsForDocker(t *testing.T) {
	got := containerBuildArgs(dockerBuildOptions{
		Engine: "docker",
		Image:  "agent-core:latest",
		Ref:    "v0.20260612.2",
	})
	want := []string{
		"build",
		"--progress=plain",
		"--build-arg", "AGENT_CORE_REF=v0.20260612.2",
		"-t", "agent-core:latest",
		".",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("containerBuildArgs = %#v, want %#v", got, want)
	}
}

func TestContainerBuildSummaryForPodman(t *testing.T) {
	opts := dockerBuildOptions{
		Engine:    "podman",
		Image:     "agent-core:latest",
		Ref:       "v0.20260612.1",
		NetRC:     "/home/user/.netrc",
		TLSVerify: "false",
	}
	args := containerBuildArgs(opts)
	got := containerBuildSummary(opts, args)
	for _, want := range []string{
		"building agent-core:latest from v0.20260612.1 with podman",
		"  engine: podman",
		"  image: agent-core:latest",
		"  release ref: v0.20260612.1",
		"  source repo: https://gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core.git (Dockerfile default)",
		"  git credentials secret: /home/user/.netrc",
		"  podman tls verify: false",
		"  container output: streamed directly",
		"command: podman build --tls-verify=false --secret id=git_credentials,src=/home/user/.netrc --build-arg AGENT_CORE_REF=v0.20260612.1 -t agent-core:latest .",
		"mounted profile example: podman run --rm -v '$AGENT_PROFILES_ROOT:/profiles/agents:ro' -v '$PWD:/work' -w /work agent-core:latest --profile /profiles/agents/generator/profile.yaml --directory /work",
		"integration image command: podman build --tls-verify=false --secret id=git_credentials,src=/home/user/.netrc --build-arg AGENT_CORE_REF=v0.20260612.1 -t agent-core-integration:latest --target integration .",
		"integration container example: podman run --rm -v '$AGENT_PROFILES_ROOT:/profiles/agents:ro' -w /src -e AGENT_PROFILES_ROOT=/profiles/agents agent-core-integration:latest mage integration:uc006",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("containerBuildSummary missing %q in:\n%s", want, got)
		}
	}
}

func TestDisplayBuildCommandForDockerIncludesBuildkit(t *testing.T) {
	opts := dockerBuildOptions{
		Engine: "docker",
		Image:  "agent-core:latest",
		Ref:    "v0.20260612.1",
	}
	got := displayBuildCommand(opts, containerBuildArgs(opts))
	want := "DOCKER_BUILDKIT=1 docker build --progress=plain --build-arg AGENT_CORE_REF=v0.20260612.1 -t agent-core:latest ."
	if got != want {
		t.Fatalf("displayBuildCommand = %q, want %q", got, want)
	}
}

func TestDisplayIntegrationBuildCommandUsesTarget(t *testing.T) {
	opts := dockerBuildOptions{
		Engine: "docker",
		Image:  "agent-core:latest",
		Ref:    "v0.20260612.1",
	}
	got := displayIntegrationBuildCommand(opts)
	want := "DOCKER_BUILDKIT=1 docker build --progress=plain --build-arg AGENT_CORE_REF=v0.20260612.1 -t agent-core-integration:latest --target integration ."
	if got != want {
		t.Fatalf("displayIntegrationBuildCommand = %q, want %q", got, want)
	}
}

func TestDockerfileRuntimeExcludesAgentProfiles(t *testing.T) {
	content := readDockerfile(t)
	for _, forbidden := range []string{
		"/src/agents/generator",
		"/src/agents/evaluator",
		"/opt/agent-core/agents",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("Dockerfile contains forbidden profile copy path %q", forbidden)
		}
	}
	if !strings.Contains(content, "COPY --from=builder /src/tools /opt/agent-core/tools") {
		t.Fatal("Dockerfile should copy core-owned tools into the runtime image")
	}
	for _, want := range []string{
		"Error: --profile is required; mount profiles and pass --profile /profiles/agents/<name>/profile.yaml",
		"ENTRYPOINT [\"agent-entrypoint\"]",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("Dockerfile missing %q", want)
		}
	}
}

func TestDockerfileDefinesIntegrationTarget(t *testing.T) {
	content := readDockerfile(t)
	for _, want := range []string{
		"FROM builder AS integration",
		"RUN apk add --no-cache nodejs npm",
		"go install github.com/magefile/mage@latest",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("Dockerfile missing %q", want)
		}
	}
}

func readDockerfile(t *testing.T) string {
	t.Helper()
	for _, path := range []string{"Dockerfile", "../Dockerfile"} {
		data, err := os.ReadFile(path)
		if err == nil {
			return string(data)
		}
	}
	t.Fatal("read Dockerfile")
	return ""
}

func TestShellCommand(t *testing.T) {
	got := shellCommand([]string{
		"podman",
		"build",
		"--secret",
		"id=git_credentials,src=/Users/test user/.netrc",
		"--build-arg",
		"AGENT_CORE_REF=v0.20260612.1",
		"--build-arg",
		"AGENT_CORE_REPO=https://example.invalid/agent-core.git",
		"-t",
		"agent-core:latest",
		".",
	})
	want := "podman build --secret 'id=git_credentials,src=/Users/test user/.netrc' --build-arg AGENT_CORE_REF=v0.20260612.1 --build-arg AGENT_CORE_REPO=https://example.invalid/agent-core.git -t agent-core:latest ."
	if got != want {
		t.Fatalf("shellCommand = %q, want %q", got, want)
	}
}

func TestShellQuoteEscapesSingleQuote(t *testing.T) {
	got := shellQuote("repo's")
	want := "'repo'\\''s'"
	if got != want {
		t.Fatalf("shellQuote = %q, want %q", got, want)
	}
}

func TestTLSVerifyForEngine(t *testing.T) {
	if got := tlsVerifyForEngine("podman", ""); got != "false" {
		t.Fatalf("tlsVerifyForEngine podman default = %q, want false", got)
	}
	if got := tlsVerifyForEngine("docker", ""); got != "" {
		t.Fatalf("tlsVerifyForEngine docker default = %q, want empty", got)
	}
	if got := tlsVerifyForEngine("podman", "true"); got != "true" {
		t.Fatalf("tlsVerifyForEngine override = %q, want true", got)
	}
}
