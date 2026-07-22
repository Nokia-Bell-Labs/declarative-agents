// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// The tests spawn the test binary itself as the child process rather than a
// built agent binary, so they stay hermetic. TestMain routes a child
// invocation to the requested behavior.

const (
	envChildMode = "SERVICE_TEST_CHILD"
	envChildAddr = "SERVICE_TEST_ADDR"
	envChildEcho = "SERVICE_TEST_ECHO"
)

func TestMain(m *testing.M) {
	switch os.Getenv(envChildMode) {
	case "serve":
		runChildServer()
		return
	case "exit0":
		os.Exit(0)
	case "exit3":
		os.Exit(3)
	case "hang":
		// A bare select{} would trip Go's deadlock detector and exit at once;
		// sleeping actually hangs, which is what the timeout path needs.
		time.Sleep(time.Hour)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runChildServer serves health until signalled, standing in for a serve-mode
// agent.
func runChildServer() {
	addr := os.Getenv(envChildAddr)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","echo":"` + os.Getenv(envChildEcho) + `"}`))
	})
	server := &http.Server{Addr: addr, Handler: mux}
	_ = server.ListenAndServe()
	os.Exit(0)
}

// childSpec builds a StartSpec that re-executes the test binary as a server.
func childSpec(t *testing.T, name, addr, echo string) StartSpec {
	t.Helper()
	return StartSpec{
		Name:    name,
		Binary:  os.Args[0],
		Profile: "unused-by-the-test-child",
		Address: addr,
		Env: []string{
			envChildMode + "=serve",
			envChildAddr + "=" + addr,
			envChildEcho + "=" + echo,
		},
	}
}

func processAlive(pid int) bool {
	// Signal 0 probes for existence without delivering anything.
	return syscall.Kill(pid, 0) == nil
}

// TestServiceChild_StartAwaitStopNoOrphans covers srd040 AC1: a serve-mode
// child starts with injected environment, reports healthy, stops, and leaves
// no process behind. A repeated cycle passes, so ports and state do not leak.
func TestServiceChild_StartAwaitStopNoOrphans(t *testing.T) {
	for cycle := 1; cycle <= 2; cycle++ {
		t.Run("cycle"+strconv.Itoa(cycle), func(t *testing.T) {
			state := NewState()
			addr, err := FreeAddress()
			require.NoError(t, err)

			started, err := state.Start(childSpec(t, "twin", addr, "cycle"))
			require.NoError(t, err)
			require.Equal(t, "twin", started["service"])
			require.Equal(t, "http://"+addr, started["base_url"])
			pid, ok := started["pid"].(int)
			require.True(t, ok)
			require.Equal(t, []string{"twin"}, state.Running())

			health, healthy := state.AwaitHealthy(started["base_url"].(string)+"/healthz", 10*time.Second, 20*time.Millisecond)
			require.True(t, healthy, "child should become healthy: %v", health)

			// The injected environment reached the child.
			resp, err := http.Get(started["base_url"].(string) + "/healthz")
			require.NoError(t, err)
			var body map[string]string
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			_ = resp.Body.Close()
			require.Equal(t, "cycle", body["echo"])

			stopped := state.Stop("twin", 2*time.Second)
			require.Equal(t, true, stopped["stopped"])
			require.Empty(t, state.Running())

			require.Eventually(t, func() bool { return !processAlive(pid) }, 3*time.Second, 20*time.Millisecond,
				"child process %d should be gone after stop", pid)
		})
	}
}

// TestServiceChild_StopIsIdempotentAndStopAllReaps covers srd040 R3.2 and
// R1.4: stopping an unknown service succeeds, and StopAll reaps the set.
func TestServiceChild_StopIsIdempotentAndStopAllReaps(t *testing.T) {
	state := NewState()

	out := state.Stop("never-started", time.Second)
	require.Equal(t, false, out["stopped"])
	require.Equal(t, "not running", out["reason"])

	var pids []int
	for _, name := range []string{"a", "b"} {
		addr, err := FreeAddress()
		require.NoError(t, err)
		started, err := state.Start(childSpec(t, name, addr, name))
		require.NoError(t, err)
		pids = append(pids, started["pid"].(int))
	}
	require.Equal(t, []string{"a", "b"}, state.Running())

	results := state.StopAll(2 * time.Second)
	require.Len(t, results, 2)
	require.Empty(t, state.Running())
	for _, pid := range pids {
		require.Eventually(t, func() bool { return !processAlive(pid) }, 3*time.Second, 20*time.Millisecond)
	}

	// Stopping again is a no-op rather than an error.
	require.Equal(t, false, state.Stop("a", time.Second)["stopped"])
}

