// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// totalMachine waits on nothing that cannot arrive, so each test below adds
// exactly one transition that can or cannot fire.
//
// Coverage — every emitted signal being handled where it lands — is
// ValidateToolEmits' job and is tested with it; these cover liveness only.
func totalMachine() core.MachineSpec {
	return core.MachineSpec{
		Name: "total", InitialState: "Idle",
		States:         core.StateSpecs{{Name: "Idle"}, {Name: "Working"}, {Name: "Done"}},
		TerminalStates: []string{"Done"},
		Transitions: []core.TransitionSpec{
			{State: "Idle", Signal: "Seed", Next: "Working", Action: "work"},
			{State: "Working", Signal: "Worked", Next: "Done", Action: "finish"},
			{State: "Working", Signal: "CommandError", Next: "Done", Action: "finish"},
		},
	}
}

func totalDefs() []ToolDef {
	return []ToolDef{
		{Name: "work", Emits: []string{"Worked", "CommandError"}},
		{Name: "finish", Emits: []string{"Done"}},
	}
}

func codesOf(diagnostics []core.MachineDiagnostic) map[string]int {
	counts := map[string]int{}
	for _, diagnostic := range diagnostics {
		counts[diagnostic.Code]++
	}
	return counts
}

func TestExhaustivenessAcceptsATotalMachine(t *testing.T) {
	t.Parallel()
	require.Empty(t, ValidateMachineExhaustiveness(
		totalMachine(), totalDefs(), ExhaustivenessInputs{}))
}

func TestExhaustivenessReportsADeadTransition(t *testing.T) {
	t.Parallel()
	spec := totalMachine()
	spec.Signals = core.SignalSpecsFromNames("Seed", "Worked", "CommandError", "NeverEmitted")
	spec.Transitions = append(spec.Transitions, core.TransitionSpec{
		State: "Working", Signal: "NeverEmitted", Next: "Done",
	})

	diagnostics := ValidateMachineExhaustiveness(spec, totalDefs(), ExhaustivenessInputs{})

	require.Equal(t, 1, codesOf(diagnostics)[core.DiagnosticDeadTransition])
	require.Contains(t, diagnostics[len(diagnostics)-1].Message,
		"waits on a signal nothing entering \"Working\" emits")
}

func TestExhaustivenessAcceptsJoinSignals(t *testing.T) {
	t.Parallel()
	spec := totalMachine()
	spec.Transitions = append(spec.Transitions,
		core.TransitionSpec{
			State: "Working", Signal: "Worked", Next: "Joined", Action: "work",
			ForEach: &core.ForEachSpec{
				Items: "$from(work).rows", As: "row",
				Join: core.JoinSpec{
					Next: "Joined",
					Signals: core.JoinSignalSpec{
						AllSuccess: "AllDone", Failed: "SomeFailed", Empty: "NothingToDo",
					},
				},
			},
		},
		core.TransitionSpec{State: "Joined", Signal: "AllDone", Next: "Done", Action: "finish"},
		core.TransitionSpec{State: "Joined", Signal: "SomeFailed", Next: "Done", Action: "finish"},
		core.TransitionSpec{State: "Joined", Signal: "NothingToDo", Next: "Done", Action: "finish"},
	)
	spec.States = append(spec.States, core.StateSpec{Name: "Joined"})

	diagnostics := ValidateMachineExhaustiveness(spec, totalDefs(), ExhaustivenessInputs{})

	require.Zero(t, codesOf(diagnostics)[core.DiagnosticDeadTransition],
		"a join publishes its own signals into the join's next state")
}

func TestExhaustivenessAcceptsExternallyInjectedSignals(t *testing.T) {
	t.Parallel()
	spec := totalMachine()
	spec.Signals = core.SignalSpecsFromNames("Seed", "Worked", "CommandError", "Injected")
	spec.Transitions = append(spec.Transitions, core.TransitionSpec{
		State: "Working", Signal: "Injected", Next: "Done",
	})

	require.NotZero(t,
		codesOf(ValidateMachineExhaustiveness(spec, totalDefs(), ExhaustivenessInputs{}))[core.DiagnosticDeadTransition],
		"without the source declared, the transition is dead")

	require.Zero(t, codesOf(ValidateMachineExhaustiveness(
		spec, totalDefs(), ExhaustivenessInputs{External: []string{"Injected"}},
	))[core.DiagnosticDeadTransition],
		"a request signal source makes it live")
}

func TestExhaustivenessAcceptsDeclaredResumeAndSummarySignals(t *testing.T) {
	t.Parallel()
	spec := totalMachine()
	spec.ResumeSignal = "Resumed"
	spec.SummarySignal = "Summarized"
	spec.Signals = core.SignalSpecsFromNames("Seed", "Worked", "CommandError", "Resumed", "Summarized")
	spec.Transitions = append(spec.Transitions,
		core.TransitionSpec{State: "Working", Signal: "Resumed", Next: "Done"},
		core.TransitionSpec{State: "Working", Signal: "Summarized", Next: "Done"},
	)

	require.Zero(t, codesOf(ValidateMachineExhaustiveness(
		spec, totalDefs(), ExhaustivenessInputs{}))[core.DiagnosticDeadTransition])
}
