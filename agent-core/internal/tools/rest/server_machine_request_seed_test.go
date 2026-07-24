// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	exectool "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
)

// TestMachineRequestSeedFeedsRESTClientFirstWord proves a REST-client word can be
// the first word of a machine_request request machine: the seed exposes the
// mapped request input under "parameters", so runtimeParams reads it and the
// runtime-input authority guard does not see a transport-metadata "method"
// (srd030 seed-parameters contract). The seeded query vector threads into the
// outbound request body.
func TestMachineRequestSeedFeedsRESTClientFirstWord(t *testing.T) {
	t.Parallel()

	var gotBody map[string]interface{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = json.NewDecoder(req.Body).Decode(&gotBody)
		writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
	}))
	defer srv.Close()

	def := Definition{
		Version: "v1",
		Auth:    map[string]AuthProfile{"none": {Type: authNone}},
		Limits:  map[string]LimitProfile{"test": {}},
		Clients: map[string]Client{
			"chroma": {
				BaseURL: srv.URL, AuthRef: "none", LimitsRef: "test",
				Operations: map[string]Operation{
					"query": {
						Method: http.MethodPost,
						Path:   "/query",
						Params: RequestBinding{
							BodySchema:   objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
							BodySource:   bodySourcePreviousResult,
							InputMapping: map[string]string{"query_embeddings": "$.query_embeddings"},
						},
						Body:          map[string]interface{}{"query_embeddings": []interface{}{"{{ params.query_embeddings }}"}},
						Success:       StatusMapping{Status: []int{200}, Signal: "QueryResponded"},
						Response:      ResponseMapping{Output: map[string]string{"ok": "$.ok"}},
						SideEffects:   []SideEffect{{Kind: "external_api", State: "read_only"}},
						Reversibility: Reversibility{Classification: "reversible", Undo: "noop"},
					},
				},
			},
		},
	}
	require.NoError(t, ValidateDefinition(def))
	op := resolveThreadingOp(t, def, "chroma", "query")

	// The seed a machine_request produces for the first word: the HTTP method is
	// POST (a forbidden runtime-authority field name), but the seed exposes only
	// the mapped parameters, so a REST-client word consumes it without error.
	seed := requestSeed(MachineRequestRun{
		Server:  "chroma_rag_requests",
		Route:   "query",
		Method:  http.MethodPost,
		Path:    "/api/v1/rag/query",
		Payload: map[string]interface{}{"query_embeddings": []interface{}{0.1, 0.2, 0.3}},
	}, "Seed")

	result := threadingCommand(op, seed).Execute()

	require.Equal(t, core.Signal("QueryResponded"), result.Signal, result.Output)
	require.NotContains(t, result.Output, "cannot set REST authority")
	require.Equal(t, []interface{}{[]interface{}{0.1, 0.2, 0.3}}, gotBody["query_embeddings"])
}

// TestRequestSeedExposesParametersNotTransportMetadata pins the seed shape: the
// mapped request input is under "parameters" and the transport metadata is not
// present, so a request-machine word never sees transport authority.
func TestRequestSeedExposesParametersNotTransportMetadata(t *testing.T) {
	t.Parallel()

	seed := requestSeed(MachineRequestRun{
		Server:  "s",
		Route:   "r",
		Method:  http.MethodPost,
		Path:    "/p",
		Payload: map[string]interface{}{"name": "alice", "path": "docs/VISION.yaml"},
	}, "Seed")

	var out map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(seed.Output), &out))

	params, ok := out["parameters"].(map[string]interface{})
	require.True(t, ok, "seed exposes a parameters object")
	require.Equal(t, "alice", params["name"])
	require.Equal(t, "docs/VISION.yaml", params["path"])

	// The forbidden transport authority is absent; the non-authority URL path is
	// kept for adapters. The seed passes the runtime-input authority guard.
	require.NotContains(t, out, "method")
	require.NotContains(t, out, "server")
	require.NotContains(t, out, "route")
	require.NoError(t, ValidateRuntimeInput(out), "the seed passes the runtime-input authority guard")
}

