// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	requestedSignal Signal = "WorkRequested"
	continueSignal  Signal = "ContinueRequested"
)

type signalTestBuilder func(Result) Command

func (b signalTestBuilder) Build(result Result) Command { return b(result) }

type signalTestCommand struct {
	name    string
	execute func() Result
}

func (c signalTestCommand) Name() string { return c.name }
func (c signalTestCommand) Execute() Result {
	if c.execute == nil {
		return Result{Signal: ToolDone}
	}
	return c.execute()
}
func (c signalTestCommand) Undo(Result) Result { return NoopUndo(c.name) }

type signalCountCheckpoint struct {
	mu       sync.Mutex
	position Position
	exec     Execution
	loadErr  error
	saveErr  error
	loads    int
	saves    int
}

func (c *signalCountCheckpoint) Load() (Position, Execution, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loads++
	return clonePosition(c.position), cloneExecution(c.exec), c.loadErr
}

func (c *signalCountCheckpoint) Save(position Position, execution Execution) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.saves++
	if c.saveErr != nil {
		return c.saveErr
	}
	c.position = clonePosition(position)
	c.exec = cloneExecution(execution)
	return nil
}

func signalMachineSpec() MachineSpec {
	return MachineSpec{
		Name:           "request-signal-test",
		InitialState:   "Start",
		States:         StateSpecsFromNames("Start", "Working", "Waiting", "Done", "Failed"),
		TerminalStates: []string{"Done", "Failed"},
		Signals: SignalSpecsFromNames(
			string(requestedSignal), string(continueSignal), string(ToolDone),
			string(CommandError), string(AwaitApproval),
		),
		Transitions: []TransitionSpec{
			{State: "Start", Signal: string(requestedSignal), Next: "Working", Action: "work"},
			{State: "Working", Signal: string(ToolDone), Next: "Done"},
			{State: "Working", Signal: string(CommandError), Next: "Failed"},
		},
	}
}

func signalLoopParams(spec MachineSpec, builder Builder, checkpoint Checkpoint) LoopParams {
	return LoopParams{
		MachineSpec: &spec,
		Trace:       &loopRecorder{},
		Budget:      Budget{MaxIterations: 10},
		Checkpoint:  checkpoint,
		InitFunc: func(registry *Registry) error {
			if builder != nil {
				return registry.RegisterChecked(
					ToolSpec{Name: "work", Visibility: Internal},
					builder,
				)
			}
			return nil
		},
		Hooks: LoopHooks{TerminalStatus: func(state State) RunStatus {
			if state == "Done" {
				return StatusSucceeded
			}
			return StatusFailed
		}},
	}
}

func signalEnvelope(runID string, signal Signal) SignalEnvelope {
	return SignalEnvelope{
		Source: "configured-source", Route: "work",
		RequestID: "request-" + runID, RunID: runID, Signal: signal,
		Payload: json.RawMessage(`{"public":"ok"}`),
	}
}

func successfulSignalBuilder(builds *atomic.Int32) Builder {
	return signalTestBuilder(func(Result) Command {
		if builds != nil {
			builds.Add(1)
		}
		return signalTestCommand{name: "work"}
	})
}

