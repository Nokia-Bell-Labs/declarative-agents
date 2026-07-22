// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
	"time"
)

func TestRESTAwaitEvent_MultiSourceFanIn(t *testing.T) {
	t.Parallel()

	state := NewServerState()
	_, _ = launchRESTServerWithState(t, state, namedControlServer("first"), LimitProfile{})
	defer stopRESTServer(t, state, "first")
	_, secondURL := launchRESTServerWithState(t, state, namedControlServer("second"), LimitProfile{})
	defer stopRESTServer(t, state, "second")

	postStatus(t, secondURL+"/approve/123", `{}`, http.StatusAccepted)
	event, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{Server: "first"}, {Server: "second"}},
		Timeout: time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, "Approved", signal)
	require.Equal(t, "second", event.Source)
	require.Equal(t, "approve", event.Route)
}

func TestRESTAwaitEvent_SourceFiltersPreserveUnrelatedEvents(t *testing.T) {
	t.Parallel()

	state, baseURL := launchRESTServer(t, controlServer(), LimitProfile{})
	defer stopRESTServer(t, state, "control")

	postStatus(t, baseURL+"/domain?signal=DomainEventReceived", `{}`, http.StatusAccepted)
	postStatus(t, baseURL+"/approve/123", `{}`, http.StatusAccepted)
	event, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{
			Server: "control", Routes: []string{"approve"}, Signals: []string{"Approved"},
		}},
		Timeout: time.Second,
	})
	require.NoError(t, err)
	require.Equal(t, "Approved", signal)
	require.Equal(t, "approve", event.Route)

	preserved, preservedSignal, err := state.Await("control")
	require.NoError(t, err)
	require.Equal(t, "DomainEventReceived", preservedSignal)
	require.Equal(t, "domain", preserved.Route)
}

func TestRESTAwaitEvent_Timeout(t *testing.T) {
	t.Parallel()

	state, _ := launchRESTServer(t, namedControlServer("timeout"), LimitProfile{})
	defer stopRESTServer(t, state, "timeout")

	_, signal, err := state.AwaitAny(AwaitAnyOptions{
		Sources: []AwaitSource{{Server: "timeout"}}, Timeout: 10 * time.Millisecond,
	})
	require.NoError(t, err)
	require.Equal(t, "AwaitTimedOut", signal)
}

func TestRESTAwaitEvent_ServerStopped(t *testing.T) {
	t.Parallel()

	state, _ := launchRESTServer(t, namedControlServer("stopped"), LimitProfile{})
	results := startRESTAwait(t, func() core.Result {
		return awaitAnyResult(state, AwaitSource{Server: "stopped"})
	})
	requireAwaitBlocked(t, results)
	stopRESTServer(t, state, "stopped")
	require.Equal(t, core.Signal("ServerStopped"), requireRESTResult(t, results).Signal)
}

func TestRESTAwaitEvent_StoppedSourceCommandError(t *testing.T) {
	t.Parallel()

	state, _ := launchRESTServer(t, namedControlServer("stopped_error"), LimitProfile{})
	source := AwaitSource{Server: "stopped_error", StoppedBehavior: StoppedSourceCommandError}
	results := startRESTAwait(t, func() core.Result { return awaitAnyResult(state, source) })
	requireAwaitBlocked(t, results)
	stopRESTServer(t, state, "stopped_error")
	require.Equal(t, core.Signal("CommandError"), requireRESTResult(t, results).Signal)
}

func TestRESTAwaitEvent_FactoryBuildsConfiguredCommand(t *testing.T) {
	t.Parallel()

	state := NewServerState()
	collection := NewCollection()
	require.NoError(t, collection.Add(Definition{Servers: map[string]Server{"control": controlServer()}}))
	_, baseURL := launchRESTServerWithState(t, state, controlServer(), LimitProfile{})
	defer stopRESTServer(t, state, "control")

	def := requireRESTToolDef(t, InitAwaitEvent)
	def.Config = map[string]interface{}{"sources": []interface{}{
		map[string]interface{}{"server": "control", "routes": []interface{}{"approve"}},
	}}
	command := requireRESTCommand(t, def, collection, state)
	postStatus(t, baseURL+"/approve/123", `{}`, http.StatusAccepted)
	result := command.Execute()

	require.Equal(t, core.Signal("Approved"), result.Signal, result.Output)
	require.Contains(t, result.Output, `"source":"control"`)
	require.Contains(t, result.Output, `"route":"approve"`)
	require.Equal(t, core.ToolDone, command.Undo(core.Result{}).Signal)
}

func TestRESTAwaitEvent_RejectsUnsupportedReadPolicy(t *testing.T) {
	t.Parallel()

	requireUnsupportedReadPolicyRejected(t)
}

func TestRESTAwaitEvent_FactoryBuildsStagedFanIn(t *testing.T) {
	t.Parallel()

	state := NewServerState()
	collection := stagedFanInCollection(t)
	launchRESTServerCommand(t, collection, state, "first")
	defer stopRESTServer(t, state, "first")
	secondURL := launchRESTServerCommand(t, collection, state, "second")
	defer stopRESTServer(t, state, "second")

	postStatus(t, secondURL+"/approve/123", `{}`, http.StatusAccepted)
	firstAwait := awaitEventCommand(t, collection, state, "first", "second")
	result := firstAwait.Execute()
	requireAwaitEventOutput(t, result, "second", "SecondApproved")
	require.Equal(t, core.ToolDone, firstAwait.Undo(core.Result{}).Signal)

	thirdURL := launchRESTServerCommand(t, collection, state, "third")
	defer stopRESTServer(t, state, "third")
	postStatus(t, thirdURL+"/approve/456", `{}`, http.StatusAccepted)
	secondAwait := awaitEventCommand(t, collection, state, "first", "second", "third")
	result = secondAwait.Execute()
	requireAwaitEventOutput(t, result, "third", "ThirdApproved")
	require.Equal(t, core.ToolDone, secondAwait.Undo(core.Result{}).Signal)
}
