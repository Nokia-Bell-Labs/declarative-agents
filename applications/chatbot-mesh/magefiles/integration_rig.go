// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
)

const rigProfile = "testdata/rig/profile.yaml"

// rigVerdictPattern matches the scenario critic's per-scenario verdict signals in
// trace output order. Discovery sorts scenarios by subject directory then
// name, so the catalog reference subject's three scenarios come first
// (its root path sorts before "agents"), then this application's rag-server query.
var rigExpectedVerdicts = []string{
	"ScenarioFailed", // rig-subject/broken: the deliberately wrong expectation must fail
	"ScenarioPassed", // rig-subject/dep-failure: the subject degraded correctly
	"ScenarioPassed", // rig-subject/happy-path
	"ScenarioPassed", // chatbot/degraded-rag: the turn answers while rag0 fails every query
	"ScenarioPassed", // chatbot/single-turn: the routed turn against mock provider and RAGs
	"ScenarioPassed", // rag-server/query: the mesh agent against a mock Chroma
}

// Rig runs the scenario critic test rig over this application's agents and the
// catalog reference subject in one pass — the cross-root proof: one
// rig, two repository areas, discovered by convention. The rag-server is
// exercised end to end against a mock Chroma pinned to the port the
// server's network limits allow; no live Chroma, Ollama, Docker, or
// Kubernetes is involved. The aggregate is failed by design, because the
// reference subject ships a deliberately broken scenario that must fail; this
// target asserts the exact verdict pattern instead. Skips (does not fail)
// when the agent-core checkout is unavailable.
func (Integration) Rig() error {
	applicationRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault("AGENT_CORE_ROOT", siblingPath(applicationRoot, "agent-core"))
	if !agentCoreAvailable(coreRoot) {
		fmt.Printf("SKIP integration:rig: agent-core checkout not found at %s\n", coreRoot)
		return nil
	}
	catalogRoot, err := resolveCatalogRoot("chatbot-mesh integration rig", applicationRoot)
	if err != nil {
		return err
	}
	if err := requireSharedObservability(helmReadyTimeout); err != nil {
		return fmt.Errorf("shared observability stack is required: %w", err)
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return err
	}

	// The scenario critic's children resolve "agent" from PATH; stage the built
	// binary under that name.
	binDir, err := os.MkdirTemp("", "rig-bin")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(binDir) }()
	staged := filepath.Join(binDir, "agent")
	data, err := os.ReadFile(binary)
	if err != nil {
		return err
	}
	if err := os.WriteFile(staged, data, 0o755); err != nil {
		return err
	}
	if err := runCollectorIntakeScenario(staged, coreRoot, catalogRoot, binDir); err != nil {
		return fmt.Errorf("collector intake-filter scenario: %w", err)
	}
	stagedRigRoot, cleanupRig, err := stageRigRuntime(applicationRoot, catalogRoot)
	if err != nil {
		return err
	}
	defer cleanupRig()

	trace := filepath.Join(binDir, "rig.otel.json")
	args := []string{
		"--profile", filepath.Join(stagedRigRoot, filepath.FromSlash(rigProfile)),
		"--core-root", coreRoot,
		"--otel-log-file", trace,
	}
	endpoint := firstNonEmpty(
		os.Getenv(integrationOTLPEndpointEnv),
		"127.0.0.1:"+envOrDefault("DA_OTEL_GRPC_PORT", "4317"),
	)
	runID := firstNonEmpty(os.Getenv(integrationRunIDEnv), generatedRunID("integration:rig"))
	commit := firstNonEmpty(os.Getenv(integrationCommitEnv), gitCommit(applicationRoot))
	telemetryArgs := integrationTelemetryArgs(endpoint, "scenario-critic-rig")
	resourceEnv := "OTEL_RESOURCE_ATTRIBUTES=" +
		integrationResourceAttributes("integration:rig", runID, commit)
	cmd := exec.Command(binary, append(args, telemetryArgs...)...)
	cmd.Dir = applicationRoot
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"AGENT_CATALOG_ROOT="+catalogRoot,
		"SCENARIO_CRITIC_OTEL_ENDPOINT="+endpoint,
		resourceEnv,
	)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	fmt.Println("running the catalog scenario critic rig over application agents and the catalog reference subject...")
	start := time.Now()
	// The rig's aggregate is failed by design: the reference subject ships a
	// scenario that must fail, so the run reaches a failure terminal and exits
	// with the machine-failed code (srd018-cli-flag-contract R6). Only a run
	// the binary could not complete is an error here; the verdict pattern
	// below is the real assertion.
	if err := cmd.Run(); !agentRunCompleted(err) {
		return fmt.Errorf("rig run: %w\n%s", err, out.String())
	}

	verdicts := rigVerdicts(trace)
	if len(verdicts) != len(rigExpectedVerdicts) {
		return fmt.Errorf("rig verdicts = %v, want %d scenarios\noutput:\n%s", verdicts, len(rigExpectedVerdicts), out.String())
	}
	for i, want := range rigExpectedVerdicts {
		if verdicts[i] != want {
			return fmt.Errorf("rig verdict[%d] = %s, want %s (order: rig-subject broken, dep-failure, happy-path; chatbot degraded-rag, single-turn; rag-server query)\nall: %v",
				i, verdicts[i], want, verdicts)
		}
	}
	fmt.Printf("integration:rig passed in %s: %d scenarios across two roots, verdicts %v for run %s\n",
		time.Since(start).Round(time.Millisecond), len(verdicts), verdicts, runID)
	return nil
}

