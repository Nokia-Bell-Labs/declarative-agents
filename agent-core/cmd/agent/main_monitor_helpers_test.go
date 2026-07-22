// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func requireMainWiresMonitorRecorder(t *testing.T) {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(repoRootFromTest(t), "cmd", "agent", "main.go"))
	require.NoError(t, err)
	require.Contains(t, string(source), "MonitorRecorder: monitorRuntime.Recorder")
}

type loopResult struct {
	result core.RunResult
	err    error
}

func monitorRuntimeMachine() core.MachineSpec {
	return core.MachineSpec{
		Name:         "monitor-runtime-test",
		MetricLabels: core.MetricLabels{"profile": "monitor"},
		InitialState: "Idle",
		States: core.StateSpecs{
			{Name: "Idle"}, {Name: "Working"}, {Name: "Done"},
		},
		TerminalStates: []string{"Done"},
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Working", Action: "run"},
			{State: "Working", Signal: "ToolDone", Next: "Done"},
		},
	}
}

func runMonitorRuntimeLoop(t *testing.T, runtime monitorRuntime) core.RunResult {
	t.Helper()
	reg := core.NewRegistry()
	registerStaticSignal(reg, "run", core.ToolDone, "ok", "")
	machine := monitorRuntimeMachine()
	result, err := core.Loop(core.LoopParams{
		MachineSpec:     &machine,
		AgentName:       "agent",
		Registry:        reg,
		Trace:           tracing.NoopTracer{},
		MonitorRecorder: runtime.Recorder,
	}, context.Background())
	require.NoError(t, err)
	return result
}

type monitorProof struct {
	params         core.LoopParams
	monitor        monitorRuntime
	monitorState   toolrest.MonitorState
	restDefs       toolrest.Collection
	launchDef      catalog.ToolDef
	metricReader   *sdkmetric.ManualReader
	monitorBaseURL string
}

func monitorReleaseProof(t *testing.T) monitorProof {
	t.Helper()
	clearAgentFlags()
	root := repoRootFromTest(t)
	profilePath, baseURL := isolatedMonitorProfile(t, profileRootFromTest(t))
	flagProfile = profilePath
	flagDirectory = root

	cfg, err := loadRuntimeConfig()
	require.NoError(t, err)
	defs, err := loadProfileToolDefs(cfg)
	require.NoError(t, err)
	restDefs, err := toolrest.LoadDefinitions(cfg.RestDefinitions, cfg.RestConfigDirs)
	require.NoError(t, err)
	machine, err := core.LoadMachineSpec(cfg.Machine)
	require.NoError(t, err)
	require.NoError(t, catalog.ValidateToolEmits(machine, defs))

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	mon := newMonitorRuntime(machine, defs, restDefs, provider.Meter("agent"))
	require.NotNil(t, mon.Store)
	require.NotNil(t, mon.Recorder)

	reg := core.NewRegistry()
	builtins := toolregistry.NewBuiltinRegistry()
	st := monitorProofAgentState(cfg, reg, mon, &machine, defs, restDefs)
	registerBuiltinFactories(builtins, st, selectedBuiltinInits(defs))
	vars := map[string]string{"directory": cfg.Directory, "request": cfg.Request}
	require.NoError(t, toolregistry.RegisterUnifiedTools(reg, builtins, cfg.Directory, defs, vars, execBuilder))

	return monitorProof{
		params: core.LoopParams{
			MachineSpec:     &machine,
			AgentName:       "agent",
			Registry:        reg,
			Trace:           tracing.NoopTracer{},
			Budget:          machine.BudgetSpec.ToBudget(core.Budget{MaxIterations: 2}),
			MonitorRecorder: mon.Recorder,
		},
		monitor:        mon,
		monitorState:   monitorState(mon.Store, &machine, defs),
		restDefs:       restDefs,
		launchDef:      requireToolDef(t, defs, "launch_monitor_rest"),
		metricReader:   reader,
		monitorBaseURL: baseURL,
	}
}

func monitorProofAgentState(
	cfg runtimeConfig,
	reg *core.Registry,
	mon monitorRuntime,
	machine *core.MachineSpec,
	defs []catalog.ToolDef,
	restDefs toolrest.Collection,
) *agentState {
	return &agentState{
		registry:  reg,
		tracer:    tracing.NoopTracer{},
		ctx:       context.Background(),
		directory: cfg.Directory,
		request:   cfg.Request,
		monitor:   monitorState(mon.Store, machine, defs),
		restDefs:  restDefs,
		shutdown:  func() {},
	}
}

func isolatedMonitorProfile(t *testing.T, profileRoot string) (string, string) {
	t.Helper()
	dir := t.TempDir()
	address := isolatedMonitorAddress(t)
	writeIsolatedMonitorREST(t, profileRoot, dir, address)
	profilePath := filepath.Join(dir, "profile.yaml")
	profile := fmt.Sprintf(`name: monitor
machine: %s
tools:
  - %s
tool_declarations:
  - %s
rest_definitions:
  - %s
`, profilePathFromRoot(profileRoot, "monitor/machine.yaml"),
		profilePathFromRoot(profileRoot, "monitor/tools.yaml"),
		profilePathFromRoot(profileRoot, "monitor/declarations.yaml"),
		filepath.Join(dir, "rest.yaml"))
	require.NoError(t, os.WriteFile(profilePath, []byte(profile), 0o644))
	return profilePath, "http://" + address
}

