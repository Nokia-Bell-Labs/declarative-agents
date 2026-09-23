// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"strings"
	"testing"
)

const (
	sampleGit    = "a1b2c3d4e5f6"
	sampleRecipe = "7fc291a0b4e2"
	sampleDigest = "sha256:9c4976d47656d78cf53a92b0203fc54ac45eae18a2b45001ac221c27da4c8036"
)

func TestFormatLocalRoundTripsEveryRoleAndIdentity(t *testing.T) {
	cases := []LocalImage{
		{Role: RuntimeRole, Component: "agent-core", Type: GitIdentity,
			Revision: sampleGit, OS: "linux", Arch: "arm64"},
		{Role: TestRole, Component: "coding-model", Type: GitIdentity,
			Revision: sampleGit, OS: "linux", Arch: "amd64"},
		{Role: CacheRole, Component: "ollama-models", Type: RecipeIdentity,
			Recipe: sampleRecipe, OS: "linux", Arch: "arm64"},
		{Role: DerivedRole, Component: "ollama", Type: UpstreamIdentity,
			UpstreamVersion: "0.34.2", Recipe: sampleRecipe, OS: "linux", Arch: "amd64"},
		{Role: DerivedRole, Component: "ollama", Type: UpstreamIdentity,
			UpstreamVersion: "v3.7.10", Recipe: sampleRecipe, OS: "linux", Arch: "arm64"},
	}
	for _, image := range cases {
		formatted, err := FormatLocal(image)
		if err != nil {
			t.Fatalf("FormatLocal(%+v): %v", image, err)
		}
		parsed, err := ParseLocal(formatted)
		if err != nil {
			t.Fatalf("ParseLocal(%q): %v", formatted, err)
		}
		again, err := FormatLocal(parsed)
		if err != nil {
			t.Fatal(err)
		}
		if again != formatted {
			t.Errorf("round-trip %q → %q", formatted, again)
		}
		if parsed.Role != image.Role || parsed.Component != image.Component ||
			parsed.Type != image.Type || parsed.OS != image.OS || parsed.Arch != image.Arch {
			t.Errorf("parsed %+v, want %+v", parsed, image)
		}
	}
}

func TestFormatHelpersMatchCanonicalStrings(t *testing.T) {
	git, err := FormatGitLocal(RuntimeRole, "agent-core", sampleGit+"deadbeef", "linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	if git != "localhost/declarative-agents/runtime/agent-core:git-a1b2c3d4e5f6-linux-arm64" {
		t.Errorf("git = %q", git)
	}
	recipe, err := FormatRecipeLocal(CacheRole, "ollama-models", "sha256:"+sampleRecipe+strings.Repeat("0", 52), "linux-amd64")
	if err != nil {
		t.Fatal(err)
	}
	if recipe != "localhost/declarative-agents/cache/ollama-models:recipe-7fc291a0b4e2-linux-amd64" {
		t.Errorf("recipe = %q", recipe)
	}
	derived, err := FormatDerivedLocal("ollama", "0.32.5-kind-trusted", sampleRecipe, "linux/arm64")
	if err != nil {
		t.Fatal(err)
	}
	if derived != "localhost/declarative-agents/derived/ollama:upstream-0.32.5-kind-trusted-recipe-7fc291a0b4e2-linux-arm64" {
		t.Errorf("derived = %q", derived)
	}
	parsed, err := ParseLocal(git)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Revision != sampleGit {
		t.Errorf("short revision = %q", parsed.Revision)
	}
}

func TestParseLocalRejectsMalformed(t *testing.T) {
	for _, ref := range []string{
		"",
		"declarative-agents/runtime/agent-core:git-" + sampleGit + "-linux-arm64",
		"localhost/declarative-agents/agent-core:git-" + sampleGit + "-linux-arm64",
		"localhost/declarative-agents/other/agent-core:git-" + sampleGit + "-linux-arm64",
		"localhost/declarative-agents/runtime/:git-" + sampleGit + "-linux-arm64",
		"localhost/declarative-agents/runtime/agent-core",
		"localhost/declarative-agents/runtime/agent-core:local",
		"localhost/declarative-agents/runtime/agent-core:latest",
		"localhost/declarative-agents/runtime/agent-core:" + sampleGit,
		"localhost/declarative-agents/runtime/agent-core:commit-" + sampleGit,
		"localhost/declarative-agents/runtime/example-smoke:git-" + sampleGit + "-linux-arm64",
		"localhost/declarative-agents/runtime/agent-core:git-" + sampleGit + "-windows-amd64",
		"localhost/declarative-agents/runtime/agent-core:git-abc-linux-arm64",
		"localhost/declarative-agents/runtime/agent-core:git-" + sampleGit + "-linux-arm64@sha256:" + strings.Repeat("a", 64),
	} {
		if _, err := ParseLocal(ref); err == nil {
			t.Errorf("ParseLocal(%q) succeeded", ref)
		}
	}
}

