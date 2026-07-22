// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// These cover the terminal-state response mapping (srd030 R4.3; GH-615). The
// machine here has the shape an exec-driven request machine has: every word
// emits only ToolDone or ToolFailed, so two different failures reach two
// different terminal states through the same signal. Keyed by signal that is
// one response; keyed by state it is two.

// stageWords is a two-stage pipeline whose outcome is chosen per stage. It
// stands in for exec words without running subprocesses, emitting exactly the
// signals exec words can emit (exec/subprocess.go SubprocessResult).
type stageWords struct {
	name    string
	succeed bool
}

func (w stageWords) Build(core.Result) core.Command { return w }

func (w stageWords) Name() string { return w.name }

func (w stageWords) Undo(core.Result) core.Result { return core.NoopUndo(w.Name()) }

func (w stageWords) Execute() core.Result {
	if w.succeed {
		return core.Result{Signal: core.ToolDone, CommandName: w.name, Output: `{"stage":"` + w.name + `"}`}
	}
	return core.Result{Signal: core.ToolFailed, CommandName: w.name, Output: `{"stage":"` + w.name + `"}`}
}

// applyShapedMachine mirrors examples/chatbot-mesh/agents/executor/apply-machine.yaml:
// a validate stage whose failure is the caller's fault, and an apply stage whose
// failure is the server's. Both failures arrive as ToolFailed.
func applyShapedMachine() *core.MachineSpec {
	return &core.MachineSpec{
		Name:           "apply-shaped",
		InitialState:   "Start",
		States:         core.StateSpecsFromNames("Start", "Validating", "Applying", "Done", "Rejected", "Failed"),
		TerminalStates: []string{"Done", "Rejected", "Failed"},
		Signals:        core.SignalSpecsFromNames("Seed", string(core.ToolDone), string(core.ToolFailed)),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "Seed", Next: "Validating", Action: "validate"},
			{State: "Validating", Signal: string(core.ToolDone), Next: "Applying", Action: "apply"},
			{State: "Validating", Signal: string(core.ToolFailed), Next: "Rejected"},
			{State: "Applying", Signal: string(core.ToolDone), Next: "Done"},
			{State: "Applying", Signal: string(core.ToolFailed), Next: "Failed"},
		},
	}
}

func applyShapedConfig(validateOK, applyOK bool) MachineRequest {
	return MachineRequest{
		Timeout:     "2s",
		Request:     MachineRequestMapping{Body: map[string]string{"name": "$.name"}},
		MachineSpec: applyShapedMachine(),
		Response: MachineRequestResponse{TerminalStates: map[string]MachineResponseMapping{
			"Done":     {Status: http.StatusOK, Body: map[string]string{"status": "$.stage"}},
			"Rejected": {Status: http.StatusBadRequest, Body: map[string]string{"status": "$.stage"}},
			"Failed":   {Status: http.StatusInternalServerError, Body: map[string]string{"status": "$.stage"}},
		}},
		InitFunc: func(reg *core.Registry) error {
			reg.Register(core.ToolSpec{Name: "validate"}, stageWords{name: "validate", succeed: validateOK})
			reg.Register(core.ToolSpec{Name: "apply"}, stageWords{name: "apply", succeed: applyOK})
			return nil
		},
	}
}

// TestMachineRequestTerminalStateSeparatesClientAndServerErrors is the GH-615
// acceptance case. Before terminal-state mapping the reject and the apply
// failure were indistinguishable over HTTP: both terminate on ToolFailed, so
// both got whichever single status ToolFailed was mapped to.
func TestMachineRequestTerminalStateSeparatesClientAndServerErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		validateOK bool
		applyOK    bool
		wantStatus int
		wantRun    core.RunStatus
		wantStage  string
	}{
		{name: "applied", validateOK: true, applyOK: true, wantStatus: http.StatusOK, wantRun: core.StatusSucceeded, wantStage: "apply"},
		{name: "validate reject is a client error", validateOK: false, wantStatus: http.StatusBadRequest, wantRun: core.StatusFailed, wantStage: "validate"},
		{name: "apply failure is a server error", validateOK: true, applyOK: false, wantStatus: http.StatusInternalServerError, wantRun: core.StatusFailed, wantStage: "apply"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			state, baseURL := launchMachineRequestServerWithConfig(t, applyShapedConfig(tt.validateOK, tt.applyOK))
			defer stopRESTServer(t, state, "machine")

			body := postJSON(t, baseURL+"/docs", `{"name":"patch"}`, tt.wantStatus)
			require.Equal(t, tt.wantStage, body["status"])

			trace := body["trace"].(map[string]interface{})
			// Both failures carry the same terminal signal; only the state differs.
			require.Equal(t, string(core.ToolFailed) == trace["terminal_signal"], !tt.validateOK || !tt.applyOK)
			require.Equal(t, string(tt.wantRun), trace["status"])
		})
	}
}

