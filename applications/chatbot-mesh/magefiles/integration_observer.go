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
	observerFleetPath   = "/monitor/fleet"

	// observerMonitorReadSignal is the signal a monitor fan-in item emits when it
	// read its agent's endpoint; any other signal marks that agent unreachable.
	observerMonitorReadSignal = "AgentMonitorRead"
)

// observerFleetLabels are the command_state labels the fleet view exposes, one
// per aggregation step. The discovery steps are addressed by command name
// (GH-1270); the four monitor fan-in steps are for_each joins that all execute
// under the command name for_each.join, so each is addressed by its declared
// join label instead (GH-1319).
var observerFleetLabels = []string{
	"discover_mesh_pods",
	"list_mesh_deployments",
	"list_mesh_services",
	"agent_machine_fanin",
	"agent_state_fanin",
	"agent_tools_fanin",
	"agent_events_fanin",
	"poll_pod_metrics",
}

// observerFanInLabels are the monitor fan-in join labels, the subset whose
// entries carry per-pod outcomes.
var observerFanInLabels = []string{
	"agent_machine_fanin",
	"agent_state_fanin",
	"agent_tools_fanin",
	"agent_events_fanin",
}

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
		// Point the kube client at a non-routable loopback address so discovery
		// degrades instead of hanging. http matches the client's proxy-only
		// network limits (GH-1302).
		"KUBE_API_URL=http://127.0.0.1:1",
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

	// The fleet view serves the command_state allowlist even when discovery
	// degrades: every declared label is a key whose value is an entry or null.
	fleet, err := observerFleetLabelsView(monitorURL)
	if err != nil {
		return fmt.Errorf("observer fleet: %w\n%s", err, output.String())
	}
	for _, label := range observerFleetLabels {
		if _, ok := fleet[label]; !ok {
			return fmt.Errorf("observer fleet view missing declared label %q: %v", label, fleet)
		}
	}
	// This scenario points the kube client at a non-routable address, so
	// discovery degrades and the fan-in never resolves a pod list: its join
	// entries stay null rather than failing the cycle. When a cycle did fan in,
	// the join's reachable and unreachable counts must cover every dispatched
	// item (srd008 R2.3).
	for _, label := range observerFanInLabels {
		items, reachable, unreachable, ok := observerFanInCounts(fleet, label)
		if !ok {
			continue
		}
		if reachable+unreachable != items {
			return fmt.Errorf(
				"observer fan-in %q reports %d reachable + %d unreachable for %d items",
				label, reachable, unreachable, items)
		}
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
	fmt.Printf("integration:observer passed in %s: observer booted, monitor and fan-in surface verified, clean exit\n",
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

// observerFleetLabelsView GETs the command_state fleet view and returns its
// labels object, which maps each declared label to an entry or null (GH-1270).
func observerFleetLabelsView(monitorURL string) (map[string]interface{}, error) {
	body, err := observerGetJSON(monitorURL + observerFleetPath)
	if err != nil {
		return nil, err
	}
	labels, ok := body["labels"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("fleet response missing labels object: %v", body)
	}
	return labels, nil
}

// observerFleetDiscoveredPods counts the pods the observer discovered from the
// fleet view's discover_mesh_pods entry, returning 0 when the label has no
// recorded step or its output is unavailable. The REST client exposes the
// operation's response.output result under `mapped`, so the pod list lives at
// output.mapped.pods (GH-1302).
func observerFleetDiscoveredPods(labels map[string]interface{}) int {
	mapped, ok := observerLabelOutput(labels, "discover_mesh_pods")
	if !ok {
		return 0
	}
	nested, ok := mapped["mapped"].(map[string]interface{})
	if !ok {
		return 0
	}
	pods, ok := nested["pods"].([]interface{})
	if !ok {
		return 0
	}
	return len(pods)
}

// observerLabelOutput returns one fleet label's recorded output, reporting false
// when the step is absent or its output did not cross the redaction boundary.
func observerLabelOutput(labels map[string]interface{}, label string) (map[string]interface{}, bool) {
	entry, ok := labels[label].(map[string]interface{})
	if !ok {
		return nil, false
	}
	output, ok := entry["output"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	return output, true
}

// observerFanInCounts reads one monitor fan-in join's per-item outcomes and
// splits them into reachable and unreachable agents. The split comes from each
// item's own result signal, not the join's succeeded and failed counts: the
// fan-in lists CommandError in continue_on so one unreachable agent cannot halt
// the pass (srd008 R2.3), which makes the join count every dispatched item as
// succeeded (GH-1319).
func observerFanInCounts(labels map[string]interface{}, label string) (items, reachable, unreachable int, ok bool) {
	output, found := observerLabelOutput(labels, label)
	if !found {
		return 0, 0, 0, false
	}
	entries, found := output["items"].([]interface{})
	if !found {
		return 0, 0, 0, false
	}
	for _, entry := range entries {
		if observerItemReachable(entry) {
			reachable++
			continue
		}
		unreachable++
	}
	return len(entries), reachable, unreachable, true
}

// observerItemReachable reports whether one fan-in item read its agent's monitor
// endpoint, which the item's recorded result signal states.
func observerItemReachable(entry interface{}) bool {
	item, ok := entry.(map[string]interface{})
	if !ok {
		return false
	}
	result, ok := item["result"].(map[string]interface{})
	if !ok {
		return false
	}
	signal, _ := result["signal"].(string)
	return signal == observerMonitorReadSignal
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

// observerKindFleet summarizes one fan-in cycle observed on a kind cluster: the
// pods discovery listed, and how many of them the monitor fan-in reached.
type observerKindFleet struct {
	Pods        int
	Items       int
	Reachable   int
	Unreachable int
}

// observerKindIntegration runs against a live kind cluster to verify kube API
// discovery and monitor fan-in. TestObserverKindIntegration is its skip-guarded
// caller (GH-1226), gated on the -kind-kube-api-url go-test flag.
func observerKindIntegration(applicationRoot, coreRoot, kubeAPIURL, namespace, labelSelector string) (observerKindFleet, error) {
	var fleetSummary observerKindFleet
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return fleetSummary, err
	}

	controlAddr, err := freeLoopbackAddr()
	if err != nil {
		return fleetSummary, err
	}
	monitorAddr, err := freeLoopbackAddr()
	if err != nil {
		return fleetSummary, err
	}
	_, controlPort, _ := splitHostPort(controlAddr)
	_, monitorPort, _ := splitHostPort(monitorAddr)

	workDir, err := os.MkdirTemp("", "observer-kind-*")
	if err != nil {
		return fleetSummary, err
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
		return fleetSummary, fmt.Errorf("start observer: %w", err)
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
		return fleetSummary, fmt.Errorf("observer health: %w\n%s", err, output.String())
	}

	// Allow two poll cycles for discovery + fan-in.
	time.Sleep(8 * time.Second)

	fleet, err := observerFleetLabelsView(monitorURL)
	if err != nil {
		return fleetSummary, fmt.Errorf("observer fleet: %w\n%s", err, output.String())
	}
	fleetSummary.Pods = observerFleetDiscoveredPods(fleet)
	// The state fan-in is the liveness pass, so its per-item outcomes are the
	// reachable split the fleet cards render (srd008 R3.3, GH-1319).
	items, reachable, unreachable, _ := observerFanInCounts(fleet, "agent_state_fanin")
	fleetSummary.Items, fleetSummary.Reachable, fleetSummary.Unreachable = items, reachable, unreachable

	req, _ := http.NewRequest(http.MethodPost,
		controlURL+observerExitPath,
		strings.NewReader(`{"reason":"kind integration complete"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	_ = cmd.Wait()

	return fleetSummary, nil
}