func TestNormalizeUpstreamPreservesTagAndDigest(t *testing.T) {
	cases := map[string]string{
		"busybox:1.36@" + sampleDigest:                                         "docker.io/library/busybox:1.36@" + sampleDigest,
		"chromadb/chroma:1.5.3@" + sampleDigest:                                "docker.io/chromadb/chroma:1.5.3@" + sampleDigest,
		"docker.io/library/golang:1.26-alpine@" + sampleDigest:                 "docker.io/library/golang:1.26-alpine@" + sampleDigest,
		"docker.io/busybox:1.36":                                               "docker.io/library/busybox:1.36",
		"index.docker.io/library/traefik:v3.7.10":                              "docker.io/library/traefik:v3.7.10",
		"registry.k8s.io/metrics-server/metrics-server:v0.9.0@" + sampleDigest: "registry.k8s.io/metrics-server/metrics-server:v0.9.0@" + sampleDigest,
		"ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0.1.0":          "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:0.1.0",
		"quay.io/calico/cni:v3.30.2@" + sampleDigest:                           "quay.io/calico/cni:v3.30.2@" + sampleDigest,
		"localhost:5000/agents/agent-core:a1b2c3d4e5f6":                        "localhost:5000/agents/agent-core:a1b2c3d4e5f6",
		"docker.io/alpine/k8s:1.31.4@" + strings.ToUpper(sampleDigest):         "docker.io/alpine/k8s:1.31.4@" + sampleDigest,
	}
	for input, want := range cases {
		got, err := NormalizeUpstream(input)
		if err != nil {
			t.Errorf("NormalizeUpstream(%q): %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeUpstream(%q) = %q, want %q", input, got, want)
		}
		parsed, err := ParseUpstream(got)
		if err != nil {
			t.Errorf("ParseUpstream(%q): %v", got, err)
			continue
		}
		if parsed.String() != want {
			t.Errorf("round-trip %q → %q", want, parsed.String())
		}
	}
}

func TestParseUpstreamRejectsMutableAndAmbiguous(t *testing.T) {
	for _, ref := range []string{
		"",
		"busybox",
		"busybox:latest",
		"docker.io/library/busybox:local",
		"kindrig/traefik:v3.7.10",
		"docker.io/kindrig/cli-donor:1.31.4",
		"declarative-agents/example-smoke:" + sampleGit,
		"declarative-agents/agent-core:" + sampleGit,
		"localhost/declarative-agents/runtime/agent-core:git-" + sampleGit + "-linux-arm64",
		"busybox:1.36@sha256:deadbeef",
		"busybox:1.36@md5:" + strings.Repeat("a", 32),
	} {
		if _, err := ParseUpstream(ref); err == nil {
			t.Errorf("ParseUpstream(%q) succeeded", ref)
		}
	}
}

func TestParseUpstreamRegistryPortIsNotATag(t *testing.T) {
	image, err := ParseUpstream("localhost:5000/agent-core:1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	if image.Registry != "localhost:5000" || image.Path != "agent-core" || image.Tag != "1.2.3" {
		t.Fatalf("%+v", image)
	}
}

func TestFormatLocalRejectsInvalidCombinations(t *testing.T) {
	for _, image := range []LocalImage{
		{Role: "other", Component: "agent-core", Type: GitIdentity, Revision: sampleGit, OS: "linux", Arch: "arm64"},
		{Role: RuntimeRole, Component: "Agent Core", Type: GitIdentity, Revision: sampleGit, OS: "linux", Arch: "arm64"},
		{Role: RuntimeRole, Component: "example-smoke", Type: GitIdentity, Revision: sampleGit, OS: "linux", Arch: "arm64"},
		{Role: RuntimeRole, Component: "agent-core", Type: GitIdentity, Revision: "short", OS: "linux", Arch: "arm64"},
		{Role: RuntimeRole, Component: "agent-core", Type: GitIdentity, Revision: sampleGit, Recipe: sampleRecipe, OS: "linux", Arch: "arm64"},
		{Role: CacheRole, Component: "ollama-models", Type: RecipeIdentity, OS: "linux", Arch: "arm64"},
		{Role: DerivedRole, Component: "ollama", Type: UpstreamIdentity, Recipe: sampleRecipe, UpstreamVersion: "0.1-recipe-x", OS: "linux", Arch: "arm64"},
		{Role: RuntimeRole, Component: "agent-core", Type: GitIdentity, Revision: sampleGit, OS: "darwin", Arch: "arm64"},
	} {
		if _, err := FormatLocal(image); err == nil {
			t.Errorf("FormatLocal(%+v) succeeded", image)
		}
	}
}

func TestSplitPlatform(t *testing.T) {
	for _, platform := range []string{"linux/arm64", "linux-amd64", "Linux/ARM64"} {
		osName, arch, err := SplitPlatform(platform)
		if err != nil {
			t.Errorf("SplitPlatform(%q): %v", platform, err)
		}
		if osName != "linux" || (arch != "arm64" && arch != "amd64") {
			t.Errorf("SplitPlatform(%q) = %s/%s", platform, osName, arch)
		}
	}
	for _, platform := range []string{"", "arm64", "windows/amd64", "linux/", "linux-arm-v7"} {
		if _, _, err := SplitPlatform(platform); err == nil {
			t.Errorf("SplitPlatform(%q) succeeded", platform)
		}
	}
}

func TestParseLocalFoldsRepositoryCase(t *testing.T) {
	parsed, err := ParseLocal("localhost/Declarative-Agents/Runtime/Agent-Core:git-" + sampleGit + "-linux-arm64")
	if err != nil {
		t.Fatal(err)
	}
	if parsed.String() != "localhost/declarative-agents/runtime/agent-core:git-"+sampleGit+"-linux-arm64" {
		t.Fatalf("canonical = %q", parsed.String())
	}
}

func TestParseLocalErrorsNameTheProblem(t *testing.T) {
	_, err := ParseLocal("localhost/declarative-agents/runtime/agent-core:local")
	if err == nil || !strings.Contains(err.Error(), "mutable") {
		t.Fatalf("missing mutable diagnosis: %v", err)
	}
	_, err = ParseUpstream("kindrig/cli-donor:1.31.4")
	if err == nil || !strings.Contains(err.Error(), "kindrig") {
		t.Fatalf("missing kindrig diagnosis: %v", err)
	}
}
