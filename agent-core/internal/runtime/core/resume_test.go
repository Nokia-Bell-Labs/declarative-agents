// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestResumeFromCheckpointRestoresStateAndReentersLoop(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	store := &memoryStateStore{}
	cp := CheckpointRecord{
		ID:        "cp-1",
		Iteration: 1,
		Timestamp: time.Now().UTC(),
		AgentState: AgentSnapshot{
			State:     "AwaitingApproval",
			Signal:    AwaitApproval,
			Iteration: 1,
			TokensIn:  10,
			TokensOut: 5,
			TotalCost: 0.25,
		},
		ConversationLog: json.RawMessage(`[{"role":"user","content":"before"}]`),
		DomainState:     json.RawMessage(`{"task":"state"}`),
		WorkspaceRef:    "ref-1",
		History: []HistoryDigest{{
			Iteration:    1,
			CommandName:  "suspend",
			FromState:    "Start",
			ToState:      "AwaitingApproval",
			Signal:       AwaitApproval,
			WorkspaceRef: "ref-1",
		}},
	}
	saveCheckpoint(t, store, cp)

	var restoredConversation json.RawMessage
	var restoredDomain json.RawMessage
	params := resumeLoopParams()
	result, err := ResumeFromCheckpoint(ResumeOptions{
		Store:        store,
		CheckpointID: "cp-1",
		Params:       params,
		RestoreConversation: func(data json.RawMessage) error {
			restoredConversation = append(json.RawMessage(nil), data...)
			return nil
		},
		RestoreDomain: func(data json.RawMessage) error {
			restoredDomain = append(json.RawMessage(nil), data...)
			return nil
		},
		ValidateCheckpoint: func(cp CheckpointRecord, params LoopParams) error {
			if cp.AgentState.State != "AwaitingApproval" {
				return fmt.Errorf("unexpected state")
			}
			return nil
		},
		Ctx: ctx,
	})

	require.NoError(t, err)
	require.Equal(t, StatusSucceeded, result.Run.Status)
	require.Equal(t, State("Finished"), result.Run.FinalState)
	require.Equal(t, 2, result.Run.Iterations)
	require.Equal(t, 10, result.Run.TokensIn)
	require.Equal(t, 5, result.Run.TokensOut)
	require.Equal(t, 0.25, result.Run.TotalCost)
	require.JSONEq(t, string(cp.ConversationLog), string(restoredConversation))
	require.JSONEq(t, string(cp.DomainState), string(restoredDomain))
}

func TestResumeFromCheckpointMissingCheckpointError(t *testing.T) {
	t.Parallel()
	_, err := ResumeFromCheckpoint(ResumeOptions{
		Store:        coreFileStoreForMissing(t),
		CheckpointID: "missing",
		Params:       resumeLoopParams(),
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrCheckpointMissing))
}

func TestResumeFromCheckpointIncompatibleCheckpointError(t *testing.T) {
	t.Parallel()
	store := &memoryStateStore{}
	require.NoError(t, store.Save(context.Background(), "checkpoint/bad", []byte(`{"iteration":1}`)))

	_, err := ResumeFromCheckpoint(ResumeOptions{
		Store:        store,
		CheckpointID: "bad",
		Params:       resumeLoopParams(),
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrCheckpointIncompatible))
}

func TestResumeFromCheckpointRestoreErrorsAreClassified(t *testing.T) {
	t.Parallel()
	store := &memoryStateStore{}
	cp := CheckpointRecord{
		ID:              "cp-1",
		AgentState:      AgentSnapshot{State: "AwaitingApproval", Iteration: 1},
		ConversationLog: json.RawMessage(`[{"role":"user","content":"before"}]`),
	}
	saveCheckpoint(t, store, cp)

	_, err := ResumeFromCheckpoint(ResumeOptions{
		Store:               store,
		CheckpointID:        "cp-1",
		Params:              resumeLoopParams(),
		RestoreConversation: func(json.RawMessage) error { return fmt.Errorf("restore boom") },
	})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrResumeRestore))
	require.Contains(t, err.Error(), "restore boom")
}

func TestResumeFromCheckpointValidatesMachineBeforeLoop(t *testing.T) {
	t.Parallel()
	store := &memoryStateStore{}
	cp := CheckpointRecord{
		ID:         "cp-1",
		AgentState: AgentSnapshot{State: "AwaitingApproval", Iteration: 1},
	}
	saveCheckpoint(t, store, cp)

	_, err := ResumeFromCheckpoint(ResumeOptions{
		Store:        store,
		CheckpointID: "cp-1",
		Params: LoopParams{
			MachineSpec: &MachineSpec{
				Name:           "bad",
				InitialState:   "Start",
				States:         StateSpecsFromNames("Start", "AwaitingApproval", "Finished"),
				TerminalStates: []string{"Finished"},
				Signals:        SignalSpecsFromNames("Approved"),
				Transitions: []TransitionSpec{{
					State: "AwaitingApproval", Signal: "Approved", Next: "Finished", Action: "missing",
				}},
			},
			Registry: NewRegistry(),
			Trace:    &loopRecorder{},
			Budget:   Budget{MaxIterations: 10},
		},
	})

	require.Error(t, err)
	require.Contains(t, err.Error(), "action \"missing\" not found")
}

func resumeLoopParams() LoopParams {
	reg := NewRegistry()
	reg.Register(ToolSpec{Name: "finish", Visibility: Internal}, &fakeBuilder{name: "finish", signal: TaskCompleted})
	builder, _ := reg.Resolve("finish")
	return LoopParams{
		InitialState: "Start",
		Registry:     reg,
		Table: TransitionTable{
			{State: "AwaitingApproval", Signal: Approved}: {
				NextState: "Finishing",
				Action:    func(r Result) Command { return builder.Build(r) },
			},
			{State: "Finishing", Signal: TaskCompleted}: {
				NextState: "Finished",
			},
		},
		IsTerminal: func(s State) bool { return s == "Finished" },
		Trace:      &loopRecorder{},
		Budget:     Budget{MaxIterations: 10},
		Hooks: LoopHooks{
			TaskCompletedSignal: TaskCompleted,
			TerminalStatus: func(s State) RunStatus {
				if s == "Finished" {
					return StatusSucceeded
				}
				return StatusFailed
			},
		},
	}
}

func saveCheckpoint(t *testing.T, store StateStore, cp CheckpointRecord) {
	t.Helper()
	data, err := json.Marshal(cp)
	require.NoError(t, err)
	require.NoError(t, store.Save(context.Background(), "checkpoint/"+cp.ID, data))
}

func coreFileStoreForMissing(t *testing.T) StateStore {
	t.Helper()
	return NewFileStore(t.TempDir())
}