func TestMachineRequestSeedCommandStateAddress(t *testing.T) {
	t.Parallel()

	cfg := MachineRequest{
		MachineSpec: &core.MachineSpec{
			Name:           "seed-exec-source",
			InitialState:   "Start",
			States:         core.StateSpecsFromNames("Start", "Counted", "Running", "Done"),
			TerminalStates: []string{"Done"},
			Signals:        core.SignalSpecsFromNames(string(core.Seed), string(core.ToolDone)),
			Transitions: []core.TransitionSpec{
				{State: "Start", Signal: string(core.Seed), Next: "Counted", Action: "count_before", Label: "count_before"},
				{State: "Counted", Signal: string(core.ToolDone), Next: "Running", Action: "run_corpus_ingest"},
				{State: "Running", Signal: string(core.ToolDone), Next: "Done"},
			},
		},
		Budget: core.Budget{MaxIterations: 3},
		InitFunc: func(reg *core.Registry) error {
			reg.Register(
				core.ToolSpec{Name: "count_before", Visibility: core.Internal},
				seedInterveningBuilder{},
			)
			exectool.RegisterToolDefs(reg, t.TempDir(), []catalog.ToolDef{{
				Name:   "run_corpus_ingest",
				Type:   "exec",
				Binary: "printf",
				Args:   []string{"%s"},
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"directory": map[string]interface{}{
							"type": "string", "positional": true,
							"source": "$from(seed).parameters.directory",
						},
					},
					"required": []interface{}{"directory"},
				},
			}})
			return nil
		},
	}

	result, err := (defaultMachineRequestRunner{}).RunMachineRequest(
		context.Background(),
		MachineRequestRun{
			Payload: map[string]interface{}{"directory": "/tmp/corpus"},
			Config:  cfg,
		},
	)

	require.NoError(t, err)
	require.Equal(t, "/tmp/corpus", result.Output["output"])

	seed := requestSeed(MachineRequestRun{
		Payload: map[string]interface{}{"directory": "/tmp/corpus"},
	}, core.Seed)
	fresh := machineRequestSeedExecution(seed, nil)
	require.Len(t, fresh, 1)
	require.Equal(t, "seed", fresh[0].Label)
	require.Equal(t, seed.Output, fresh[0].Result.Output)

	resumed := append(fresh, core.Entry{CommandName: "count_before", Label: "count_before"})
	require.Len(t, machineRequestSeedExecution(seed, resumed), 2)
	require.Equal(t, 1, countExecutionLabel(resumed, "seed"))
}

func TestMachineRequestSeedRedactionLiveAndReloaded(t *testing.T) {
	t.Parallel()

	mapping := MachineRequestMapping{
		Body: map[string]string{
			"directory": "$.directory",
			"token":     "$.token",
		},
		Sensitive: []string{"token"},
	}
	seed := requestSeed(MachineRequestRun{
		Payload: map[string]interface{}{
			"directory": "/tmp/corpus",
			"token":     "request-secret",
		},
		Config: MachineRequest{Request: mapping},
	}, core.Seed)
	require.Equal(t, []core.OutputRedactionPath{{"parameters", "token"}}, seed.Redaction.Paths)

	fresh := machineRequestSeedExecution(seed, nil)
	require.Len(t, fresh, 1)
	require.NotContains(t, fresh[0].Result.Output, "request-secret")
	require.NotContains(t, fresh[0].Result.Output, `"token"`)
	require.Equal(t, core.OutputRedactionApplied, fresh[0].Result.RedactionStatus)
	assertSeedSelection(t, fresh, "$from(seed).parameters.directory", "/tmp/corpus")
	assertSeedPathMissing(t, fresh, "$from(seed).parameters.token")

	checkpoint := &core.InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(core.Position{}, fresh))
	_, loaded, err := checkpoint.Load()
	require.NoError(t, err)
	require.NotContains(t, loaded[0].Result.Output, "request-secret")
	assertSeedSelection(t, loaded, "$from(seed).parameters.directory", "/tmp/corpus")
	assertSeedPathMissing(t, loaded, "$from(seed).parameters.token")

	resumed := append(loaded, core.Entry{CommandName: "count_before", Label: "count_before"})
	require.Len(t, machineRequestSeedExecution(seed, resumed), 2)
	require.Equal(t, 1, countExecutionLabel(resumed, "seed"))
}

