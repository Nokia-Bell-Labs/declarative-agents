// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"flag"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var (
	kindKubeAPIURL = flag.String("kind-kube-api-url", "",
		"Kubernetes API URL for the observer kind integration")
	kindNamespace = flag.String("kind-namespace", "default",
		"Kubernetes namespace for the observer kind integration")
	kindLabelSelector = flag.String("kind-label-selector", "app.kubernetes.io/instance=chatbot-mesh",
		"Kubernetes label selector for the observer kind integration")
)

func TestObserverProfileExists(t *testing.T) {
	applicationRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(applicationRoot, observerProfile)); err != nil {
		t.Fatalf("observer profile missing: %v", err)
	}
}

func TestObserverMonitorEndpoints(t *testing.T) {
	applicationRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	coreRoot := demoCoreRoot(applicationRoot)
	if !agentCoreAvailable(coreRoot) {
		t.Skipf("agent-core checkout not found at %s", coreRoot)
	}
	if err := requireProfilePaths(applicationRoot, observerProfile); err != nil {
		t.Fatal(err)
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		t.Fatal(err)
	}

	controlAddr, err := freeLoopbackAddr()
	if err != nil {
		t.Fatal(err)
	}
	monitorAddr, err := freeLoopbackAddr()
	if err != nil {
		t.Fatal(err)
	}
	_, controlPort, _ := splitHostPort(controlAddr)
	_, monitorPort, _ := splitHostPort(monitorAddr)

	workDir := t.TempDir()

	cmd := exec.Command(binary,
		"--profile", observerProfile,
		"--core-root", coreRoot,
		"--directory", workDir,
	)
	cmd.Dir = applicationRoot
	cmd.Env = append(os.Environ(),
		"OBSERVER_BIND_HOST=127.0.0.1",
		"OBSERVER_CONTROL_PORT="+controlPort,
		"OBSERVER_MONITOR_PORT="+monitorPort,
		"OBSERVER_POLL_INTERVAL=2s",
		"KUBE_API_URL=https://127.0.0.1:1",
		"OBSERVER_KUBE_TIMEOUT=1s",
		"OBSERVER_AGENT_TIMEOUT=1s",
	)

	if err := cmd.Start(); err != nil {
		t.Fatalf("start observer: %v", err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	controlURL := "http://" + controlAddr
	monitorURL := "http://" + monitorAddr

	if err := waitHTTPStatus(controlURL+observerHealthPath, http.StatusOK, 20*time.Second); err != nil {
		t.Fatalf("observer health: %v", err)
	}

	machine, err := observerGetJSON(monitorURL + observerMachinePath)
	if err != nil {
		t.Fatalf("observer machine: %v", err)
	}
	if name, _ := machine["name"].(string); name != "observer" {
		t.Fatalf("observer machine name = %q, want %q", name, "observer")
	}

	state, err := observerGetJSON(monitorURL + observerStatePath)
	if err != nil {
		t.Fatalf("observer state: %v", err)
	}
	if !observerHasRunState(state) {
		t.Fatalf("observer state missing run state: %v", state)
	}

	req, _ := http.NewRequest(http.MethodPost,
		controlURL+observerExitPath,
		strings.NewReader(`{"reason":"test complete"}`))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
	}
	_ = cmd.Wait()
}

func TestSplitHostPort(t *testing.T) {
	tests := []struct {
		addr, wantHost, wantPort string
	}{
		{"127.0.0.1:8080", "127.0.0.1", "8080"},
		{"localhost:443", "localhost", "443"},
	}
	for _, tt := range tests {
		host, port, err := splitHostPort(tt.addr)
		if err != nil {
			t.Errorf("splitHostPort(%q): %v", tt.addr, err)
			continue
		}
		if host != tt.wantHost || port != tt.wantPort {
			t.Errorf("splitHostPort(%q) = %q, %q; want %q, %q",
				tt.addr, host, port, tt.wantHost, tt.wantPort)
		}
	}
}

// TestObserverRBACRender proves the observer's rendered RBAC is a namespaced
// Role (never a ClusterRole) granting only get/list on pods, services,
// deployments, and metrics.k8s.io pod metrics -- no write verbs (srd008 R5, AC5).
func TestObserverRBACRender(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	out, err := exec.Command("helm", "template", "t", findChartDir(t), "--set", "observer.enabled=true").CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	var observerRole string
	for _, doc := range strings.Split(string(out), "\n---") {
		if strings.Contains(doc, "kind: Role") && strings.Contains(doc, "name: t-chatbot-mesh-observer") {
			observerRole = doc
			break
		}
	}
	if observerRole == "" {
		t.Fatal("observer Role not rendered")
	}
	for _, want := range []string{"resources: [pods, services]", "apiGroups: [apps]", "resources: [deployments]", "apiGroups: [metrics.k8s.io]"} {
		if !strings.Contains(observerRole, want) {
			t.Errorf("observer Role missing %q", want)
		}
	}
	for _, writeVerb := range []string{"create", "update", "patch", "delete", "watch"} {
		if strings.Contains(observerRole, writeVerb) {
			t.Errorf("observer Role must be read-only but contains verb %q", writeVerb)
		}
	}
}

// TestObserverKindIntegration runs observerKindIntegration against a live kind
// cluster when -kind-kube-api-url is passed, verifying kube-API discovery and
// monitor fan-in on a real cluster (GH-1226); it skips cleanly otherwise.
func TestObserverKindIntegration(t *testing.T) {
	if *kindKubeAPIURL == "" {
		t.Skip("pass -kind-kube-api-url with go test -args to run the observer kind integration")
	}
	applicationRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	coreRoot := demoCoreRoot(applicationRoot)
	if !agentCoreAvailable(coreRoot) {
		t.Skipf("agent-core checkout not found at %s", coreRoot)
	}
	discovered, err := observerKindIntegration(
		applicationRoot, coreRoot, *kindKubeAPIURL, *kindNamespace, *kindLabelSelector,
	)
	if err != nil {
		t.Fatalf("observer kind integration: %v", err)
	}
	if discovered == 0 {
		t.Errorf("observer discovered 0 agents on the kind cluster; want at least one")
	}
}
