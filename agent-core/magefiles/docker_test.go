// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestRequireDocker(t *testing.T) {
	if err := requireDocker(func(name string) (string, error) {
		if name == "docker" {
			return "/usr/bin/docker", nil
		}
		return "", errors.New("missing")
	}); err != nil {
		t.Fatalf("requireDocker with docker present returned error: %v", err)
	}
	if err := requireDocker(func(string) (string, error) {
		return "", errors.New("missing")
	}); err == nil {
		t.Fatal("requireDocker without docker should return an error")
	}
}

func TestContainerBuildArgsForDocker(t *testing.T) {
	got := containerBuildArgs(dockerBuildOptions{
		Image: "agent-core:latest",
		Ref:   "v0.20260612.2",
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

func TestContainerBuildSummaryForDocker(t *testing.T) {
	opts := dockerBuildOptions{
		Image: "agent-core:latest",
		Ref:   "v0.20260612.1",
		NetRC: "/home/user/.netrc",
	}
	args := containerBuildArgs(opts)
	got := containerBuildSummary(opts, args)
	for _, want := range []string{
		"building agent-core:latest from v0.20260612.1 with docker",
		"  engine: docker",
		"  image: agent-core:latest",
		"  release ref: v0.20260612.1",
		"  source repo: https://github.com/Nokia-Bell-Labs/declarative-agents/agent-core.git (Dockerfile default)",
		"  git credentials secret: /home/user/.netrc",
		"  docker buildkit: enabled",
		"  docker progress: plain",
		"  container output: streamed directly",
		"command: DOCKER_BUILDKIT=1 docker build --progress=plain --secret id=git_credentials,src=/home/user/.netrc --build-arg AGENT_CORE_REF=v0.20260612.1 -t agent-core:latest .",
		"mounted profile example: docker run --rm -v /path/to/agent-profiles:/profiles/agents:ro -v '$PWD:/work' -w /work agent-core:latest --profile /profiles/agents/agents/generator/profile.yaml --directory /work",
		"integration container example: docker run --rm -v /path/to/agent-profiles:/profiles/agents:ro -w /src agent-core-integration:latest mage integration:uc001",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("containerBuildSummary missing %q in:\n%s", want, got)
		}
	}
}

func TestDisplayBuildCommandForDockerIncludesBuildkit(t *testing.T) {
	opts := dockerBuildOptions{
		Image: "agent-core:latest",
		Ref:   "v0.20260612.1",
	}
	got := displayBuildCommand(opts, containerBuildArgs(opts))
	want := "DOCKER_BUILDKIT=1 docker build --progress=plain --build-arg AGENT_CORE_REF=v0.20260612.1 -t agent-core:latest ."
	if got != want {
		t.Fatalf("displayBuildCommand = %q, want %q", got, want)
	}
}

func TestDisplayIntegrationBuildCommandUsesTarget(t *testing.T) {
	opts := dockerBuildOptions{
		Image: "agent-core:latest",
		Ref:   "v0.20260612.1",
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
		"docker",
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
	want := "docker build --secret 'id=git_credentials,src=/Users/test user/.netrc' --build-arg AGENT_CORE_REF=v0.20260612.1 --build-arg AGENT_CORE_REPO=https://example.invalid/agent-core.git -t agent-core:latest ."
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