// TestMachineRequestTerminalStateOutranksSignal proves precedence. A config
// carrying both maps resolves by state, so a machine can adopt state keying
// without first deleting the signal keys it already ships.
func TestMachineRequestTerminalStateOutranksSignal(t *testing.T) {
	t.Parallel()
	cfg := applyShapedConfig(false, false)
	cfg.Response.TerminalSignals = map[string]MachineResponseMapping{
		string(core.ToolFailed): {Status: http.StatusTeapot, Body: map[string]string{"status": "$.stage"}},
	}
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg)
	defer stopRESTServer(t, state, "machine")

	postJSON(t, baseURL+"/docs", `{"name":"patch"}`, http.StatusBadRequest)
}

// TestMachineRequestFallsBackToSignalMapping keeps the existing contract: a
// config with no terminal_states resolves by signal exactly as before.
func TestMachineRequestFallsBackToSignalMapping(t *testing.T) {
	t.Parallel()
	cfg := applyShapedConfig(false, false)
	cfg.Response.TerminalStates = nil
	cfg.Response.TerminalSignals = map[string]MachineResponseMapping{
		string(core.ToolFailed): {Status: http.StatusConflict, Body: map[string]string{"status": "$.stage"}},
	}
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg)
	defer stopRESTServer(t, state, "machine")

	postJSON(t, baseURL+"/docs", `{"name":"patch"}`, http.StatusConflict)
}

// TestMachineRequestUnmappedTerminalNamesBothKeys checks the diagnostic. A run
// that ends somewhere neither map covers must say what it looked up, since the
// author's mistake is as likely to be the state as the signal.
func TestMachineRequestUnmappedTerminalNamesBothKeys(t *testing.T) {
	t.Parallel()
	cfg := applyShapedConfig(false, false)
	delete(cfg.Response.TerminalStates, "Rejected")
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg)
	defer stopRESTServer(t, state, "machine")

	body := postJSON(t, baseURL+"/docs", `{"name":"patch"}`, http.StatusBadGateway)
	message, _ := body["error"].(string)
	require.Contains(t, message, "response_missing")
	require.Contains(t, message, "Rejected")
	require.Contains(t, message, string(core.ToolFailed))
}

// TestValidateMachineResponsesRejectsNonTerminalState keeps dead configuration
// out: a mapping onto a state the machine never terminates in can never fire,
// so it fails at load rather than surfacing as a response_missing per request.
func TestValidateMachineResponsesRejectsNonTerminalState(t *testing.T) {
	t.Parallel()
	machine := *applyShapedMachine()

	err := validateMachineResponses(machine, MachineRequestResponse{
		TerminalStates: map[string]MachineResponseMapping{"Applying": {Status: http.StatusOK}},
	})
	require.ErrorContains(t, err, "machine_config_invalid")
	require.ErrorContains(t, err, "Applying")

	require.NoError(t, validateMachineResponses(machine, MachineRequestResponse{
		TerminalStates: map[string]MachineResponseMapping{"Rejected": {Status: http.StatusBadRequest}},
	}))
}

// TestMachineRequestEndpointAcceptsEitherResponseMap covers the config gate:
// one of the two maps is required, and either alone is enough.
func TestMachineRequestEndpointAcceptsEitherResponseMap(t *testing.T) {
	t.Parallel()
	endpoint := func(response MachineRequestResponse) Endpoint {
		return Endpoint{
			Method: "POST", Path: "/docs", Binding: bindingMachineRequest,
			MachineRequest: MachineRequest{Machine: "m.yaml", Timeout: "2s", Response: response},
		}
	}
	mapping := map[string]MachineResponseMapping{"Done": {Status: http.StatusOK}}

	require.NoError(t, validateMachineRequestEndpoint("docs", endpoint(MachineRequestResponse{TerminalStates: mapping})))
	require.NoError(t, validateMachineRequestEndpoint("docs", endpoint(MachineRequestResponse{TerminalSignals: mapping})))

	err := validateMachineRequestEndpoint("docs", endpoint(MachineRequestResponse{}))
	require.ErrorContains(t, err, "terminal_states or terminal_signals")
}
