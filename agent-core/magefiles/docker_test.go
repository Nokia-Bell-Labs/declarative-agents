// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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

func TestResolveDockerBuildOptionsFromDemoConfig(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     dockerBuildOptions
	}{
		{
			name: "defaults",
			want: dockerBuildOptions{
				Image: defaultContainerImage,
				Ref:   "v0.20260612.2",
				NetRC: defaultContainerNetRC,
			},
		},
		{
			name: "overrides",
			contents: strings.Join([]string{
				"release_repo: https://example.invalid/agent-core.git",
				"container_image: registry.example/agent-core:test",
				"container_netrc: /tmp/build.netrc",
			}, "\n"),
			want: dockerBuildOptions{
				Image: "registry.example/agent-core:test",
				Ref:   "v0.20260612.2",
				Repo:  "https://example.invalid/agent-core.git",
				NetRC: "/tmp/build.netrc",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.contents != "" {
				writeAgentCoreDemoConfig(t, root, tc.contents)
			}
			got, err := resolveDockerBuildOptions("v0.20260612.2", root, func(string) (string, error) {
				return "/usr/bin/docker", nil
			})
			if err != nil {
				t.Fatalf("resolveDockerBuildOptions returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("resolveDockerBuildOptions = %#v, want %#v", got, tc.want)
			}
		})
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

func TestContainerBuildArgsIncludesVersionMetadata(t *testing.T) {
	got := containerBuildArgs(dockerBuildOptions{
		Image:   "agent-core:latest",
		Ref:     "v0.20260612.2",
		Version: "v0.20260612.2",
		Commit:  "abc1234",
		Date:    "2026-08-24T15:00:00-04:00",
	})
	want := []string{
		"build",
		"--progress=plain",
		"--build-arg", "AGENT_CORE_REF=v0.20260612.2",
		"--build-arg", "AGENT_VERSION=v0.20260612.2",
		"--build-arg", "AGENT_COMMIT=abc1234",
		"--build-arg", "AGENT_DATE=2026-08-24T15:00:00-04:00",
		"-t", "agent-core:latest",
		".",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("containerBuildArgs = %#v, want %#v", got, want)
	}
}

func TestWithContainerVersionUsesReleaseRef(t *testing.T) {
	git := func(args ...string) (string, error) {
		switch strings.Join(args, " ") {
		case "rev-parse --short v0.20260612.2":
			return "def5678", nil
		case "log -1 --format=%cI v0.20260612.2":
			return "2026-06-12T12:00:00Z", nil
		default:
			t.Fatalf("unexpected git args %q", args)
			return "", nil
		}
	}
	got := withContainerVersion(dockerBuildOptions{Ref: "v0.20260612.2"}, git)
	if got.Version != "v0.20260612.2" {
		t.Fatalf("Version = %q, want release ref", got.Version)
	}
	if got.Commit != "def5678" || got.Date != "2026-06-12T12:00:00Z" {
		t.Fatalf("commit/date = %q %q", got.Commit, got.Date)
	}
}

func TestWithContainerVersionKeepsRefWhenGitMissing(t *testing.T) {
	got := withContainerVersion(dockerBuildOptions{Ref: "v0.20260612.2"}, func(...string) (string, error) {
		return "", errors.New("git not found")
	})
	if got.Version != "v0.20260612.2" {
		t.Fatalf("Version = %q, want release ref even without git", got.Version)
	}
	if got.Commit != "" || got.Date != "" {
		t.Fatalf("commit/date should stay empty without git, got %q %q", got.Commit, got.Date)
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
		"mounted profile example: docker run --rm -v /path/to/applications/catalog:/profiles:ro -v '$PWD:/work' -w /work agent-core:latest --profile /profiles/agents/executor/profile.yaml --directory /work",
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

func TestDockerfileRuntimeExcludesAgentProfiles(t *testing.T) {
	content := readDockerfile(t)
	for _, forbidden := range []string{
		"/src/agents/executor",
		"/src/agents/critic",
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
		"ARG AGENT_VERSION=v0.0.0-dev",
		"ARG AGENT_COMMIT=unknown",
		"ARG AGENT_DATE=unknown",
		"-X ${VERSION_PKG}.Version=${AGENT_VERSION}",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("Dockerfile missing version injection %q", want)
		}
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
