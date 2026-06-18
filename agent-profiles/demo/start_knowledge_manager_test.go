// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestResolveDemoConfigFindsProfilesByWalking(t *testing.T) {
	root := t.TempDir()
	writeDemoFile(t, filepath.Join(root, knowledgeManagerProfile), "name: documentation-curator\n")
	demoDir := filepath.Join(root, "demo")
	mkdirDemoDir(t, demoDir)
	withWorkingDirectory(t, demoDir)

	cfg, err := ResolveDemoConfig("", "", "", "")
	if err != nil {
		t.Fatalf("ResolveDemoConfig: %v", err)
	}
	if got := cleanPath(t, cfg.ProfilesRoot); got != cleanPath(t, root) {
		t.Fatalf("ProfilesRoot = %q, want %q", got, cleanPath(t, root))
	}
}

func TestResolveDemoConfigExplicitProfilesRoot(t *testing.T) {
	root := t.TempDir()
	writeDemoFile(t, filepath.Join(root, knowledgeManagerProfile), "name: documentation-curator\n")

	cfg, err := ResolveDemoConfig(root, "", "", "")
	if err != nil {
		t.Fatalf("ResolveDemoConfig: %v", err)
	}
	if got := cleanPath(t, cfg.ProfilesRoot); got != cleanPath(t, root) {
		t.Fatalf("ProfilesRoot = %q, want %q", got, root)
	}
}

func TestResolveDemoConfigMissingProfilesReturnsError(t *testing.T) {
	tmp := t.TempDir()
	withWorkingDirectory(t, tmp)

	_, err := ResolveDemoConfig("", "", "", "")
	if err == nil {
		t.Fatal("expected error when profile tree is missing")
	}
	if !strings.Contains(err.Error(), "agent-profiles root not found") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentCommandUsesConfiguredBinary(t *testing.T) {
	cfg := demoConfig{
		ProfilesRoot: "/profiles",
		Workspace:    "/work",
		AgentBinary:  "/tmp/current-agent",
	}

	cmd := agentCommand("/profiles/agent/profile.yaml", "/work", cfg)

	want := []string{"/tmp/current-agent", "--profile", "/profiles/agent/profile.yaml", "--directory", "/work"}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("cmd.Args = %#v, want %#v", cmd.Args, want)
	}
	if cmd.Dir != "/profiles" {
		t.Fatalf("cmd.Dir = %q, want /profiles", cmd.Dir)
	}
}

func TestAgentCommandUsesCoreRootAddsFlag(t *testing.T) {
	coreRoot := t.TempDir()
	stubBuildAgentBinary(t, "/tmp/current-source-agent")

	cfg := demoConfig{
		ProfilesRoot: "/profiles",
		CoreRoot:     coreRoot,
		Workspace:    "/work",
	}

	cmd := agentCommand("/profiles/agent/profile.yaml", "/work", cfg)

	want := []string{
		"/tmp/current-source-agent",
		"--profile", "/profiles/agent/profile.yaml",
		"--directory", "/work",
		"--core-root", coreRoot,
	}
	if !reflect.DeepEqual(cmd.Args, want) {
		t.Fatalf("cmd.Args = %#v, want %#v", cmd.Args, want)
	}
	if cmd.Dir != "/profiles" {
		t.Fatalf("cmd.Dir = %q, want /profiles", cmd.Dir)
	}
}

func TestAgentCommandDiscoversSiblingCoreCheckout(t *testing.T) {
	parent := t.TempDir()
	profilesRoot := filepath.Join(parent, "agent-profiles")
	coreRoot := filepath.Join(parent, "agent-core")
	writeDemoFile(t, filepath.Join(profilesRoot, knowledgeManagerProfile), "name: documentation-curator\n")
	writeDemoFile(t, filepath.Join(coreRoot, "cmd", "agent", "main.go"), "package main\n")
	stubBuildAgentBinary(t, "/tmp/current-source-agent")

	cfg, err := ResolveDemoConfig(profilesRoot, "", "", "")
	if err != nil {
		t.Fatalf("ResolveDemoConfig: %v", err)
	}
	if cfg.CoreRoot != coreRoot {
		t.Fatalf("CoreRoot = %q, want %q", cfg.CoreRoot, coreRoot)
	}

	cmd := agentCommand("/profiles/agent/profile.yaml", "/work", cfg)

	if cmd.Args[0] != "/tmp/current-source-agent" {
		t.Fatalf("agent binary = %q, want /tmp/current-source-agent", cmd.Args[0])
	}
	if cmd.Dir != profilesRoot {
		t.Fatalf("cmd.Dir = %q, want %q", cmd.Dir, profilesRoot)
	}
	if got := strings.Join(cmd.Args, " "); !strings.Contains(got, "--core-root") {
		t.Fatalf("expected --core-root in args: %#v", cmd.Args)
	}
}