func TestSignalAdmission_Outcomes(t *testing.T) {
	tests := []struct {
		name       string
		envelope   SignalEnvelope
		changeSpec func(*MachineSpec)
		holdOwner  bool
		want       AdmissionOutcome
	}{
		{
			name: "accepted", envelope: signalEnvelope("accepted", requestedSignal),
			want: AdmissionAccepted,
		},
		{
			name: "empty signal", envelope: signalEnvelope("empty", ""),
			want: AdmissionRefusedUndeclared,
		},
		{
			name: "undeclared", envelope: signalEnvelope("undeclared", "CallerInvented"),
			want: AdmissionRefusedUndeclared,
		},
		{
			name: "no transition", envelope: signalEnvelope("no-transition", continueSignal),
			want: AdmissionRefusedConflict,
		},
		{
			name: "stale expected state",
			envelope: func() SignalEnvelope {
				envelope := signalEnvelope("stale", requestedSignal)
				envelope.ExpectedState = "Waiting"
				return envelope
			}(),
			want: AdmissionRefusedConflict,
		},
		{
			name: "active owner", envelope: signalEnvelope("owned", requestedSignal),
			holdOwner: true, want: AdmissionRefusedConflict,
		},
		{
			name:     "duplicate transition is not exact",
			envelope: signalEnvelope("duplicate", requestedSignal),
			changeSpec: func(spec *MachineSpec) {
				spec.Transitions = append(spec.Transitions, spec.Transitions[0])
			},
			want: AdmissionRefusedConflict,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			spec := signalMachineSpec()
			if test.changeSpec != nil {
				test.changeSpec(&spec)
			}
			checkpoint := &signalCountCheckpoint{
				position: Position{CurrentState: "unchanged"},
				exec: Execution{{
					CommandName: "unchanged",
					Result:      checkpointDigest(ToolDone, "unchanged", Cost{}),
				}},
				loadErr: ErrNoCheckpoint,
			}
			var builds atomic.Int32
			source := NewLoopSignalSource()
			var heldRelease func()
			if test.holdOwner {
				var acquired bool
				heldRelease, acquired = source.Ownership().TryAcquire(test.envelope.RunID)
				require.True(t, acquired)
				defer heldRelease()
			}

			admission := source.Admit(
				context.Background(),
				test.envelope,
				signalLoopParams(spec, successfulSignalBuilder(&builds), checkpoint),
			)

			require.Equal(t, test.want, admission.Outcome)
			if test.want == AdmissionAccepted {
				require.Equal(t, int32(1), builds.Load())
				require.Greater(t, checkpoint.saves, 0)
				return
			}
			require.Zero(t, builds.Load(), "refusal must not construct a command")
			require.Zero(t, checkpoint.saves, "refusal must not save a checkpoint")
			require.Equal(t, State("unchanged"), checkpoint.position.CurrentState)
			require.Len(t, checkpoint.exec, 1)
			require.Equal(t, "unchanged", checkpoint.exec[0].CommandName)
		})
	}
}

func TestSignalSource_FreshAndFailedExecution(t *testing.T) {
	t.Run("fresh ordinary loop", func(t *testing.T) {
		checkpoint := &InMemoryCheckpoint{}
		var received Result
		builder := signalTestBuilder(func(result Result) Command {
			received = result
			return signalTestCommand{name: "work"}
		})
		admission := NewLoopSignalSource().Admit(
			context.Background(),
			signalEnvelope("fresh", requestedSignal),
			signalLoopParams(signalMachineSpec(), builder, checkpoint),
		)

		require.Equal(t, AdmissionAccepted, admission.Outcome)
		require.Equal(t, StatusSucceeded, admission.RunStatus)
		require.Equal(t, State("Start"), admission.StateBefore)
		require.Equal(t, State("Done"), admission.StateAfter)
		require.Equal(t, requestedSignal, received.Signal)
		require.JSONEq(t, `{"public":"ok"}`, received.Output)
		position, execution, err := checkpoint.Load()
		require.NoError(t, err)
		require.Equal(t, State("Done"), position.CurrentState)
		require.Len(t, execution, 1, "only the ordinary dispatched command is recorded")
		require.Equal(t, requestedSignal, execution[0].Signal)
	})

	t.Run("command failure stays accepted", func(t *testing.T) {
		builder := signalTestBuilder(func(Result) Command {
			return signalTestCommand{name: "work", execute: func() Result {
				return Result{Err: errors.New("command failed")}
			}}
		})
		admission := NewLoopSignalSource().Admit(
			context.Background(),
			signalEnvelope("failed", requestedSignal),
			signalLoopParams(signalMachineSpec(), builder, &InMemoryCheckpoint{}),
		)

		require.Equal(t, AdmissionAccepted, admission.Outcome)
		require.Equal(t, StatusFailed, admission.RunStatus)
		require.Equal(t, "command_error", admission.Stage)
		require.ErrorContains(t, admission.Run.LastError, "command failed")
	})
}