func TestMachineRequestSeedRedactionFailsClosed(t *testing.T) {
	t.Parallel()

	seed := requestSeed(MachineRequestRun{
		Payload: map[string]interface{}{"token": "request-secret"},
	}, core.Seed)
	seed.Redaction.Paths = []core.OutputRedactionPath{{"parameters", ""}}

	execution := machineRequestSeedExecution(seed, nil)
	require.Empty(t, execution[0].Result.Output)
	require.Equal(t, core.OutputRedactionOmitted, execution[0].Result.RedactionStatus)
	_, err := core.ResolveFromSelector(
		core.NewCommandStateView(execution),
		"$from(seed).parameters.token",
	)
	var unavailable *core.CommandStateOutputUnavailableError
	require.True(t, errors.As(err, &unavailable), err)
}

func TestMachineRequestSensitiveFieldsMustBeMapped(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateMachineRequestSensitiveFields(MachineRequestMapping{
		Body:      map[string]string{"token": "$.token"},
		Sensitive: []string{"token"},
	}))
	require.ErrorContains(t, validateMachineRequestSensitiveFields(MachineRequestMapping{
		Sensitive: []string{"token"},
	}), "not a mapped request field")
	require.ErrorContains(t, validateMachineRequestSensitiveFields(MachineRequestMapping{
		Body:      map[string]string{"token": "$.token"},
		Sensitive: []string{"token", "token"},
	}), "duplicated")
}

func assertSeedSelection(
	t *testing.T,
	execution core.Execution,
	selector string,
	want interface{},
) {
	t.Helper()
	got, err := core.ResolveFromSelector(core.NewCommandStateView(execution), selector)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func assertSeedPathMissing(t *testing.T, execution core.Execution, selector string) {
	t.Helper()
	got, err := core.ResolveFromSelector(core.NewCommandStateView(execution), selector)
	require.Nil(t, got)
	var missing *core.UnresolvedPathError
	require.True(t, errors.As(err, &missing), err)
}

type seedInterveningBuilder struct{}

func (seedInterveningBuilder) Build(core.Result) core.Command {
	return seedInterveningCommand{}
}

type seedInterveningCommand struct{}

func (seedInterveningCommand) Name() string { return "count_before" }
func (seedInterveningCommand) Execute() core.Result {
	return core.Result{
		Signal:      core.ToolDone,
		CommandName: "count_before",
		Output:      `{"mapped":{"count":"4"}}`,
	}
}
func (seedInterveningCommand) Undo(core.Result) core.Result {
	return core.NoopUndo("count_before")
}

func countExecutionLabel(execution core.Execution, label string) int {
	var count int
	for _, entry := range execution {
		if entry.Label == label {
			count++
		}
	}
	return count
}

// plainTextRespondBuilder emits a raw (non-JSON) string, as invoke_llm does.
type plainTextRespondBuilder struct{ signal core.Signal }

func (b plainTextRespondBuilder) Build(_ core.Result) core.Command {
	return plainTextRespondCommand(b)
}

type plainTextRespondCommand struct{ signal core.Signal }

func (c plainTextRespondCommand) Name() string { return "respond" }
func (c plainTextRespondCommand) Execute() core.Result {
	return core.Result{Signal: c.signal, Output: "I am a plain text answer."}
}
func (c plainTextRespondCommand) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

// TestRESTServerMachineRequestWrapsPlainTextTerminalOutput proves a terminal word
// that emits plain text rather than a JSON object (as invoke_llm does) yields the
// configured 200 response with the text mapped under $.output, not a 502
// (srd030 R4.3).
func TestRESTServerMachineRequestWrapsPlainTextTerminalOutput(t *testing.T) {
	t.Parallel()
	cfg := machineRequestConfig("DocumentationReady", 0, false)
	cfg.Response.TerminalSignals["DocumentationReady"] = MachineResponseMapping{Status: 200, Body: map[string]string{"answer": "$.output"}}
	cfg.InitFunc = func(reg *core.Registry) error {
		reg.Register(core.ToolSpec{Name: "respond"}, plainTextRespondBuilder{signal: "DocumentationReady"})
		return nil
	}
	state, baseURL := launchMachineRequestServerWithConfig(t, cfg, catchAllDocsEndpoint(cfg))
	defer stopRESTServer(t, state, "machine")

	body := getJSON(t, baseURL+"/docs/VISION.yaml")
	require.Equal(t, "I am a plain text answer.", body["answer"])
}
