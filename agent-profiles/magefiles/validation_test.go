// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestValidateProfilesResolvesExternalCoreToolRefs(t *testing.T) {
	root := t.TempDir()
	coreRoot := t.TempDir()
	writeProfileFixture(t, root, "generator")
	mkdir(t, filepath.Join(coreRoot, "tools", "builtin", "llm"))

	if err := validateProfiles(root, coreRoot); err != nil {
		t.Fatalf("validateProfiles returned error: %v", err)
	}
}

func TestValidateProfilesRejectsCopiedCoreAgentRefs(t *testing.T) {
	root := t.TempDir()
	coreRoot := t.TempDir()
	writeProfileFixture(t, root, "generator")
	profilePath := filepath.Join(root, "agents", "generator", "profile.yaml")
	appendFile(t, profilePath, "tool_declarations:\n  - /opt/agent-core/agents/generator/profile.yaml\n")

	err := validateProfiles(root, coreRoot)
	if err == nil {
		t.Fatal("validateProfiles returned nil error for copied agent asset reference")
	}
	if !strings.Contains(err.Error(), "must not require copied core agent assets") {
		t.Fatalf("error = %q, want copied asset rejection", err)
	}
}

func TestValidateProfilesReportsMissingReference(t *testing.T) {
	root := t.TempDir()
	coreRoot := t.TempDir()
	writeProfileFixture(t, root, "generator")

	err := validateProfiles(root, coreRoot)
	if err == nil {
		t.Fatal("validateProfiles returned nil error for missing core tools")
	}
	if !strings.Contains(err.Error(), "missing referenced path /opt/agent-core/tools/builtin/llm") {
		t.Fatalf("error = %q, want missing core tool path", err)
	}
}

func TestProfileContainerEngine(t *testing.T) {
	got, err := profileContainerEngine("podman", func(name string) (string, error) {
		t.Fatalf("lookPath called for override %q", name)
		return "", nil
	})
	if err != nil {
		t.Fatalf("profileContainerEngine override returned error: %v", err)
	}
	if got != "podman" {
		t.Fatalf("profileContainerEngine = %q, want podman", got)
	}

	got, err = profileContainerEngine("", func(name string) (string, error) {
		if name == "docker" {
			return "/usr/bin/docker", nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatalf("profileContainerEngine lookup returned error: %v", err)
	}
	if got != "docker" {
		t.Fatalf("profileContainerEngine lookup = %q, want docker", got)
	}
}

func TestRunContainerSmokeCommands(t *testing.T) {
	var calls [][]string
	err := runContainerSmoke("docker", func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}, "/profiles-src", "/core-src", "agent-core:test")
	if err != nil {
		t.Fatalf("runContainerSmoke returned error: %v", err)
	}
	want := [][]string{
		{"docker", "run", "--rm", "--entrypoint", "sh", "agent-core:test", "-c", "test ! -e /opt/agent-core/agents"},
		{"docker", "run", "--rm", "-v", "/profiles-src:/profiles:ro", "-v", "/core-src/tools:/opt/agent-core/tools:ro", "-v", "/profiles-src:/work:ro", "-w", "/work", "agent-core:test", "--profile", "/profiles/agents/jurist/profile.yaml", "--directory", "/work"},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("container calls = %#v, want %#v", calls, want)
	}
}

func writeProfileFixture(t *testing.T, root, name string) {
	t.Helper()
	dir := filepath.Join(root, "agents", name)
	mkdir(t, dir)
	writeFile(t, filepath.Join(dir, "machine.yaml"), "name: test\n")
	writeFile(t, filepath.Join(dir, "tools.yaml"), "tools: []\n")
	writeFile(t, filepath.Join(dir, "profile.yaml"), `name: test
machine: machine.yaml
tools:
  - tools.yaml
tool_config_dirs:
  - /opt/agent-core/tools/builtin/llm
`)
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func appendFile(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			t.Fatalf("close %s: %v", path, err)
		}
	}()
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
}