func stageRigRuntime(applicationRoot, catalogRoot string) (string, func(), error) {
	stage, err := os.MkdirTemp("", "chatbot-mesh-rig-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(stage) }
	if err := copyDirContents(
		filepath.Join(applicationRoot, "testdata", "rig"),
		filepath.Join(stage, "testdata", "rig"),
	); err != nil {
		cleanup()
		return "", nil, err
	}
	if err := copyDirContents(
		filepath.Join(catalogRoot, "agents", "scenario-critic"),
		filepath.Join(stage, "agents", "scenario-critic"),
	); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("stage catalog scenario critic: %w", err)
	}
	return stage, cleanup, nil
}

type collectorIntakeScenario struct {
	Batch             string `yaml:"batch"`
	ExpectedSpanCount int    `yaml:"expected_span_count"`
	ExpectedService   string `yaml:"expected_service"`
}

// runCollectorIntakeScenario drives the shipped collector profile over a real
// OTLP/gRPC boundary with a canned protobuf-JSON batch. The spool assertion is
// the deterministic rig verdict for the receive -> positive-span filter -> spool leg.
func runCollectorIntakeScenario(binary, coreRoot, catalogRoot, workDir string) error {
	scenarioDir := filepath.Join(catalogRoot, "agents/collector/tests/intake-filter")
	var scenario collectorIntakeScenario
	if err := readIntegrationYAML(filepath.Join(scenarioDir, "scenario.yaml"), "collector scenario", &scenario); err != nil {
		return err
	}
	batchJSON, err := os.ReadFile(filepath.Join(scenarioDir, scenario.Batch))
	if err != nil {
		return fmt.Errorf("read collector batch: %w", err)
	}
	var batch coltracepb.ExportTraceServiceRequest
	if err := protojson.Unmarshal(batchJSON, &batch); err != nil {
		return fmt.Errorf("parse collector batch: %w", err)
	}
	receiver, err := freeLoopbackAddr()
	if err != nil {
		return err
	}
	control, err := freeLoopbackAddr()
	if err != nil {
		return err
	}
	_, controlPort, err := net.SplitHostPort(control)
	if err != nil {
		return err
	}
	monitor, err := freeLoopbackAddr()
	if err != nil {
		return err
	}
	_, monitorPort, err := net.SplitHostPort(monitor)
	if err != nil {
		return err
	}
	query, err := freeLoopbackAddr()
	if err != nil {
		return err
	}
	_, queryPort, err := net.SplitHostPort(query)
	if err != nil {
		return err
	}
	spool := filepath.Join(workDir, "collector.ndjson")
	cmd := exec.Command(binary,
		"--profile", "agents/collector/profile.yaml",
		"--core-root", coreRoot,
		"--directory", workDir,
	)
	cmd.Dir = catalogRoot
	cmd.Env = append(os.Environ(),
		"COLLECTOR_BIND_HOST=127.0.0.1",
		"COLLECTOR_RECEIVER_ADDRESS="+receiver,
		"COLLECTOR_CONTROL_PORT="+controlPort,
		"COLLECTOR_MONITOR_PORT="+monitorPort,
		"COLLECTOR_QUERY_PORT="+queryPort,
		"COLLECTOR_SPOOL_PATH="+spool,
		"COLLECTOR_MODE=spool",
	)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		return err
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()
	controlURL := "http://" + control
	if err := waitHTTPStatus(controlURL+"/api/lifecycle/health", http.StatusOK, 20*time.Second); err != nil {
		return fmt.Errorf("collector health: %w\n%s", err, output.String())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := grpc.NewClient(receiver, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if _, err := coltracepb.NewTraceServiceClient(conn).Export(ctx, &batch, grpc.WaitForReady(true)); err != nil {
		return fmt.Errorf("export canned batch: %w\n%s", err, output.String())
	}
	var spooled []byte
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		spooled, err = os.ReadFile(spool)
		if err == nil && len(bytes.TrimSpace(spooled)) > 0 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	lines := bytes.Split(bytes.TrimSpace(spooled), []byte("\n"))
	if len(lines) != scenario.ExpectedSpanCount {
		return fmt.Errorf("spooled spans = %d, want %d\n%s", len(lines), scenario.ExpectedSpanCount, output.String())
	}
	if !bytes.Contains(spooled, []byte(scenario.ExpectedService)) {
		return fmt.Errorf("spool does not preserve service %q", scenario.ExpectedService)
	}
	req, _ := http.NewRequest(http.MethodPost,
		controlURL+"/api/lifecycle/exit", strings.NewReader(`{"reason":"rig complete"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("stop collector: %w", err)
	}
	_ = resp.Body.Close()
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("collector exit: %w\n%s", err, output.String())
	}
	return nil
}

// collectorLifecycleResult holds the evidence from a collector lifecycle scenario.
type collectorLifecycleResult struct {
	TerminalState    string
	AllAddrsRebind   bool
	MonitorReachable bool
}

// runCollectorLifecycleScenario launches the collector, issues an exit, waits
// for the process to stop, then verifies that all listener addresses are free
// (rebind proof) and that the monitor reported a bounded terminal state before
// it stopped.
func runCollectorLifecycleScenario(binary, coreRoot, catalogRoot, workDir string) (*collectorLifecycleResult, error) {
	receiver, err := freeLoopbackAddr()
	if err != nil {
		return nil, err
	}
	control, err := freeLoopbackAddr()
	if err != nil {
		return nil, err
	}
	_, controlPort, err := net.SplitHostPort(control)
	if err != nil {
		return nil, err
	}
	monitor, err := freeLoopbackAddr()
	if err != nil {
		return nil, err
	}
	_, monitorPort, err := net.SplitHostPort(monitor)
	if err != nil {
		return nil, err
	}
	query, err := freeLoopbackAddr()
	if err != nil {
		return nil, err
	}
	_, queryPort, err := net.SplitHostPort(query)
	if err != nil {
		return nil, err
	}
	spool := filepath.Join(workDir, "collector.ndjson")
	cmd := exec.Command(binary,
		"--profile", "agents/collector/profile.yaml",
		"--core-root", coreRoot,
		"--directory", workDir,
	)
	cmd.Dir = catalogRoot
	cmd.Env = append(os.Environ(),
		"COLLECTOR_BIND_HOST=127.0.0.1",
		"COLLECTOR_RECEIVER_ADDRESS="+receiver,
		"COLLECTOR_CONTROL_PORT="+controlPort,
		"COLLECTOR_MONITOR_PORT="+monitorPort,
		"COLLECTOR_QUERY_PORT="+queryPort,
		"COLLECTOR_SPOOL_PATH="+spool,
		"COLLECTOR_MODE=spool",
	)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}()
	controlURL := "http://" + control
	monitorURL := "http://" + monitor
	if err := waitHTTPStatus(controlURL+"/api/lifecycle/health", http.StatusOK, 20*time.Second); err != nil {
		return nil, fmt.Errorf("collector health: %w\n%s", err, output.String())
	}

	// Verify monitor is reachable while running.
	monitorReachable := false
	resp, err := http.Get(monitorURL + "/monitor/state")
	if err == nil {
		_ = resp.Body.Close()
		monitorReachable = resp.StatusCode == http.StatusOK
	}

	// Issue exit.
	req, _ := http.NewRequest(http.MethodPost,
		controlURL+"/api/lifecycle/exit", strings.NewReader(`{"reason":"lifecycle-proof"}`))
	req.Header.Set("Content-Type", "application/json")
	exitResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stop collector: %w", err)
	}
	_ = exitResp.Body.Close()
	if err := cmd.Wait(); err != nil {
		return nil, fmt.Errorf("collector exit: %w\n%s", err, output.String())
	}

	// Extract terminal state from process output.
	terminalState := extractTerminalState(output.String())

	// Rebind proof: try to bind each address the collector held.
	allRebind := true
	for _, addr := range []string{receiver, control, monitor} {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			allRebind = false
			continue
		}
		_ = ln.Close()
	}

	return &collectorLifecycleResult{
		TerminalState:    terminalState,
		AllAddrsRebind:   allRebind,
		MonitorReachable: monitorReachable,
	}, nil
}

var terminalStatePattern = regexp.MustCompile(`terminal state: (\w+)`)

func extractTerminalState(output string) string {
	m := terminalStatePattern.FindStringSubmatch(output)
	if len(m) >= 2 {
		return m[1]
	}
	return ""
}

var rigVerdictSignal = regexp.MustCompile(`"command\.signal"[^}]*?"(Scenario(?:Passed|Failed))"`)

// rigVerdicts reads the per-scenario verdict signals from the trace file, in
// execution order, from the verdict-collecting words' spans.
func rigVerdicts(tracePath string) []string {
	data, err := os.ReadFile(tracePath)
	if err != nil {
		return nil
	}
	var verdicts []string
	for _, line := range bytes.Split(data, []byte("\n")) {
		if !bytes.Contains(line, []byte("collect_scenario_verdict")) && !bytes.Contains(line, []byte("fail_scenario")) {
			continue
		}
		if !bytes.Contains(line, []byte("execute_tool")) {
			continue
		}
		match := rigVerdictSignal.FindSubmatch(line)
		if match != nil {
			verdicts = append(verdicts, string(match[1]))
		}
	}
	return verdicts
}
