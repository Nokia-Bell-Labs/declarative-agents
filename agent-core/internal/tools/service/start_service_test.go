// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package service

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func listed(t *testing.T, state *State, name string) map[string]interface{} {
	t.Helper()
	for _, item := range state.List() {
		if item["service"] == name {
			return item
		}
	}
	return nil
}

// TestStartService_DetachedReturnsAndListsExit covers srd040 R7.1 and R7.4:
// the word returns while its child still runs, list_services reports it
// running and then exited with its exit code, and stopping the exited child
// succeeds without a signal.
func TestStartService_DetachedReturnsAndListsExit(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: spawns a real OS child")
	}
	state := NewState()
	t.Cleanup(func() { state.Reap() })
	cmd := Builder{
		ToolName: "launch_evaluator", Init: InitStartService, State: state,
		Config: ToolConfig{
			Profile: "agents/critic/profile.yaml", Binary: os.Args[0],
			ServiceFrom: "$from(seed).request_id", RequestFrom: "$from(seed).parameters.suite",
			Env: []string{envChildMode + "=exit3slow"},
		},
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(labeledStateView{
		label:  "seed",
		output: `{"request_id":"run-1","parameters":{"suite":"suites/basic.yaml"}}`,
	})

	started := cmd.Execute()
	require.Equal(t, SignalServiceStarted, started.Signal, started.Output)
	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(started.Output), &out))
	require.Equal(t, "run-1", out["service"])
	require.Equal(t, "running", listed(t, state, "run-1")["status"])

	list := Builder{ToolName: "list", Init: InitListServices, State: state}.Build(core.Result{})
	require.Eventually(t, func() bool {
		return listed(t, state, "run-1")["status"] == "exited"
	}, 10*time.Second, 20*time.Millisecond)
	result := list.Execute()
	require.Equal(t, SignalChildrenListed, result.Signal)
	var body struct {
		Services []map[string]interface{} `json:"services"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &body))
	require.Len(t, body.Services, 1)
	require.Equal(t, "exited", body.Services[0]["status"])
	require.EqualValues(t, 3, body.Services[0]["exit_code"])
	require.NotEmpty(t, body.Services[0]["finished_at"])
	require.Zero(t, state.RunningCount())
	require.Empty(t, state.Running())

	stopped := state.Stop("run-1", time.Second)
	require.Equal(t, true, stopped["stopped"])
	require.Equal(t, true, stopped["exited"])
	require.NotContains(t, stopped, "signal")
	require.Empty(t, state.List())
}

// TestStartService_MaxRunningEmitsLimitReached covers srd040 R7.3: at the
// declared bound the word starts nothing and reports the limit, and the bound
// holds when concurrent request runs race for the last slot.
func TestStartService_MaxRunningEmitsLimitReached(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: spawns a real OS child")
	}
	state := NewState()
	t.Cleanup(func() { state.Reap() })
	start := func(name string) core.Result {
		return Builder{
			ToolName: "launch", Init: InitStartService, State: state,
			Config: ToolConfig{
				Profile: "p", Service: name, Binary: os.Args[0], MaxRunning: 1,
				Env: []string{envChildMode + "=hang"},
			},
		}.Build(core.Result{}).Execute()
	}
	require.Equal(t, SignalServiceStarted, start("first").Signal)
	limited := start("second")
	require.Equal(t, SignalServiceLimitReached, limited.Signal, limited.Output)
	require.Contains(t, limited.Output, `"max_running":1`)
	require.Equal(t, 1, state.RunningCount())
	require.Nil(t, listed(t, state, "second"))

	state.Reap()
	var wg sync.WaitGroup
	var mu sync.Mutex
	started, refused := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := state.Start(StartSpec{
				Name: "racer-" + string(rune('a'+i)), Binary: os.Args[0], Profile: "p",
				Env: []string{envChildMode + "=hang"}, MaxRunning: 2,
			})
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				started++
			case errors.Is(err, ErrLimitReached):
				refused++
			default:
				t.Errorf("unexpected start error: %v", err)
			}
		}(i)
	}
	wg.Wait()
	require.Equal(t, 2, started)
	require.Equal(t, 6, refused)
	require.Equal(t, 2, state.RunningCount())
}

// TestStartService_DerivedNameAndOutputFlag covers the undeclared name and the
// --output flag a detached child still receives.
func TestStartService_DerivedNameAndOutputFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("integration-grade: spawns a real OS child")
	}
	at := time.Unix(0, 42)
	require.Equal(t, "critic-42", derivedServiceName("agents/critic/profile.yaml", at))
	require.Equal(t, "bench-42", derivedServiceName("agents/bench.yaml", at))

	spec := childProcessSpec(StartSpec{Binary: "agent", Profile: "p.yaml", Request: "s.yaml", Output: "eval-results"})
	require.Contains(t, strings.Join(spec.Args, " "), "--request s.yaml --output eval-results")

	state := NewState()
	t.Cleanup(func() { state.Reap() })
	result := Builder{
		ToolName: "launch", Init: InitStartService, State: state, ChildAgentBinary: os.Args[0],
		Config: ToolConfig{Profile: "agents/critic/profile.yaml", Env: []string{envChildMode + "=hang"}},
	}.Build(core.Result{}).Execute()
	require.Equal(t, SignalServiceStarted, result.Signal, result.Output)
	require.Regexp(t, `"service":"critic-\d+"`, result.Output)
}
