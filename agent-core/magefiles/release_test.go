// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadAgentCoreDemoConfig(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     agentCoreDemoConfig
	}{
		{name: "absent"},
		{name: "commented", contents: "# release_ref: v0.20260612.1\n"},
		{
			name: "overrides",
			contents: strings.Join([]string{
				"release_ref: v0.20260612.2",
				"release_repo: https://example.invalid/agent-core.git",
				"container_image: registry.example/agent-core:test",
				"container_netrc: /tmp/build.netrc",
				"dolt_bin: /opt/dolt/bin/dolt",
			}, "\n"),
			want: agentCoreDemoConfig{
				ReleaseRef:     "v0.20260612.2",
				ReleaseRepo:    "https://example.invalid/agent-core.git",
				ContainerImage: "registry.example/agent-core:test",
				ContainerNetRC: "/tmp/build.netrc",
				DoltBin:        "/opt/dolt/bin/dolt",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.contents != "" {
				writeAgentCoreDemoConfig(t, root, tc.contents)
			}
			got, err := loadAgentCoreDemoConfig(root)
			if err != nil {
				t.Fatalf("loadAgentCoreDemoConfig returned error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("loadAgentCoreDemoConfig = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestContainerReleaseRefFromDemoConfig(t *testing.T) {
	t.Run("release ref", func(t *testing.T) {
		root := t.TempDir()
		writeAgentCoreDemoConfig(t, root, "release_ref: v0.20260612.2\n")
		got, err := containerReleaseRefFrom(root, func(args ...string) (string, error) {
			t.Fatal("containerReleaseRefFrom called git despite release_ref")
			return "", nil
		})
		if err != nil {
			t.Fatalf("containerReleaseRefFrom returned error: %v", err)
		}
		if got != "v0.20260612.2" {
			t.Fatalf("containerReleaseRefFrom = %q, want v0.20260612.2", got)
		}
	})
	t.Run("release repo", func(t *testing.T) {
		root := t.TempDir()
		writeAgentCoreDemoConfig(t, root, "release_repo: https://example.invalid/agent-core.git\n")
		got, err := containerReleaseRefFrom(root, func(args ...string) (string, error) {
			want := "ls-remote --tags --refs https://example.invalid/agent-core.git v0.*"
			if strings.Join(args, " ") != want {
				t.Fatalf("git args = %q, want %q", strings.Join(args, " "), want)
			}
			return "abc123\trefs/tags/v0.20260612.3", nil
		})
		if err != nil {
			t.Fatalf("containerReleaseRefFrom returned error: %v", err)
		}
		if got != "v0.20260612.3" {
			t.Fatalf("containerReleaseRefFrom = %q, want v0.20260612.3", got)
		}
	})
}

func TestLoadAgentCoreDemoConfigRejectsMalformedYAML(t *testing.T) {
	root := t.TempDir()
	writeAgentCoreDemoConfig(t, root, "release_ref: [\n")
	if _, err := loadAgentCoreDemoConfig(root); err == nil {
		t.Fatal("loadAgentCoreDemoConfig returned nil error for malformed demo.yaml")
	}
}

func TestResolveDoltBinary(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		wantLookup string
	}{
		{name: "path fallback", wantLookup: "dolt"},
		{name: "declared setting", configured: " /opt/dolt/bin/dolt ", wantLookup: "/opt/dolt/bin/dolt"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveDoltBinary(tc.configured, func(name string) (string, error) {
				if name != tc.wantLookup {
					t.Fatalf("lookPath name = %q, want %q", name, tc.wantLookup)
				}
				return "/resolved/dolt", nil
			})
			if err != nil {
				t.Fatalf("resolveDoltBinary returned error: %v", err)
			}
			if got != "/resolved/dolt" {
				t.Fatalf("resolveDoltBinary = %q, want /resolved/dolt", got)
			}
		})
	}
}

func TestResolveContainerReleaseRefUsesOverride(t *testing.T) {
	called := false
	got, err := resolveContainerReleaseRef(" v0.20260612.2 ", "", func(args ...string) (string, error) {
		called = true
		return "", nil
	})
	if err != nil {
		t.Fatalf("resolveContainerReleaseRef override returned error: %v", err)
	}
	if got != "v0.20260612.2" {
		t.Fatalf("resolveContainerReleaseRef override = %q, want v0.20260612.2", got)
	}
	if called {
		t.Fatal("resolveContainerReleaseRef called git despite override")
	}
}

func TestResolveContainerReleaseRefFindsLatestReleaseTag(t *testing.T) {
	got, err := resolveContainerReleaseRef("", "https://example.invalid/agent-core.git", func(args ...string) (string, error) {
		if strings.Join(args, " ") != "ls-remote --tags --refs https://example.invalid/agent-core.git v0.*" {
			t.Fatalf("git args = %q, want remote release tag query", strings.Join(args, " "))
		}
		return strings.Join([]string{
			"abc123\trefs/tags/v0.20260611.4",
			"def456\trefs/tags/not-a-release",
			"abc789\trefs/tags/v0.20260612.1",
			"def012\trefs/tags/v0.20260612.10",
			"abc345\trefs/tags/v0.20260612.bad",
			"def678\trefs/tags/v0.20260609.99",
		}, "\n"), nil
	})
	if err != nil {
		t.Fatalf("resolveContainerReleaseRef returned error: %v", err)
	}
	if got != "v0.20260612.10" {
		t.Fatalf("resolveContainerReleaseRef = %q, want v0.20260612.10", got)
	}
}

func TestResolveContainerReleaseRefErrorsWhenNoReleaseTags(t *testing.T) {
	_, err := resolveContainerReleaseRef("", "", func(args ...string) (string, error) {
		return "abc123\trefs/tags/v1.0.0\njunk\nabc456\trefs/tags/v0.20260612", nil
	})
	if err == nil {
		t.Fatal("resolveContainerReleaseRef returned nil error for no release tags")
	}
	if !strings.Contains(err.Error(), "no release tags") {
		t.Fatalf("error = %q, want no release tags", err)
	}
}

func TestResolveContainerReleaseRefWrapsGitError(t *testing.T) {
	want := errors.New("git unavailable")
	_, err := resolveContainerReleaseRef("", "", func(args ...string) (string, error) {
		return "", want
	})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v, want to wrap %v", err, want)
	}
}

func TestRemoteReleaseTagNames(t *testing.T) {
	got := remoteReleaseTagNames(strings.Join([]string{
		"abc123\trefs/tags/v0.20260612.1",
		"missing-fields",
		"def456\trefs/heads/main",
		"ghi789\trefs/tags/v0.20260612.2",
	}, "\n"))
	want := []string{"v0.20260612.1", "v0.20260612.2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("remoteReleaseTagNames = %#v, want %#v", got, want)
	}
}

func TestLatestReleaseTag(t *testing.T) {
	got, ok := latestReleaseTag([]string{
		"v0.20260608.4",
		"v0.20260608.12",
		"v0.20260609.0",
	})
	if !ok {
		t.Fatal("latestReleaseTag returned ok=false")
	}
	if got != "v0.20260609.0" {
		t.Fatalf("latestReleaseTag = %q, want v0.20260609.0", got)
	}
}

func writeAgentCoreDemoConfig(t *testing.T, root, contents string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, agentCoreDemoFile), []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