func TestSignalSource_RedactsPayloadBeforeLiveAndPersistedState(t *testing.T) {
	tests := []struct {
		name      string
		paths     []OutputRedactionPath
		want      string
		wantStage OutputRedactionStatus
	}{
		{
			name:  "valid paths",
			paths: []OutputRedactionPath{{"credentials", "token"}},
			want:  `{"credentials":{"owner":"alice"},"public":"ok"}`,
		},
		{
			name:  "malformed paths omit output",
			paths: []OutputRedactionPath{{}},
			want:  "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			checkpoint := &InMemoryCheckpoint{}
			var received Result
			builder := signalTestBuilder(func(result Result) Command {
				received = result
				return signalTestCommand{name: "work", execute: func() Result {
					return Result{
						Signal: ToolDone, Output: result.Output,
						Redaction: result.Redaction,
					}
				}}
			})
			envelope := signalEnvelope("redaction-"+test.name, requestedSignal)
			envelope.Payload = json.RawMessage(
				`{"credentials":{"token":"secret","owner":"alice"},"public":"ok"}`,
			)
			envelope.SensitivePaths = test.paths

			admission := NewLoopSignalSource().Admit(
				context.Background(), envelope,
				signalLoopParams(signalMachineSpec(), builder, checkpoint),
			)

			require.Equal(t, AdmissionAccepted, admission.Outcome)
			if test.want == "" {
				require.Empty(t, received.Output)
			} else {
				require.JSONEq(t, test.want, received.Output)
			}
			require.NotContains(t, received.Output, "secret")
			_, execution, err := checkpoint.Load()
			require.NoError(t, err)
			require.Len(t, execution, 1)
			require.NotContains(t, execution[0].Result.Output, "secret")
			if test.want == "" {
				require.Empty(t, execution[0].Result.Output)
				require.Equal(t, OutputRedactionOmitted, execution[0].Result.RedactionStatus)
			}
		})
	}
}

func TestSignalSource_ResumeOverridesPreviousResultExactlyOnce(t *testing.T) {
	program := ProgramRef{Profile: "/profiles/request.yaml", Digest: "digest"}
	checkpoint := &InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(
		Position{
			CurrentState: "Waiting",
			Snapshot: AgentSnapshot{
				State: "Waiting", Iteration: 1, TokensIn: 7, Program: program,
			},
		},
		Execution{{
			Iteration: 1, CommandName: "prior", FromState: "Start", ToState: "Waiting",
			Signal: requestedSignal,
			Result: checkpointDigest(AwaitApproval, `{"previous":"must-not-seed"}`, Cost{}),
		}},
	))
	spec := signalMachineSpec()
	spec.Transitions = append(spec.Transitions,
		TransitionSpec{
			State: "Waiting", Signal: string(continueSignal),
			Next: "Working", Action: "work",
		},
	)
	var received []string
	builder := signalTestBuilder(func(result Result) Command {
		received = append(received, result.Output)
		return signalTestCommand{name: "work"}
	})
	envelope := signalEnvelope("resume", continueSignal)
	envelope.ExpectedState = "Waiting"
	envelope.Payload = json.RawMessage(`{"new":"envelope-only"}`)
	params := signalLoopParams(spec, builder, checkpoint)
	params.Program = program

	admission := NewLoopSignalSource().Admit(context.Background(), envelope, params)

	require.Equal(t, AdmissionAccepted, admission.Outcome)
	require.Equal(t, StatusSucceeded, admission.RunStatus)
	require.Equal(t, State("Waiting"), admission.StateBefore)
	require.Equal(t, 2, admission.Run.Iterations)
	require.Equal(t, 7, admission.Run.TokensIn)
	require.Equal(t, []string{`{"new":"envelope-only"}`}, received)
	position, execution, err := checkpoint.Load()
	require.NoError(t, err)
	require.Equal(t, program, position.Snapshot.Program)
	require.Len(t, execution, 2, "resume appends one command and no source entry")
	require.Equal(t, "prior", execution[0].CommandName)
	require.Equal(t, continueSignal, execution[1].Signal)
}