func TestPrepareDemoProfilePointsAtCoreDocs(t *testing.T) {
	profilesRoot := t.TempDir()
	coreRoot := t.TempDir()
	profileDir := filepath.Join(profilesRoot, filepath.Dir(knowledgeManagerProfile))
	writeDemoFile(t, filepath.Join(profileDir, "machine.yaml"), "name: documentation-curator\n")
	writeDemoFile(t, filepath.Join(profileDir, "tools.yaml"), "tools: []\n")
	writeDemoFile(t, filepath.Join(profileDir, "declarations.yaml"), "tools: []\n")
	writeDemoFile(t, filepath.Join(profileDir, "request-declarations.yaml"), "tools: []\n")
	writeDemoFile(t, filepath.Join(profileDir, "rest.yaml"), "rest:\n  openapi:\n    docs:\n      path: openapi.yaml\n")
	writeDemoFile(t, filepath.Join(profileDir, "openapi.yaml"), "openapi: 3.0.0\n")
	writeDemoFile(t, filepath.Join(profileDir, "request-machine.yaml"), "name: request\n")
	writeDemoFile(t, filepath.Join(profileDir, "ui", "ux.yaml"), "id: documentation-curator-ui\n")
	writeDemoFile(t, filepath.Join(profileDir, "builtin.yaml"), `tools:
  - name: serve_documentation
    config:
      docs_dir: docs
      configs_dir: configs
      source_dir: .
      profile_path: agents/knowledge-manager/documentation-curator/profile.yaml
      timeout: 30s
`)

	tmpProfile := prepareDemoProfile(filepath.Join(profilesRoot, knowledgeManagerProfile), profilesRoot, coreRoot)
	builtin := readFile(filepath.Join(filepath.Dir(tmpProfile), "builtin.yaml"))

	for _, want := range []string{
		"docs_dir: " + quotePath(filepath.Join(coreRoot, "docs")),
		"configs_dir: " + quotePath(filepath.Join(coreRoot, "configs")),
		"source_dir: " + quotePath(coreRoot),
		"profile_path: " + quotePath(tmpProfile),
		"timeout: 24h",
	} {
		if !strings.Contains(builtin, want) {
			t.Fatalf("builtin overlay missing %q in:\n%s", want, builtin)
		}
	}
	if !pathExists(filepath.Join(filepath.Dir(tmpProfile), "ui", "ux.yaml")) {
		t.Fatal("temp profile UX config was not copied")
	}
}

func TestPrepareDemoProfileCopiesMonitorAssetsAndRewritesRest(t *testing.T) {
	profilesRoot := t.TempDir()
	coreRoot := t.TempDir()
	profileDir := filepath.Join(profilesRoot, filepath.Dir(knowledgeManagerProfile))
	writeDemoFile(t, filepath.Join(profileDir, "machine.yaml"), "name: documentation-curator\n")
	writeDemoFile(t, filepath.Join(profileDir, "tools.yaml"), "tools: []\n")
	writeDemoFile(t, filepath.Join(profileDir, "declarations.yaml"), "tools: []\n")
	writeDemoFile(t, filepath.Join(profileDir, "request-declarations.yaml"), "tools: []\n")
	writeDemoFile(t, filepath.Join(profileDir, "rest.yaml"), "rest:\n  version: v1\n  servers:\n    monitor:\n      endpoints:\n        monitor_ui:\n          static_assets:\n            root: "+monitorDistYAMLPath+"\n        root_redirect:\n          method: GET\n          path: /\n          binding: redirect\n          redirect:\n            location: \"/ui/\"\n            status: 302\n")
	writeDemoFile(t, filepath.Join(profileDir, "openapi.yaml"), "openapi: 3.0.0\n")
	writeDemoFile(t, filepath.Join(profileDir, "request-machine.yaml"), "name: request\n")
	writeDemoFile(t, filepath.Join(profileDir, "ui", "ux.yaml"), "id: documentation-curator-ui\n")
	writeDemoFile(t, filepath.Join(profileDir, "ui", "monitor", "dist", "index.html"), "<html>monitor</html>\n")
	writeDemoFile(t, filepath.Join(profileDir, "builtin.yaml"), `tools:
  - name: serve_documentation
    config:
      docs_dir: docs
      configs_dir: configs
      source_dir: .
      profile_path: agents/knowledge-manager/documentation-curator/profile.yaml
      timeout: 30s
`)

	tmpProfile := prepareDemoProfile(filepath.Join(profilesRoot, knowledgeManagerProfile), profilesRoot, coreRoot)
	tmpDir := filepath.Dir(tmpProfile)

	gotDist := readFile(filepath.Join(tmpDir, "ui", "monitor", "dist", "index.html"))
	if gotDist != "<html>monitor</html>\n" {
		t.Fatalf("copied monitor dist index = %q", gotDist)
	}

	rest := readFile(filepath.Join(tmpDir, "rest.yaml"))
	wantDist := filepath.ToSlash(filepath.Join(tmpDir, "ui", "monitor", "dist"))
	if !strings.Contains(rest, wantDist) || strings.Contains(rest, monitorDistYAMLPath) {
		t.Fatalf("rest.yaml should rewrite monitor dist root to %q; got:\n%s", wantDist, rest)
	}
	if !strings.Contains(rest, `binding: redirect`) || !strings.Contains(rest, `location: "/ui/"`) {
		t.Fatalf("rest.yaml should keep redirect root config; got:\n%s", rest)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "ui", "monitor", "root-redirect")); err == nil {
		t.Fatal("expected no copied root-redirect tree under temp profile")
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat root-redirect: %v", err)
	}
}

func withWorkingDirectory(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(old); err != nil {
			t.Fatalf("restore working directory %s: %v", old, err)
		}
	})
}

func writeDemoFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirDemoDir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mkdirDemoDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func cleanPath(t *testing.T, path string) string {
	t.Helper()
	clean, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("canonicalize %s: %v", path, err)
	}
	return clean
}

func stubBuildAgentBinary(t *testing.T, binary string) {
	t.Helper()
	previous := buildAgentBinaryFunc
	buildAgentBinaryFunc = func(string) string {
		return binary
	}
	t.Cleanup(func() {
		buildAgentBinaryFunc = previous
	})
}

func quotePath(path string) string {
	return `"` + path + `"`
}
