// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(applicationRoot, "agent-core"))
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
