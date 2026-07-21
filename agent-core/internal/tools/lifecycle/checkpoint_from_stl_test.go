// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/filesystem"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
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

func TestCheckpointHistoryExecuteFormatsExecutionLog(t *testing.T) {
	t.Parallel()
	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(
		core.Position{
			CurrentState: "Working",
			LastSignal:   core.ToolDone,
			Snapshot:     core.AgentSnapshot{State: "Working", Signal: core.ToolDone, Iteration: 2},
		},
		core.Execution{
			{Iteration: 1, CommandName: "read", FromState: "Idle", ToState: "Reading", Signal: core.ToolDone},
			{Iteration: 2, CommandName: "write", FromState: "Reading", ToState: "Working", Signal: core.ToolDone, Receipt: `{"path":"a.txt"}`},
		},
	))

	cmd := (&CheckpointHistoryBuilder{Checkpoint: cp}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, "checkpoint_history", res.CommandName)

	// Output is the structured checkpoint-history schema {run, history} (#493).
	var out struct {
		Run     string `json:"run"`
		History string `json:"history"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &out))
	require.Equal(t, "latest", out.Run)
	require.Contains(t, out.History, "state: Working")
	require.Contains(t, out.History, "step=0  iteration=1  read  Idle -> Reading  signal=ToolDone")
	require.Contains(t, out.History, "step=1  iteration=2  write  Reading -> Working  signal=ToolDone  reversible")
}

func TestCheckpointHistoryExecuteRequiresCheckpoint(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires a Checkpoint")
}

func TestCheckpointHistoryExecuteReportsNoCheckpoint(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{Checkpoint: &core.InMemoryCheckpoint{}}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "no checkpoint persisted")
}

func TestCheckpointHistoryUndoIsNoop(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointHistoryBuilder{}).Build(core.Result{})
	require.Equal(t, core.ToolDone, cmd.Undo(core.Result{}).Signal)
}

func TestCheckpointRollbackRequiresRevertibleCheckpoint(t *testing.T) {
	t.Parallel()
	target := 1
	cmd := (&CheckpointRollbackBuilder{
		Config: catalog.CheckpointRollbackConfig{ToIteration: &target},
	}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires a revertible Checkpoint backend")
}

func TestCheckpointRollbackRequiresTargetIteration(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointRollbackBuilder{Checkpoint: &fakeReverter{}}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Output, "requires to_iteration")
}

func TestCheckpointRollbackRevertsDBStateToTargetStep(t *testing.T) {
	t.Parallel()
	target := 1
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", FromState: "Idle", ToState: "Reading", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "write", FromState: "Reading", ToState: "Working", Signal: core.ToolDone},
	}))

	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &target},
		Checkpoint: rev,
		RunID:      "run-1",
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, []int{0}, rev.reverted)
	require.Equal(t, "run-1", rev.runID)
	require.Contains(t, res.Output, "rolled back run run-1 to iteration 1 (step 0)")
	require.Contains(t, res.Output, "step=1 write: skipped (no registry)")

	_, reloaded, err := rev.Load()
	require.NoError(t, err)
	require.Len(t, reloaded, 1)
	require.Equal(t, "read", reloaded[0].CommandName)
}

func TestCheckpointRollbackReportsMissingTargetIteration(t *testing.T) {
	t.Parallel()
	target := 99
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{}, core.Execution{
		{Iteration: 1, CommandName: "read"},
	}))

	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &target},
		Checkpoint: rev,
		RunID:      "run-1",
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "target iteration 99 not found")
	require.Empty(t, rev.reverted)
}

func TestCheckpointRollbackRestoresFileViaPersistedReceipt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(target, []byte("v1"), 0o644))

	// Execute a real write that overwrites the file; capture its opaque receipt.
	writeBuilder := &filesystem.WriteBuilder{Root: dir}
	writeCmd := writeBuilder.Build(core.Result{Output: `{"parameters":{"path":"a.txt","content":"v2"}}`})
	writeRes := writeCmd.Execute()
	require.Equal(t, core.ToolDone, writeRes.Signal)
	require.NotEmpty(t, writeRes.Receipt)
	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "v2", string(content))

	// Persist the execution (including the receipt) in the reverter.
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "write", Signal: core.ToolDone, Receipt: writeRes.Receipt},
	}))

	// A fresh registry resolves "write" to a builder that implements core.Reverser.
	reg := core.NewRegistry()
	reg.Register(filesystem.WriteToolSpec(), &filesystem.WriteBuilder{Root: dir})

	toIteration := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &toIteration},
		Checkpoint: rev,
		Registry:   reg,
		RunID:      "run-1",
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal)
	require.Contains(t, res.Output, "step=1 write:")
	restored, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "v1", string(restored))
}

func TestRollbackCheckpointExecutesBoundaryCompensation(t *testing.T) {
	t.Parallel()
	var restored bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/repos/acme/agent-core/issues/1":
			require.Equal(t, http.MethodPatch, req.Method)
			writeLifecycleJSON(t, w, http.StatusOK, map[string]interface{}{"id": "ISS-1", "title": "new"})
		case "/repos/acme/agent-core/issues/ISS-1":
			restored = true
			require.Equal(t, http.MethodPatch, req.Method)
			writeLifecycleJSON(t, w, http.StatusOK, map[string]interface{}{"id": "ISS-1", "title": "restored"})
		default:
			http.NotFound(w, req)
		}
	}))
	defer upstream.Close()

	collection, operation := lifecycleRESTCollection(t, upstream.URL)
	writeBuilder := toolrest.ClientBuilder{
		ToolName: "rest_set_issue", Init: toolrest.InitClientSet,
		Operation: operation, Definitions: collection,
	}
	writeRes := writeBuilder.Build(core.Result{Output: `{"parameters":{"owner":"acme","repo":"agent-core","number":"1","title":"new"}}`}).Execute()
	require.Equal(t, core.Signal("RESTResourceWritten"), writeRes.Signal, writeRes.Output)
	require.NotEmpty(t, writeRes.Receipt)

	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "rest_set_issue", Signal: core.Signal("RESTResourceWritten"), Receipt: writeRes.Receipt},
	}))
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "rest_set_issue", Visibility: core.Internal}, writeBuilder)

	toIteration := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &toIteration},
		Checkpoint: rev,
		Registry:   reg,
		RunID:      "run-rest",
	}).Build(core.Result{})
	res := cmd.Execute()

	require.Equal(t, core.ToolDone, res.Signal, res.Output)
	require.Contains(t, res.Output, "step=1 rest_set_issue:")
	require.True(t, restored)
}

func TestCheckpointRollbackReportsMissingRESTCompensationExecutor(t *testing.T) {
	t.Parallel()
	_, operation := lifecycleRESTCollection(t, "http://127.0.0.1")
	writeBuilder := toolrest.ClientBuilder{
		ToolName: "rest_set_issue", Init: toolrest.InitClientSet, Operation: operation,
	}
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "rest_set_issue", Visibility: core.Internal}, writeBuilder)

	toIteration := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &toIteration},
		Checkpoint: lifecycleReverterWithRESTReceipt(t),
		Registry:   reg,
		RunID:      "run-rest",
	}).Build(core.Result{})
	res := cmd.Execute()

	// A receipt-walk Undo that fails is a partial rollback, not a clean one:
	// the tool must report CommandError and name the entry whose external
	// effect was not reversed (srd026 R3.7, R6.3, R6.4; GH-491).
	require.Equal(t, core.CommandError, res.Signal, res.Output)
	require.Contains(t, res.Output, "step=1 rest_set_issue: undo failed")
	require.Contains(t, res.Output, "compensation_lookup")
	require.Contains(t, res.Output, "receipt-walk Undo failure")

	var partial *PartialRollbackError
	require.ErrorAs(t, res.Err, &partial)
	require.Equal(t, 0, partial.Reverted)
	require.Len(t, partial.Failures, 1)
	require.Equal(t, "rest_set_issue", partial.Failures[0].CommandName)
}

func TestCheckpointRollbackUndoRequestsCompensation(t *testing.T) {
	t.Parallel()
	cmd := (&CheckpointRollbackBuilder{}).Build(core.Result{})

	res := cmd.Undo(core.Result{})

	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "resume from the original checkpoint or choose another rollback checkpoint")
}

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

// TestCheckpointRollbackStructuredOutputMatchesSchema decodes the rollback
// Result.Output against the declared checkpoint-rollback schema and asserts the
// required fields — run, target_step, reverted_entries — and skipped list are
// present and correct (srd026 R3.8; GH-493).
func TestCheckpointRollbackStructuredOutputMatchesSchema(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "ok", Visibility: core.Internal}, reverserStub{name: "ok"})
	rev := &fakeReverter{}
	require.NoError(t, rev.Save(core.Position{CurrentState: "Working"}, core.Execution{
		{Iteration: 1, CommandName: "read", Signal: core.ToolDone},
		{Iteration: 2, CommandName: "ok", Signal: core.ToolDone, Receipt: "r-ok"},
	}))

	toIteration := 1
	cmd := (&CheckpointRollbackBuilder{
		Config:     catalog.CheckpointRollbackConfig{ToIteration: &toIteration},
		Checkpoint: rev,
		Registry:   reg,
		RunID:      "run-x",
	}).Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal, res.Output)

	var out struct {
		Run                 string   `json:"run"`
		TargetStep          int      `json:"target_step"`
		RevertedEntries     int      `json:"reverted_entries"`
		SkippedIrreversible []string `json:"skipped_irreversible"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &out))
	require.Equal(t, "run-x", out.Run)
	require.Equal(t, 0, out.TargetStep)
	require.Equal(t, 1, out.RevertedEntries)
	require.NotNil(t, out.SkippedIrreversible)
}

// TestCheckpointHistoryEchoesExplicitRunSelector proves the structured history
// output surfaces the explicitly selected run identity (srd026 R2.1; GH-493).
func TestCheckpointHistoryEchoesExplicitRunSelector(t *testing.T) {
	t.Parallel()
	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{CurrentState: "Working"},
		core.Execution{{Iteration: 1, CommandName: "read", Signal: core.ToolDone}}))

	cmd := (&CheckpointHistoryBuilder{
		Config:     catalog.CheckpointHistoryConfig{Checkpoint: "run-42"},
		Checkpoint: cp,
	}).Build(core.Result{})
	res := cmd.Execute()
	require.Equal(t, core.ToolDone, res.Signal)

	var out struct {
		Run     string `json:"run"`
		History string `json:"history"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Output), &out))
	require.Equal(t, "run-42", out.Run)
	require.NotEmpty(t, out.History)
}
