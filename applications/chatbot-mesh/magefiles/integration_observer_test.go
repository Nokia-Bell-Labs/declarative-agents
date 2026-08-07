// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
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

	// A single goroutine owns cmd.Wait; its result is delivered exactly once.
	// reaped is closed once the main flow has consumed that result so the
	// cleanup below knows the process is already accounted for.
	waitErr := make(chan error, 1)
	go func() { waitErr <- cmd.Wait() }()
	reaped := make(chan struct{})

	// Unconditional kill + bounded reap on every exit path (including early
	// t.Fatal), so a stuck agent can never leak or block the test forever.
	t.Cleanup(func() {
		select {
		case <-reaped:
			return
		default:
		}
		_ = cmd.Process.Kill()
		select {
		case <-waitErr:
		case <-time.After(5 * time.Second):
		}
	})

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		controlURL+observerExitPath,
		strings.NewReader(`{"reason":"test complete"}`))
	if err != nil {
		t.Fatalf("build observer exit request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post observer exit: %v", err)
	}
	status := resp.StatusCode
	_ = resp.Body.Close()
	if status != http.StatusAccepted {
		t.Fatalf("observer exit status = %d, want %d", status, http.StatusAccepted)
	}

	select {
	case waitResult := <-waitErr:
		close(reaped)
		if !agentRunCompleted(waitResult) {
			t.Fatalf("observer exit contract violated: %v", waitResult)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("observer did not exit within 15s after accepted exit request")
	}
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
	fleet, err := observerKindIntegration(
		applicationRoot, coreRoot, *kindKubeAPIURL, *kindNamespace, *kindLabelSelector,
	)
	if err != nil {
		t.Fatalf("observer kind integration: %v", err)
	}
	if fleet.Pods == 0 {
		t.Fatalf("observer discovered 0 pods on the kind cluster; want at least one")
	}
	// The fan-in dispatches one read per discovered pod, so a short item list
	// means the iteration did not cover the pod set.
	if fleet.Items != fleet.Pods {
		t.Errorf("observer fanned in %d items for %d discovered pods; want one per pod",
			fleet.Items, fleet.Pods)
	}
	if fleet.Reachable+fleet.Unreachable != fleet.Items {
		t.Errorf("observer fan-in split %d reachable + %d unreachable over %d items",
			fleet.Reachable, fleet.Unreachable, fleet.Items)
	}
	// The observer discovers itself and carries its own monitor-port label, so at
	// least one agent must be reachable on a healthy cluster.
	if fleet.Reachable == 0 {
		t.Errorf("observer reached 0 of %d discovered agents; want at least itself", fleet.Items)
	}
}