func TestSignalSource_ResumeValidatesProgramIdentity(t *testing.T) {
	checkpoint := &InMemoryCheckpoint{}
	require.NoError(t, checkpoint.Save(
		Position{
			CurrentState: "Waiting",
			Snapshot: AgentSnapshot{
				State: "Waiting",
				Program: ProgramRef{
					Profile: "/profiles/original.yaml", Digest: "original",
				},
			},
		},
		nil,
	))
	spec := signalMachineSpec()
	spec.Transitions = append(spec.Transitions, TransitionSpec{
		State: "Waiting", Signal: string(continueSignal), Next: "Working", Action: "work",
	})
	var builds atomic.Int32
	params := signalLoopParams(spec, successfulSignalBuilder(&builds), checkpoint)
	params.Program = ProgramRef{Profile: "/profiles/changed.yaml", Digest: "changed"}

	admission := NewLoopSignalSource().Admit(
		context.Background(), signalEnvelope("program-mismatch", continueSignal), params,
	)

	require.Equal(t, AdmissionRefusedConflict, admission.Outcome)
	require.Equal(t, "checkpoint_incompatible", admission.Stage)
	require.ErrorIs(t, admission.Err, ErrCheckpointIncompatible)
	require.Zero(t, builds.Load())
}

func TestSignalSource_NoopRefusesSuspendedClaim(t *testing.T) {
	t.Run("explicit claim", func(t *testing.T) {
		var builds atomic.Int32
		envelope := signalEnvelope("noop-resume", continueSignal)
		envelope.Resume = true
		admission := NewLoopSignalSource().Admit(
			context.Background(), envelope,
			signalLoopParams(signalMachineSpec(), successfulSignalBuilder(&builds), NoopCheckpoint{}),
		)

		require.Equal(t, AdmissionRefusedConflict, admission.Outcome)
		require.Equal(t, "checkpoint_unavailable", admission.Stage)
		require.Zero(t, builds.Load())
	})

	t.Run("remembered suspension", func(t *testing.T) {
		source := NewLoopSignalSource()
		suspending := signalTestBuilder(func(Result) Command {
			return signalTestCommand{name: "work", execute: func() Result {
				return Result{Signal: AwaitApproval}
			}}
		})
		first := source.Admit(
			context.Background(), signalEnvelope("noop-remembered", requestedSignal),
			signalLoopParams(signalMachineSpec(), suspending, NoopCheckpoint{}),
		)
		require.Equal(t, StatusSuspended, first.RunStatus)

		var builds atomic.Int32
		second := source.Admit(
			context.Background(), signalEnvelope("noop-remembered", continueSignal),
			signalLoopParams(signalMachineSpec(), successfulSignalBuilder(&builds), NoopCheckpoint{}),
		)
		require.Equal(t, AdmissionRefusedConflict, second.Outcome)
		require.Equal(t, "checkpoint_unavailable", second.Stage)
		require.Zero(t, builds.Load())
	})
}

func TestSignalSource_ConcurrentRequestsDoNotQueue(t *testing.T) {
	started := make(chan struct{})
	unblock := make(chan struct{})
	var once sync.Once
	builder := signalTestBuilder(func(Result) Command {
		return signalTestCommand{name: "work", execute: func() Result {
			once.Do(func() { close(started) })
			<-unblock
			return Result{Signal: ToolDone}
		}}
	})
	source := NewLoopSignalSource()
	envelope := signalEnvelope("concurrent", requestedSignal)
	first := make(chan SignalAdmission, 1)
	go func() {
		first <- source.Admit(
			context.Background(), envelope,
			signalLoopParams(signalMachineSpec(), builder, NoopCheckpoint{}),
		)
	}()
	<-started

	second := source.Admit(
		context.Background(), envelope,
		signalLoopParams(signalMachineSpec(), builder, NoopCheckpoint{}),
	)
	require.Equal(t, AdmissionRefusedConflict, second.Outcome)
	require.Equal(t, "concurrent_conflict", second.Stage)
	close(unblock)
	require.Equal(t, AdmissionAccepted, (<-first).Outcome)
}
