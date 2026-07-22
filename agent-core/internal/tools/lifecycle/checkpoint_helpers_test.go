// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"encoding/json"
	"errors"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

// fakeReverter is an in-memory CheckpointReverter for lifecycle tests. Revert
// truncates the stored Execution to the target step, mirroring the Dolt adapter
// resetting DB state, and records the calls it received.
type fakeReverter struct {
	pos        core.Position
	execution  core.Execution
	reverted   []int
	runID      string
	failRevert bool
}

func (f *fakeReverter) Save(p core.Position, e core.Execution) error {
	f.pos = p
	f.execution = append(core.Execution(nil), e...)
	return nil
}

func (f *fakeReverter) Load() (core.Position, core.Execution, error) {
	if f.execution == nil {
		return core.Position{}, nil, core.ErrNoCheckpoint
	}
	return f.pos, append(core.Execution(nil), f.execution...), nil
}

func (f *fakeReverter) Revert(runID string, step int) error {
	if f.failRevert {
		return errors.New("revert boom")
	}
	f.runID = runID
	f.reverted = append(f.reverted, step)
	if step+1 <= len(f.execution) {
		f.execution = f.execution[:step+1]
	}
	return nil
}

var _ core.CheckpointReverter = (*fakeReverter)(nil)

func lifecycleRESTCollection(t *testing.T, baseURL string) (toolrest.Collection, toolrest.ClientOperationDefinition) {
	t.Helper()
	def := toolrest.Definition{
		Version: "v1",
		Auth:    map[string]toolrest.AuthProfile{"none": {Type: "none"}},
		Clients: map[string]toolrest.Client{"github": {
			BaseURL: baseURL,
			AuthRef: "none",
			Resources: map[string]toolrest.Resource{"issue": {
				Path: "/repos/{owner}/{repo}/issues/{number}",
				Operations: map[string]toolrest.Operation{
					"set": {
						Method: http.MethodPatch,
						Params: toolrest.RequestBinding{
							Path: map[string]interface{}{"owner": map[string]interface{}{}, "repo": map[string]interface{}{}, "number": map[string]interface{}{}},
							BodySchema: map[string]interface{}{
								"type":       "object",
								"properties": map[string]interface{}{"title": map[string]interface{}{"type": "string"}},
							},
						},
						Body:          map[string]interface{}{"title": "{{ params.title }}"},
						Success:       toolrest.StatusMapping{Status: []int{200}, Signal: "RESTResourceWritten"},
						Response:      toolrest.ResponseMapping{ResourceID: "$.id"},
						SideEffects:   []toolrest.SideEffect{{Kind: "external_api", State: "issue_updated"}},
						Reversibility: toolrest.Reversibility{Classification: "compensatable", Undo: "restore"},
						Compensation: map[string]interface{}{
							"operation":  "set",
							"parameters": map[string]interface{}{"title": "restored"},
						},
					},
				},
			}},
		}},
	}
	collection := toolrest.NewCollection()
	require.NoError(t, collection.Add(def))
	operation, err := collection.ResolveClientOperation(toolrest.ClientToolConfig{
		RestRef: "github", Resource: "issue", Operation: "set",
	})
	require.NoError(t, err)
	return collection, operation
}

func lifecycleReverterWithRESTReceipt(t *testing.T) *fakeReverter {
	t.Helper()
	receipt := undo.EncodeBoundaryReceipt(undo.BoundaryCompensationPayload{
		BoundaryCompensation: undo.BoundaryCompensation{
			Strategy: "restore",
			RestRef:  "github", Resource: "issue", Operation: "set",
			Parameters:   map[string]interface{}{"owner": "acme", "repo": "agent-core", "number": "1", "title": "new"},
			ResourceID:   "1",
			Compensation: map[string]interface{}{"operation": "set", "parameters": map[string]interface{}{"title": "restored"}},
		},
	})
	require.NotEmpty(t, receipt)
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "rest_set_issue", Signal: core.Signal("RESTResourceWritten"), Receipt: receipt},
	}))
	return rev
}

func writeLifecycleJSON(t *testing.T, w http.ResponseWriter, status int, payload map[string]interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	require.NoError(t, json.NewEncoder(w).Encode(payload))
}

type undoTrackingBuilder struct {
	calls *int
}

func (b *undoTrackingBuilder) Build(core.Result) core.Command {
	return &undoTrackingCommand{calls: b.calls}
}

func (b *undoTrackingBuilder) BuildReverser() core.Command {
	return &undoTrackingCommand{calls: b.calls}
}

type undoTrackingCommand struct {
	calls *int
}

func (c *undoTrackingCommand) Name() string { return "write" }

func (c *undoTrackingCommand) Execute() core.Result { return core.Result{Signal: core.ToolDone} }

func (c *undoTrackingCommand) Undo(core.Result) core.Result {
	*c.calls = *c.calls + 1
	return core.Result{Signal: core.ToolDone}
}
