// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// The tests spawn the test binary itself as the child process rather than a
// built agent binary, so they stay hermetic. TestMain routes a child
// invocation to the requested behavior.

const (
	envChildMode = "SERVICE_TEST_CHILD"
	envChildAddr = "SERVICE_TEST_ADDR"
	envChildEcho = "SERVICE_TEST_ECHO"
	envChildPID  = "SERVICE_TEST_GRANDCHILD_PID"
)

func TestMain(m *testing.M) {
	switch os.Getenv(envChildMode) {
	case "serve":
		runChildServer()
		return
	case "exit0":
		// Real agents report their terminal status on stderr; the rig reads it
		// because the binary's exit code does not distinguish success from a
		// failed terminal state.
		fmt.Fprintln(os.Stderr, "terminal state: succeeded")
		os.Exit(0)
	case "exit3":
		fmt.Fprintln(os.Stderr, "terminal state: failed")
		os.Exit(3)
	case "exit0silent":
		// Exits zero and reports nothing: proved nothing.
		os.Exit(0)
	case "exit0failed":
		// A failed terminal now exits non-zero, matching the binary (srd018 R6).
		fmt.Fprintln(os.Stderr, "terminal state: failed")
		os.Exit(2)
	case "expect-otlp":
		for i, arg := range os.Args {
			if arg == "--otel-otlp-endpoint" && i+1 < len(os.Args) && os.Args[i+1] == "127.0.0.1:4317" {
				fmt.Fprintln(os.Stderr, "terminal state: succeeded")
				os.Exit(0)
			}
		}
		fmt.Fprintln(os.Stderr, "missing validator OTLP endpoint")
		os.Exit(2)
	case "hang":
		// A bare select{} would trip Go's deadlock detector and exit at once;
		// sleeping actually hangs, which is what the timeout path needs.
		time.Sleep(time.Hour)
		os.Exit(0)
	case "tree":
		grandchild := exec.Command("sh", "-c", "sleep 3600")
		if err := grandchild.Start(); err != nil {
			os.Exit(2)
		}
		_ = os.WriteFile(os.Getenv(envChildPID), []byte(strconv.Itoa(grandchild.Process.Pid)), 0o600)
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

func TestChildCommandPropagatesCoreRootInArgv(t *testing.T) {
	spec := childProcessSpec(StartSpec{
		Binary: "agent", Profile: "agents/mock/profile.yaml",
		CoreRoot: "/checkout/agent-core",
	})

	require.Equal(t, []string{
		"--profile", "agents/mock/profile.yaml",
		"--core-root", "/checkout/agent-core",
	}, spec.Args)
	require.Equal(t, "agent", spec.Binary)
}

// TestServiceChild_StartStopNoOrphans covers srd040 AC1: a serve-mode child
// starts with injected environment, stops, and leaves no process behind. A
// repeated cycle passes, so ports and state do not leak.
func TestServiceChild_StartStopNoOrphans(t *testing.T) {
	for cycle := 1; cycle <= 2; cycle++ {
		t.Run("cycle"+strconv.Itoa(cycle), func(t *testing.T) {
			state := NewState()
			addr, err := FreeAddress()
			require.NoError(t, err)

			started, err := state.Start(childSpec(t, "mock", addr, "cycle"))
			require.NoError(t, err)
			require.Equal(t, "mock", started["service"])
			require.Equal(t, "http://"+addr, started["base_url"])
			pid, ok := started["pid"].(int)
			require.True(t, ok)
			require.Equal(t, []string{"mock"}, state.Running())

			// Test-only readiness keeps the service boundary free of HTTP.
			var resp *http.Response
			require.Eventually(t, func() bool {
				resp, err = http.Get(started["base_url"].(string) + "/healthz")
				return err == nil
			}, 10*time.Second, 20*time.Millisecond)
			require.NoError(t, err)
			var body map[string]string
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
			_ = resp.Body.Close()
			require.Equal(t, "cycle", body["echo"])

			stopped := state.Stop("mock", 2*time.Second)
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

func TestServiceChild_ParentCancellationKillsProcessGroup(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	state := NewStateWithContext(ctx)
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	spec := StartSpec{
		Name: "tree", Binary: os.Args[0], Profile: "unused",
		Env: []string{envChildMode + "=tree", envChildPID + "=" + pidFile},
	}
	started, err := state.Start(spec)
	require.NoError(t, err)
	childPID := started["pid"].(int)
	var grandchildPID int
	require.Eventually(t, func() bool {
		data, readErr := os.ReadFile(pidFile)
		if readErr != nil {
			return false
		}
		grandchildPID, readErr = strconv.Atoi(string(data))
		return readErr == nil
	}, 3*time.Second, 20*time.Millisecond)

	cancel()

	require.Eventually(t, func() bool {
		return !processAlive(childPID) && !processAlive(grandchildPID)
	}, 3*time.Second, 20*time.Millisecond)
	state.Stop("tree", time.Second)
}

// TestServiceChild_StartRejectsBadSpawn covers srd040 R6.3: a spawn failure is
// an error, not a panic, and a duplicate service name is rejected.
func TestServiceChild_StartRejectsBadSpawn(t *testing.T) {
	state := NewState()

	_, err := state.Start(StartSpec{Name: "x", Binary: "/nonexistent/binary", Profile: "p"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "start child \"x\"")

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

// TestRunOneValidator_TerminalStateAndTimeout covers the atomic child-execution
// boundary used by MachineSpec iteration.
func TestRunOneValidator_TerminalStateAndTimeout(t *testing.T) {
	t.Parallel()

	passed := runOneValidator(t.Context(), os.Args[0], ValidatorSpec{
		Name: "passing", Profile: "p", Env: []string{envChildMode + "=exit0"},
	}, 30*time.Second)
	require.True(t, passed.Passed)
	require.Equal(t, "succeeded", passed.Terminal)

	timedOut := runOneValidator(t.Context(), os.Args[0], ValidatorSpec{
		Name: "hung", Profile: "p", Env: []string{envChildMode + "=hang"},
	}, 300*time.Millisecond)
	require.True(t, timedOut.TimedOut)
	require.False(t, timedOut.Passed)
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

func TestStopServiceSelectorResolutionFailure(t *testing.T) {
	t.Parallel()

	cmd := Builder{
		ToolName: "stop_child", Init: InitStopService, State: NewState(),
		Config: ToolConfig{Service: "$from(child).service"},
	}.Build(core.Result{})
	aware, ok := cmd.(core.CommandStateAware)
	require.True(t, ok)
	aware.SetCommandState(labeledStateView{label: "another_step", output: `{}`})

	result := cmd.Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorContains(t, result.Err, `selector "$from(child).service"`)
	require.ErrorContains(t, result.Err, `no prior step labeled "child"`)
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
	var resp *http.Response
	var err error
	require.Eventually(t, func() bool {
		resp, err = http.Get(url)
		return err == nil
	}, 10*time.Second, 20*time.Millisecond)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(data)
}
