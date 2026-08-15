// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func appliedDigest(output string) core.ResultDigest {
	return core.ResultDigest{
		Signal:           core.ToolDone,
		Output:           output,
		RedactionVersion: core.OutputRedactionVersion1,
		RedactionStatus:  core.OutputRedactionApplied,
	}
}

func commandStateSource(entries ...core.Entry) *core.LiveCommandStateSource {
	source := core.NewLiveCommandStateSource()
	source.ObserveCommandState(core.Execution(entries))
	return source
}

// commandStateServer builds a server whose one read route exposes a command_state
// view over the declared labels, with an optional response-size bound.
func commandStateServer(name string, maxBytes int, labels []string) ServerDefinition {
	server := Server{
		Address: "127.0.0.1:0",
		Queue:   QueueConfig{Name: name, Capacity: 8, Timeout: "20ms"},
		Endpoints: map[string]Endpoint{
			"monitor_fleet": {
				Method: "GET", Path: "/monitor/fleet",
				Binding: bindingReadState, MonitorView: monitorViewCommandState, Labels: labels,
			},
		},
	}
	return ServerDefinition{
		Name:   name,
		Server: server,
		Limits: LimitProfile{MaxResponseBytes: maxBytes},
	}
}

func TestMonitorREST_CommandStateView(t *testing.T) {
	t.Parallel()

	def := commandStateServer("fleet", 0, []string{"polled_fleet_step", "list_mesh_deployments", "unrecorded_label"})
	def.Monitor = MonitorState{CommandState: commandStateSource(
		core.Entry{
			CommandName: "polled_fleet_step", Label: "polled_fleet_step",
			ToState: "Polling", Signal: core.ToolDone, Iteration: 4,
			Receipt: "opaque-receipt", Result: appliedDigest(`{"agents":[{"name":"chatbot","reachable":true}]}`),
		},
		core.Entry{
			CommandName: "list_mesh_deployments", ToState: "ListingDeployments", Signal: core.ToolDone, Iteration: 2,
			Result: appliedDigest(`{"deployments":[{"name":"chatbot"}]}`),
		},
	)}

	state := NewServerState()
	_, baseURL := launchRESTServerDefinition(t, state, def)

	body := requestBody(t, http.MethodGet, baseURL+"/monitor/fleet", "", http.StatusOK)
	require.NotContains(t, body, "opaque-receipt")
	require.NotContains(t, body, "receipt")

	labels := getJSON(t, baseURL+"/monitor/fleet")["labels"].(map[string]interface{})

	poll := labels["polled_fleet_step"].(map[string]interface{})
	require.Equal(t, true, poll["available"])
	require.Equal(t, "Polling", poll["state"])
	require.Equal(t, string(core.ToolDone), poll["signal"])
	require.Equal(t, float64(4), poll["iteration"])
	agents := poll["output"].(map[string]interface{})["agents"].([]interface{})
	require.Len(t, agents, 1)
	require.Equal(t, "chatbot", agents[0].(map[string]interface{})["name"])

	require.Contains(t, labels, "unrecorded_label")
	require.Nil(t, labels["unrecorded_label"])
}

func TestMonitorCommandStateView_ValidationRejectsMisuse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		ep      Endpoint
		wantErr string
	}{
		{
			"command_state without labels",
			Endpoint{Binding: bindingReadState, MonitorView: monitorViewCommandState},
			"non-empty labels allowlist",
		},
		{
			"command_state with empty labels",
			Endpoint{Binding: bindingReadState, MonitorView: monitorViewCommandState, Labels: []string{}},
			"non-empty labels allowlist",
		},
		{
			"labels on current_state",
			Endpoint{Binding: bindingReadState, MonitorView: monitorViewState, Labels: []string{"x"}},
			"only valid with monitor_view command_state",
		},
		{
			"labels without any view",
			Endpoint{Binding: bindingReadState, Labels: []string{"x"}},
			"only valid with monitor_view command_state",
		},
		{
			"command_state on stream binding",
			Endpoint{Binding: bindingStreamEvents, MonitorView: monitorViewCommandState, Labels: []string{"x"}},
			"requires read_state binding",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMonitorView("endpoint", tc.ep)
			require.Error(t, err)
			require.Contains(t, err.Error(), tc.wantErr)
		})
	}

	valid := Endpoint{Binding: bindingReadState, MonitorView: monitorViewCommandState, Labels: []string{"polled_step"}}
	require.NoError(t, validateMonitorView("endpoint", valid))
}

func TestMonitorREST_CommandStateRedaction(t *testing.T) {
	t.Parallel()

	big := `{"pods":["` + string(make([]byte, 128)) + `"]}`
	def := commandStateServer("fleet_redaction", 64, []string{"ok_step", "omitted_step", "legacy_step", "big_step"})
	def.Monitor = MonitorState{CommandState: commandStateSource(
		core.Entry{CommandName: "ok_step", ToState: "S", Signal: core.ToolDone, Result: appliedDigest(`{"n":1}`)},
		core.Entry{CommandName: "omitted_step", ToState: "S", Signal: core.ToolDone, Result: core.ResultDigest{
			RedactionVersion: core.OutputRedactionVersion1, RedactionStatus: core.OutputRedactionOmitted,
		}},
		core.Entry{CommandName: "legacy_step", ToState: "S", Signal: core.ToolDone, Result: core.ResultDigest{
			Output: `{"secret":"x"}`,
		}},
		core.Entry{CommandName: "big_step", ToState: "S", Signal: core.ToolDone, Result: appliedDigest(big)},
	)}

	state := NewServerState()
	_, baseURL := launchRESTServerDefinition(t, state, def)
	labels := getJSON(t, baseURL+"/monitor/fleet")["labels"].(map[string]interface{})

	require.Equal(t, true, labels["ok_step"].(map[string]interface{})["available"])

	omitted := labels["omitted_step"].(map[string]interface{})
	require.Equal(t, false, omitted["available"])
	require.Equal(t, "output_unavailable", omitted["reason"])

	legacy := labels["legacy_step"].(map[string]interface{})
	require.Equal(t, false, legacy["available"])
	require.NotContains(t, requestBody(t, http.MethodGet, baseURL+"/monitor/fleet", "", http.StatusOK), "secret")

	big2 := labels["big_step"].(map[string]interface{})
	require.Equal(t, false, big2["available"])
	require.Equal(t, "exceeds_response_limit", big2["reason"])
}