func writeIsolatedMonitorREST(t *testing.T, profileRoot, dir, address string) {
	t.Helper()
	data, err := os.ReadFile(profilePathFromRoot(profileRoot, "monitor/rest.yaml"))
	require.NoError(t, err)
	replaced := monitorRESTWithAddress(t, string(data), address)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "rest.yaml"), []byte(replaced), 0o644))
}

func monitorRESTWithAddress(t *testing.T, data string, address string) string {
	t.Helper()
	lines := strings.Split(data, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "address: ") {
			continue
		}
		prefix := line[:strings.Index(line, "address: ")]
		lines[i] = prefix + "address: " + address
		return strings.Join(lines, "\n")
	}
	require.FailNow(t, "monitor REST config should declare server address")
	return ""
}

func isolatedMonitorAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}

func launchProofMonitorREST(t *testing.T, proof monitorProof) (*toolrest.ServerState, string) {
	t.Helper()
	state := toolrest.NewServerState()
	br := toolregistry.NewBuiltinRegistry()
	toolrest.RegisterFactories(br, toolrest.FactoryDeps{
		Definitions:        proof.restDefs,
		ServerState:        state,
		Monitor:            proof.monitorState,
		CredentialResolver: toolrest.EmptyCredentialResolver{},
	})
	factory, ok := br.Resolve(toolrest.InitServerLaunch)
	require.True(t, ok)
	builder, err := factory(proof.launchDef, nil)
	require.NoError(t, err)
	result := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.Signal("ServerLaunched"), result.Signal, result.Output)
	return state, "http://" + proofLaunchAddress(t, result.Output)
}

func proofLaunchAddress(t *testing.T, output string) string {
	t.Helper()
	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(output), &decoded))
	address, ok := decoded["address"].(string)
	require.True(t, ok, "launch output should include address")
	return address
}

func proofRequestBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	var body bytes.Buffer
	_, err = body.ReadFrom(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, body.String())
	return body.String()
}

func waitForProofMonitorRoute(t *testing.T, url string) {
	t.Helper()
	client := http.Client{Timeout: 100 * time.Millisecond}
	require.Eventually(t, func() bool {
		resp, err := client.Get(url)
		if err != nil {
			return false
		}
		defer func() { _ = resp.Body.Close() }()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 10*time.Millisecond)
}

func postProofMonitorExit(t *testing.T, url string) {
	t.Helper()
	resp, err := http.Post(url, "application/json", strings.NewReader(`{"reason":"test"}`))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusAccepted, resp.StatusCode)
}

func receiveLoopResult(t *testing.T, resultCh <-chan loopResult) loopResult {
	t.Helper()
	select {
	case outcome := <-resultCh:
		return outcome
	case <-time.After(2 * time.Second):
		require.FailNow(t, "monitor loop did not exit after control request")
	}
	return loopResult{}
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func startMonitorAgentProcess(t *testing.T, root string, profilePath string) (*exec.Cmd, *lockedBuffer, *lockedBuffer) {
	t.Helper()
	cmd := exec.Command("go", "run", "./cmd/agent", "--profile", profilePath, "--directory", root, "--core-root", root)
	cmd.Dir = root
	stdout := &lockedBuffer{}
	stderr := &lockedBuffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	})
	return cmd, stdout, stderr
}

func waitForMonitorBaseURL(t *testing.T, stderr *lockedBuffer) string {
	t.Helper()
	var baseURL string
	require.Eventually(t, func() bool {
		baseURL = monitorBaseURLFromOutput(stderr.String())
		return baseURL != ""
	}, 5*time.Second, 10*time.Millisecond)
	return baseURL
}

func monitorBaseURLFromOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		address, ok := strings.CutPrefix(line, "monitor address: ")
		if ok && strings.TrimSpace(address) != "" {
			return "http://" + strings.TrimSpace(address)
		}
	}
	return ""
}

func waitForProcess(t *testing.T, cmd *exec.Cmd) <-chan error {
	t.Helper()
	resultCh := make(chan error, 1)
	go func() { resultCh <- cmd.Wait() }()
	return resultCh
}

func requireProcessStillRunning(t *testing.T, resultCh <-chan error) {
	t.Helper()
	select {
	case err := <-resultCh:
		require.Failf(t, "monitor process exited early", "err=%v", err)
	default:
	}
}

func requireProcessSucceeded(t *testing.T, resultCh <-chan error, stdout, stderr *lockedBuffer) {
	t.Helper()
	select {
	case err := <-resultCh:
		require.NoError(t, err, "stdout=%s stderr=%s", stdout.String(), stderr.String())
	case <-time.After(5 * time.Second):
		require.FailNow(t, "monitor process did not exit after control request")
	}
	require.Contains(t, stderr.String(), "terminal state: succeeded")
}

func requireMonitorSample(t *testing.T, samples []monitor.MetricSample, name string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name {
			return
		}
	}
	require.Failf(t, "missing monitor sample", "sample %q not found in %#v", name, samples)
}

func requireMonitorSampleAttribute(t *testing.T, samples []monitor.MetricSample, name, key, value string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && sample.Attributes[key] == value {
			return
		}
	}
	require.Failf(t, "missing monitor sample attribute", "sample %q missing %s=%s in %#v", name, key, value, samples)
}

func requireMetricData(t *testing.T, data metricdata.ResourceMetrics, name string) {
	t.Helper()
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name == name {
				return
			}
		}
	}
	require.Failf(t, "missing OTel metric", "metric %q not found in %#v", name, data)
}
