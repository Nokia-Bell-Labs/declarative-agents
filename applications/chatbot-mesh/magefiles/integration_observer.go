// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	observerProfile     = "agents/observer/profile.yaml"
	observerHealthPath  = "/api/lifecycle/health"
	observerExitPath    = "/api/lifecycle/exit"
	observerStatePath   = "/monitor/state"
	observerMachinePath = "/monitor/machine"
)

// Observer proves the observer agent boots, serves its monitor surface, and
// reaches a running machine state. The scenario launches the observer profile
// locally with the kube client pointed at a non-routable address (so discovery
// degrades gracefully), verifies the health and monitor endpoints respond, and
// stops the agent through its lifecycle exit. Skips when agent-core is absent.
func (Integration) Observer() error {
	applicationRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := demoCoreRoot(applicationRoot)
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP integration:observer: agent-core checkout not found at %s\n", coreRoot)
		return nil
	}
	if err := requireProfilePaths(applicationRoot, observerProfile); err != nil {
		return err
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}

	controlAddr, err := freeLoopbackAddr()
	if err != nil {
		return err
	}
	monitorAddr, err := freeLoopbackAddr()
	if err != nil {
		return err
	}
	_, controlPort, _ := splitHostPort(controlAddr)
	_, monitorPort, _ := splitHostPort(monitorAddr)

	workDir, err := os.MkdirTemp("", "observer-integration-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(workDir) }()

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
		// Point kube client at a non-routable address so discovery degrades
		// instead of hanging on a real kube API.
		"KUBE_API_URL=https://127.0.0.1:1",
		"OBSERVER_KUBE_TIMEOUT=1s",
		"OBSERVER_AGENT_TIMEOUT=1s",
	)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start observer: %w", err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	controlURL := "http://" + controlAddr
	monitorURL := "http://" + monitorAddr
	start := time.Now()

	if err := waitHTTPStatus(controlURL+observerHealthPath, http.StatusOK, 20*time.Second); err != nil {
		return fmt.Errorf("observer health: %w\n%s", err, output.String())
	}

	machine, err := observerGetJSON(monitorURL + observerMachinePath)
	if err != nil {
		return fmt.Errorf("observer machine: %w\n%s", err, output.String())
	}
	if name, _ := machine["name"].(string); name != "observer" {
		return fmt.Errorf("observer machine name = %q, want %q", name, "observer")
	}

	state, err := observerGetJSON(monitorURL + observerStatePath)
	if err != nil {
		return fmt.Errorf("observer state: %w\n%s", err, output.String())
	}
	if !observerHasRunState(state) {
		return fmt.Errorf("observer state missing run state: %v", state)
	}

	req, _ := http.NewRequest(http.MethodPost,
		controlURL+observerExitPath,
		strings.NewReader(`{"reason":"integration complete"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("stop observer: %w", err)
	}
	_ = resp.Body.Close()

	if err := cmd.Wait(); err != nil {
		if !agentRunCompleted(err) {
			return fmt.Errorf("observer exit: %w\n%s", err, output.String())
		}
	}
	fmt.Printf("integration:observer passed in %s: observer booted, monitor surface verified, clean exit\n",
		time.Since(start).Round(time.Millisecond))
	return nil
}

// observerGetJSON GETs a URL and decodes the response as JSON.
func observerGetJSON(url string) (map[string]interface{}, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET %s: %d %s", url, resp.StatusCode, string(body))
	}
	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode %s: %w", url, err)
	}
	return result, nil
}

// observerHasRunState checks whether the monitor state response contains a
// machine run state, which may be at top level (current_state) or nested under
// the run object (run.state).
func observerHasRunState(state map[string]interface{}) bool {
	if cs, ok := state["current_state"].(string); ok && cs != "" {
		return true
	}
	if run, ok := state["run"].(map[string]interface{}); ok {
		if s, ok := run["state"].(string); ok && s != "" {
			return true
		}
	}
	return false
}

// splitHostPort splits a "host:port" string. Unlike net.SplitHostPort, it does
// not fail when the host portion contains colons, because the callers here only
// use loopback addresses produced by freeLoopbackAddr.
func splitHostPort(addr string) (host, port string, err error) {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("no port in %q", addr)
	}
	return addr[:idx], addr[idx+1:], nil
}

// observerKindIntegration runs against a live kind cluster to verify kube API
// discovery and monitor fan-in. Its skip-guarded test is not yet wired, so it
// has no caller; kept rather than deleted so the coverage work reuses it.
//
//nolint:unused // entry point for the not-yet-wired observer kind-integration test (GH-1213 follow-up)
func observerKindIntegration(applicationRoot, coreRoot, kubeAPIURL, namespace, labelSelector string) (int, error) {
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return 0, err
	}

	controlAddr, err := freeLoopbackAddr()
	if err != nil {
		return 0, err
	}
	monitorAddr, err := freeLoopbackAddr()
	if err != nil {
		return 0, err
	}
	_, controlPort, _ := splitHostPort(controlAddr)
	_, monitorPort, _ := splitHostPort(monitorAddr)

	workDir, err := os.MkdirTemp("", "observer-kind-*")
	if err != nil {
		return 0, err
	}
	defer func() { _ = os.RemoveAll(workDir) }()

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
		"OBSERVER_POLL_INTERVAL=3s",
		"KUBE_API_URL="+kubeAPIURL,
		"OBSERVER_NAMESPACE="+namespace,
		"OBSERVER_LABEL_SELECTOR="+labelSelector,
		"OBSERVER_KUBE_TIMEOUT=5s",
		"OBSERVER_AGENT_TIMEOUT=3s",
	)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("start observer: %w", err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()

	controlURL := "http://" + controlAddr
	monitorURL := "http://" + monitorAddr

	if err := waitHTTPStatus(controlURL+observerHealthPath, http.StatusOK, 30*time.Second); err != nil {
		return 0, fmt.Errorf("observer health: %w\n%s", err, output.String())
	}

	// Allow two poll cycles for discovery + fan-in.
	time.Sleep(8 * time.Second)

	state, err := observerGetJSON(monitorURL + observerStatePath)
	if err != nil {
		return 0, fmt.Errorf("observer state: %w\n%s", err, output.String())
	}

	discovered := 0
	if agents, ok := state["agents"].([]interface{}); ok {
		discovered = len(agents)
	} else if fleet, ok := state["fleet"].([]interface{}); ok {
		discovered = len(fleet)
	}

	req, _ := http.NewRequest(http.MethodPost,
		controlURL+observerExitPath,
		strings.NewReader(`{"reason":"kind integration complete"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	_ = cmd.Wait()

	return discovered, nil
}
