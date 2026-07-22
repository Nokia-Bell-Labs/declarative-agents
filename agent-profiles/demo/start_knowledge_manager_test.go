// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
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
  - name: launch_documentation
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
  - name: launch_documentation
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

func TestDocumentationCuratorMonitorIntegrationHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("integration needs a local agent build; skipped under -short")
	}
	profilesRoot := repoProfilesRootFromTest(t)
	coreRoot := filepath.Join(profilesRoot, "..", "agent-core")
	if _, err := os.Stat(filepath.Join(coreRoot, "cmd", "agent", "main.go")); err != nil {
		t.Skipf("sibling agent-core not found at %s", coreRoot)
	}
	distIndex := filepath.Join(profilesRoot, "agents", "knowledge-manager", "documentation-curator", "ui", "monitor", "dist", "index.html")
	if _, err := os.Stat(distIndex); err != nil {
		t.Skipf("monitor UX dist missing (build the monitor bundle first): %v", err)
	}

	agentBin := buildCuratorIntegrationAgent(t, coreRoot)
	tmpProfile := prepareDemoProfile(filepath.Join(profilesRoot, knowledgeManagerProfile), profilesRoot, coreRoot)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve monitor address: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	monitorAddress := listener.Addr().String()
	restPath := filepath.Join(filepath.Dir(tmpProfile), "rest.yaml")
	restConfig := readFile(restPath)
	const shippedMonitorAddress = "127.0.0.1:18084"
	if !strings.Contains(restConfig, shippedMonitorAddress) {
		t.Fatalf("temp profile rest config does not contain monitor address %s", shippedMonitorAddress)
	}
	writeDemoFile(t, restPath, strings.Replace(restConfig, shippedMonitorAddress, monitorAddress, 1))

	var logBuf bytes.Buffer
	cmd := exec.Command(agentBin, "--profile", tmpProfile, "--directory", coreRoot, "--core-root", coreRoot)
	cmd.Dir = profilesRoot
	cmd.Stdout = &logBuf
	cmd.Stderr = &logBuf
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved monitor address: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Log(logBuf.String())
		}
	})

	base := "http://" + monitorAddress
	pollHTTPStatus(t, base+"/monitor/state", http.StatusOK, 60*time.Second)
	stateBody := mustGETBody(t, base+"/monitor/state")
	if !strings.Contains(stateBody, `"run"`) {
		t.Fatalf("monitor state missing run field: %s", truncateForLog(stateBody, 400))
	}
	eventsBody := mustGETBody(t, base+"/monitor/events")
	if !strings.Contains(eventsBody, "recent_events") {
		t.Fatalf("monitor events missing recent_events: %s", truncateForLog(eventsBody, 400))
	}
	uiBody := mustGETBody(t, base+"/ui/index.html")
	if !strings.Contains(uiBody, `id="root"`) {
		t.Fatalf("monitor UI shell missing app root: %s", truncateForLog(uiBody, 200))
	}

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(base + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("GET / status = %d want %d", resp.StatusCode, http.StatusFound)
	}
	if got := resp.Header.Get("Location"); got != "/ui/" {
		t.Fatalf("Location = %q want /ui/", got)
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

func repoProfilesRootFromTest(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func buildCuratorIntegrationAgent(t *testing.T, coreRoot string) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "agent-integration")
	cmd := exec.Command("go", "build", "-tags", "production", "-o", out, "./cmd/agent")
	cmd.Dir = coreRoot
	combined, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build agent: %v\n%s", err, combined)
	}
	return out
}

func pollHTTPStatus(t *testing.T, url string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err != nil {
			last = err.Error()
			time.Sleep(150 * time.Millisecond)
			continue
		}
		code := resp.StatusCode
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if code == want {
			return
		}
		last = fmt.Sprintf("status %d", code)
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("poll %s want status %d: last=%s", url, want, last)
}

func mustGETBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body %s: %v", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: status %d body %s", url, resp.StatusCode, truncateForLog(string(b), 400))
	}
	return string(b)
}

func truncateForLog(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
