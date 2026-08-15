// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import (
	"errors"
	"fmt"
	"sync"
)

type rememberedSignalRun struct {
	state  State
	status RunStatus
	known  bool
}

// LoopSignalSource implements lookup-only admission followed by ordinary Loop
// execution. Its zero value is ready for use.
type LoopSignalSource struct {
	ownership RunOwnership
	mu        sync.Mutex
	runs      map[string]rememberedSignalRun
}

func NewLoopSignalSource() *LoopSignalSource {
	return &LoopSignalSource{}
}

// Ownership exposes the process-local owner set for lifecycle wiring and
// focused admission diagnostics. Callers may use TryAcquire only when they also
// arrange a matching release.
func (s *LoopSignalSource) Ownership() *RunOwnership {
	return &s.ownership
}

func (s *LoopSignalSource) remembered(runID string) rememberedSignalRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.runs[runID]
}

func (s *LoopSignalSource) remember(runID string, run RunResult, fallback State) {
	state := run.FinalState
	if state == "" {
		state = fallback
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.runs == nil {
		s.runs = make(map[string]rememberedSignalRun)
	}
	s.runs[runID] = rememberedSignalRun{
		state: state, status: run.Status, known: true,
	}
}

func admissionSpec(params LoopParams) (MachineSpec, error) {
	if params.MachineSpec == nil && params.MachineFile == "" {
		return MachineSpec{}, errors.New("signal admission requires a MachineSpec or MachineFile")
	}
	spec, err := loopMachineSpec(&params)
	if err != nil {
		return MachineSpec{}, fmt.Errorf("signal admission machine: %w", err)
	}
	return spec, nil
}

func signalDeclared(spec MachineSpec, signal Signal) bool {
	if signal == "" {
		return false
	}
	for _, declared := range spec.Signals {
		if declared.Name == string(signal) {
			return true
		}
	}
	return false
}

// exactTransitionCount deliberately reads MachineSpec instead of the built
// TransitionTable. A table collapses duplicate (state, signal) rows and could
// incorrectly turn an ambiguous declarative program into one admitted row.
func exactTransitionCount(spec MachineSpec, state State, signal Signal) int {
	count := 0
	for _, transition := range spec.Transitions {
		if transition.State == string(state) && transition.Signal == string(signal) {
			count++
		}
	}
	return count
}

func initialAdmissionState(params LoopParams, spec MachineSpec) State {
	if params.InitialState != "" {
		return params.InitialState
	}
	return State(spec.InitialState)
}

func (s *LoopSignalSource) paramsAtCurrentPosition(
	envelope SignalEnvelope,
	params LoopParams,
	spec MachineSpec,
) (LoopParams, State, string, error) {
	remembered := s.remembered(envelope.RunID)
	loadPersisted := envelope.Resume ||
		(remembered.known && remembered.status == StatusSuspended) ||
		(!remembered.known && checkpointPersistenceEnabled(params.Checkpoint))
	if !loadPersisted {
		params = signalParamsWithoutCheckpoint(params, spec, remembered)
		return params, params.InitialState, "", nil
	}
	if !checkpointPersistenceEnabled(params.Checkpoint) {
		return params, remembered.state, "checkpoint_unavailable",
			errors.New("signal admission: suspended run requires a persistent checkpoint")
	}

	// Supplying the envelope signal avoids applying the machine's model-oriented
	// resume signal. LoadResume still restores Position, counters, iterator, and
	// Execution; the caller replaces only its previous-result seed after lookup.
	params.InitialSignal = envelope.Signal
	resumed, err := LoadResume(params)
	if err != nil {
		if errors.Is(err, ErrNoCheckpoint) && !envelope.Resume && !remembered.known {
			params.InitialState = initialAdmissionState(params, spec)
			return params, params.InitialState, "", nil
		}
		return params, remembered.state, "checkpoint_load_failed", err
	}
	if err := validateSignalProgram(params.Program, resumed.Position.Snapshot.Program); err != nil {
		return params, resumed.Position.CurrentState, "checkpoint_incompatible", err
	}
	if err := restoreSignalSnapshot(params, resumed.Position.Snapshot); err != nil {
		return params, resumed.Position.CurrentState, "checkpoint_restore_failed", err
	}
	return resumed.Params, resumed.Position.CurrentState, "", nil
}

func signalParamsWithoutCheckpoint(
	params LoopParams,
	spec MachineSpec,
	remembered rememberedSignalRun,
) LoopParams {
	if remembered.known {
		params.InitialState = remembered.state
	} else {
		params.InitialState = initialAdmissionState(params, spec)
	}
	return params
}

func restoreSignalSnapshot(params LoopParams, snapshot AgentSnapshot) error {
	if params.Hooks.RestoreSnapshot == nil {
		return nil
	}
	return params.Hooks.RestoreSnapshot(snapshot)
}

func validateSignalProgram(expected, persisted ProgramRef) error {
	if expected.IsZero() && persisted.IsZero() {
		return nil
	}
	if expected.IsZero() || persisted.IsZero() || expected != persisted {
		return fmt.Errorf(
			"%w: declarative program identity does not match checkpoint",
			ErrCheckpointIncompatible,
		)
	}
	return nil
}