// TestServiceChild_StartRejectsBadSpawn covers srd040 R6.3: a spawn failure is
// an error, not a panic, and a duplicate service name is rejected.
func TestServiceChild_StartRejectsBadSpawn(t *testing.T) {
	state := NewState()

	_, err := state.Start(StartSpec{Name: "x", Binary: "/nonexistent/binary", Profile: "p"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "start_service \"x\"")

	_, err = state.Start(StartSpec{Name: "", Profile: "p"})
	require.Error(t, err)
	_, err = state.Start(StartSpec{Name: "y"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires a profile")

	addr, err := FreeAddress()
	require.NoError(t, err)
	_, err = state.Start(childSpec(t, "dup", addr, ""))
	require.NoError(t, err)
	defer state.StopAll(time.Second)

	_, err = state.Start(childSpec(t, "dup", addr, ""))
	require.Error(t, err)
	require.Contains(t, err.Error(), "already running")
}

// TestServiceChild_AwaitHealthyTimeout covers srd040 AC2: an address that
// never answers returns the timeout outcome within its declared bound rather
// than polling without limit.
func TestServiceChild_AwaitHealthyTimeout(t *testing.T) {
	t.Parallel()

	addr, err := FreeAddress()
	require.NoError(t, err)

	state := NewState()
	start := time.Now()
	out, healthy := state.AwaitHealthy("http://"+addr+"/healthz", 300*time.Millisecond, 20*time.Millisecond)
	elapsed := time.Since(start)

	require.False(t, healthy)
	require.Equal(t, "300ms", out["timeout"])
	require.Less(t, elapsed, 5*time.Second, "must respect the declared bound, not poll without limit")
	require.GreaterOrEqual(t, out["attempts"].(int), 1)
}

// TestRunValidators_ConcurrentTerminalStatesAndTimeout covers srd040 AC3: each
// validator's terminal state is reported, keyed by name and sorted. The
// hung-validator path is covered by RunsConcurrently and the signals test.
func TestRunValidators_ConcurrentTerminalStatesAndTimeout(t *testing.T) {
	t.Parallel()

	// The budget must clear real child startup: a race-instrumented re-exec of
	// the test binary needs far longer than a trivial process would, and too
	// tight a bound makes the passing child look like a timeout.
	const budget = 30 * time.Second

	// No validator here hangs, so the generous budget costs nothing: the run
	// ends as soon as the children exit. The timeout path is proven by
	// TestRunValidators_RunsConcurrently and the signals test, which keep their
	// own short bounds.
	outcomes := RunValidators(context.Background(), os.Args[0], []ValidatorSpec{
		{Name: "passing", Profile: "p", Env: []string{envChildMode + "=exit0"}},
		{Name: "failing", Profile: "p", Env: []string{envChildMode + "=exit3"}},
	}, budget)

	require.Len(t, outcomes, 2)
	require.Equal(t, []string{"failing", "passing"},
		[]string{outcomes[0].Name, outcomes[1].Name}, "outcomes are sorted for determinism")

	byName := map[string]ValidatorOutcome{}
	for _, outcome := range outcomes {
		byName[outcome.Name] = outcome
	}
	require.True(t, byName["passing"].Passed)
	require.Equal(t, 0, byName["passing"].ExitCode)
	require.False(t, byName["failing"].Passed)
	require.Equal(t, 3, byName["failing"].ExitCode)

	require.False(t, AllPassed(outcomes))
	failure, failed := FirstFailure(outcomes)
	require.True(t, failed)
	require.Equal(t, "failing", failure.Name, "the first failure names the cause")

	allGood := RunValidators(context.Background(), os.Args[0],
		[]ValidatorSpec{{Name: "ok", Profile: "p", Env: []string{envChildMode + "=exit0"}}}, budget)
	require.True(t, AllPassed(allGood))
}

// TestRunValidators_RunsConcurrently proves validators run concurrently rather
// than in sequence: three children that all hang finish in about one timeout,
// not three (srd040 R4.2).
func TestRunValidators_RunsConcurrently(t *testing.T) {
	t.Parallel()

	const timeout = 1500 * time.Millisecond
	specs := make([]ValidatorSpec, 3)
	for i := range specs {
		specs[i] = ValidatorSpec{
			Name:    "hung" + strconv.Itoa(i),
			Profile: "p",
			Env:     []string{envChildMode + "=hang"},
		}
	}

	start := time.Now()
	outcomes := RunValidators(context.Background(), os.Args[0], specs, timeout)
	elapsed := time.Since(start)

	require.Len(t, outcomes, 3)
	for _, outcome := range outcomes {
		require.True(t, outcome.TimedOut, "%s should time out", outcome.Name)
	}
	require.Less(t, elapsed, 3*timeout, "concurrent execution should cost about one timeout, not three")
}

// TestListScenarios_DeterministicDiscovery covers srd040 AC4: discovery
// returns the expected entries, sorted, with validators and fixtures, and
// repeated runs are identical.
func TestListScenarios_DeterministicDiscovery(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mk := func(parts ...string) string {
		dir := filepath.Join(append([]string{root}, parts...)...)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		return dir
	}
	write := func(dir, name string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644))
	}

	// subject-b sorts after subject-a; scenario names sort within a subject.
	happy := mk("subject-a", "tests", "happy-path")
	write(happy, machineFileName)
	write(happy, profileFileName)
	write(mk("subject-a", "tests", "happy-path", "mocks"), "dep.yaml")

	failure := mk("subject-a", "tests", "dep-failure")
	write(failure, machineFileName)
	write(failure, profileFileName)

	multi := mk("subject-b", "tests", "multi")
	write(multi, machineFileName)
	write(mk("subject-b", "tests", "multi", "second"), profileFileName)

	// Ignored: a plain agent with no tests/, and a tests/ entry with no machine.
	mk("subject-c")
	mk("subject-a", "tests", "not-a-scenario")

	scenarios, err := ListScenarios([]string{root, filepath.Join(root, "does-not-exist")})
	require.NoError(t, err)
	require.Len(t, scenarios, 3)

	require.Equal(t, "dep-failure", scenarios[0].Name)
	require.Equal(t, "subject-a", scenarios[0].Subject)
	require.Equal(t, "happy-path", scenarios[1].Name)
	require.Equal(t, "multi", scenarios[2].Name)
	require.Equal(t, "subject-b", scenarios[2].Subject)

	require.Len(t, scenarios[1].Fixtures, 1)
	require.Equal(t, "dep.yaml", filepath.Base(scenarios[1].Fixtures[0]))
	require.Empty(t, scenarios[0].Fixtures)

	// A nested directory holding a profile is an additional validator.
	require.Len(t, scenarios[2].Validators, 1)
	require.Equal(t, "second", filepath.Base(filepath.Dir(scenarios[2].Validators[0])))

	repeat, err := ListScenarios([]string{root})
	require.NoError(t, err)
	require.Equal(t, scenarios, repeat, "discovery is deterministic")
}

// TestServiceTools_DeclarationsReversibilityAndUndo covers srd040 AC5: only
// start_service is reversible, its undo stops the service, every other word's
// undo is a noop, and config validation rejects incomplete declarations.
func TestServiceTools_DeclarationsReversibilityAndUndo(t *testing.T) {
	state := NewState()

	addr, err := FreeAddress()
	require.NoError(t, err)
	startCmd := Builder{
		ToolName: "start_twin", Init: InitStartService, State: state,
		Config: ToolConfig{
			Service: "twin", Profile: "p", Binary: os.Args[0], Address: addr,
			Env: []string{envChildMode + "=serve", envChildAddr + "=" + addr},
		},
	}.Build(core.Result{})

	result := startCmd.Execute()
	require.Equal(t, SignalServiceStarted, result.Signal)
	require.Equal(t, []string{"twin"}, state.Running())

	// start_service is reversible: its undo stops what it started.
	undo := startCmd.Undo(result)
	require.Equal(t, SignalServiceStopped, undo.Signal)
	require.Empty(t, state.Running(), "undo must stop the started service")

	// Every other word is a noop undo, matching its declaration.
	for _, init := range []string{InitAwaitHealthy, InitStopService, InitRunValidators, InitListScenarios} {
		cmd := Builder{ToolName: init, Init: init, State: state}.Build(core.Result{})
		require.Equal(t, core.NoopUndo(init).Signal, cmd.Undo(core.Result{}).Signal, init)
	}

	// Incomplete declarations are rejected at build time.
	br := toolregistry.NewBuiltinRegistry()
	RegisterBuiltins(br, FactoryDeps{State: state})
	for init, want := range map[string]string{
		InitStartService:  "requires a service name",
		InitAwaitHealthy:  "requires a url",
		InitStopService:   "requires a service name",
		InitRunValidators: "requires at least one validator",
		InitListScenarios: "requires at least one root",
	} {
		factory, ok := br.Resolve(init)
		require.True(t, ok, "init %s should be registered", init)
		cfg := map[string]interface{}{}
		if init == InitStartService {
			cfg["profile"] = "p" // present, but service name missing
		}
		_, err := factory(catalog.ToolDef{Name: init, Config: cfg}, nil)
		require.Error(t, err, init)
		require.Contains(t, err.Error(), want, init)
	}
}

// TestServiceCommand_AwaitHealthySignals covers srd040 R2.3: the healthy and
// timeout outcomes are distinct signals a machine can route on.
func TestServiceCommand_AwaitHealthySignals(t *testing.T) {
	state := NewState()
	addr, err := FreeAddress()
	require.NoError(t, err)

	_, err = state.Start(childSpec(t, "svc", addr, ""))
	require.NoError(t, err)
	defer state.StopAll(2 * time.Second)

	healthy := Builder{
		ToolName: "await", Init: InitAwaitHealthy, State: state,
		Config: ToolConfig{URL: "http://" + addr + "/healthz", Timeout: "10s", Interval: "20ms"},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalHealthy, healthy.Signal)

	dead, err := FreeAddress()
	require.NoError(t, err)
	timedOut := Builder{
		ToolName: "await", Init: InitAwaitHealthy, State: state,
		Config: ToolConfig{URL: "http://" + dead + "/healthz", Timeout: "200ms", Interval: "20ms"},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalHealthTimeout, timedOut.Signal)
}

// TestServiceCommand_RunValidatorsSignals covers srd040 R4.5: a completed run
// and an incomplete one carry different signals, and the payload names the
// first failure.
func TestServiceCommand_RunValidatorsSignals(t *testing.T) {
	t.Parallel()

	run := func(specs []ValidatorSpec, timeout string) core.Result {
		return Builder{
			ToolName: "run", Init: InitRunValidators, State: NewState(),
			Config: ToolConfig{Binary: os.Args[0], Validators: specs, Timeout: timeout},
		}.Build(core.Result{}).Execute()
	}

	passed := run([]ValidatorSpec{{Name: "ok", Profile: "p", Env: []string{envChildMode + "=exit0"}}}, "1m")
	require.Equal(t, SignalValidatorsCompleted, passed.Signal)
	require.Contains(t, passed.Output, `"passed":true`)

	// A validator that ran and failed still completed: the rig reads the verdict.
	failed := run([]ValidatorSpec{{Name: "bad", Profile: "p", Env: []string{envChildMode + "=exit3"}}}, "1m")
	require.Equal(t, SignalValidatorsCompleted, failed.Signal)
	require.Contains(t, failed.Output, `"passed":false`)
	require.Contains(t, failed.Output, `"first_failure"`)

	// A validator that never finished did not complete.
	hung := run([]ValidatorSpec{{Name: "hung", Profile: "p", Env: []string{envChildMode + "=hang"}}}, "300ms")
	require.Equal(t, SignalValidatorsIncomplete, hung.Signal)
	require.Contains(t, hung.Output, `"timed_out":true`)
}

// TestServiceCommand_ListScenariosOutput covers the discovery word's result
// shape, which the rig machine routes on.
func TestServiceCommand_ListScenariosOutput(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "subject", "tests", "only")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, machineFileName), []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, profileFileName), []byte("{}"), 0o644))

	result := Builder{
		ToolName: "list", Init: InitListScenarios, State: NewState(),
		Config: ToolConfig{Roots: []string{root}},
	}.Build(core.Result{}).Execute()

	require.Equal(t, SignalScenariosListed, result.Signal)
	var payload struct {
		Count     int        `json:"count"`
		Scenarios []Scenario `json:"scenarios"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &payload))
	require.Equal(t, 1, payload.Count)
	require.Equal(t, "only", payload.Scenarios[0].Name)
	require.Equal(t, "subject", payload.Scenarios[0].Subject)
	require.Len(t, payload.Scenarios[0].Validators, 1)
}

// TestServiceCommand_UnsupportedInit guards the dispatch default.
func TestServiceCommand_UnsupportedInit(t *testing.T) {
	t.Parallel()

	result := Builder{ToolName: "x", Init: "not_a_service_word", State: NewState()}.
		Build(core.Result{}).Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "unsupported service init")
}

// httpGetBody fetches a URL and returns its body, used to confirm a child
// observed the environment it was started with.
func httpGetBody(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(data)
}
