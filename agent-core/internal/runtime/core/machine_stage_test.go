// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Machine stage fragments (srd052 R4).

const runStage = `unit: run-stage
params:
- {name: prefix, type: string}
- {name: enter, type: string}
- {name: word, type: string}
stage:
  states:
    - {name: $param(prefix)Running, meaning: Run the bound word.}
  signals: [$param(prefix)Done]
  transitions:
    - {state: Ready, signal: $param(enter), next: $param(prefix)Running, action: $param(word), label: $param(prefix)_result}
    - {state: $param(prefix)Running, signal: $param(prefix)Done, next: Done}
    - {state: $param(prefix)Running, signal: CommandError, next: Failed}
`

const machineHead = `name: staged
initial_state: Ready
budget: {command_timeout: 1m, max_iterations: 5}
states: [Ready, {name: Done, run_status: succeeded}, {name: Failed, run_status: failed}]
terminal_states: [Done, Failed]
signals: [Seed, Embed, Rerank, CommandError]
`

func writeStageFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return root
}

func TestLoadMachineClosureSplicesAStageTwice(t *testing.T) {
	t.Parallel()
	root := writeStageFixture(t, map[string]string{
		"units/run.yaml": runStage,
		"machine.yaml": machineHead + `expand:
  - {fragment: units/run.yaml, args: {prefix: Embed, enter: Embed, word: embed_query}}
  - {fragment: units/run.yaml, args: {prefix: Rerank, enter: Rerank, word: rerank}}
transitions:
  - {state: Ready, signal: Seed, next: Done}
`,
	})
	var visited []string
	spec, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), func(path string, _ []byte) error {
		visited = append(visited, filepath.Base(path))
		return nil
	})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Ready", "Done", "Failed", "EmbedRunning", "RerankRunning"}, spec.States.Names())
	require.Contains(t, spec.Signals.Names(), "EmbedDone")
	require.Len(t, spec.Transitions, 7)
	require.Equal(t, "embed_query", spec.Transitions[1].Action)
	require.Equal(t, "Embed_result", spec.Transitions[1].Label)
	require.Empty(t, spec.Instantiate, "the spliced machine carries its stages as states and transitions")
	require.Equal(t, []string{"machine.yaml", "run.yaml", "run.yaml"}, visited,
		"the closure sees the machine and the fragment for each instantiation")

	instantiations := spec.Instantiations()
	require.Len(t, instantiations, 2)
	require.Equal(t, filepath.Join(root, "units", "run.yaml"), instantiations[0].Fragment)
	require.Equal(t, map[string]string{"prefix": "Embed", "enter": "Embed", "word": "embed_query"}, instantiations[0].Args)
	require.Equal(t, []string{"signals/EmbedDone", "states/EmbedRunning",
		"transitions/EmbedRunning:CommandError", "transitions/EmbedRunning:EmbedDone", "transitions/Ready:Embed"},
		instantiations[0].Produces)
}

// TestSplicedStageIsValidatedAsOneMachine is srd052 R4.2: a stage naming a
// state the importer never declares fails the way a hand-written transition
// would, with the fragment named.
func TestSplicedStageIsValidatedAsOneMachine(t *testing.T) {
	t.Parallel()
	root := writeStageFixture(t, map[string]string{
		"units/run.yaml": runStage,
		"machine.yaml": `name: staged
initial_state: Ready
budget: {command_timeout: 1m, max_iterations: 5}
states: [Ready, {name: Done, run_status: succeeded}]
terminal_states: [Done]
signals: [Seed, Embed, CommandError]
expand:
  - {fragment: units/run.yaml, args: {prefix: Embed, enter: Embed, word: embed_query}}
transitions:
  - {state: Ready, signal: Seed, next: Done}
`,
	})

	_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

	require.ErrorContains(t, err, `next "Failed" not in states list`)
	require.ErrorContains(t, err, "after splicing")
	require.ErrorContains(t, err, "run.yaml")
}

func TestSplicingTheSamePrefixTwiceIsTheDuplicateStateError(t *testing.T) {
	t.Parallel()
	root := writeStageFixture(t, map[string]string{
		"units/run.yaml": runStage,
		"machine.yaml": machineHead + `expand:
  - {fragment: units/run.yaml, args: {prefix: Embed, enter: Embed, word: a}}
  - {fragment: units/run.yaml, args: {prefix: Embed, enter: Rerank, word: b}}
transitions:
  - {state: Ready, signal: Seed, next: Done}
`,
	})

	_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

	require.ErrorContains(t, err, `duplicate name "EmbedRunning"`)
}

func TestStageArgumentFaultsNameFragmentAndParameter(t *testing.T) {
	t.Parallel()
	root := writeStageFixture(t, map[string]string{
		"units/run.yaml": runStage,
		"machine.yaml": machineHead + `expand:
  - {fragment: units/run.yaml, args: {prefix: Embed, enter: Embed}}
transitions:
  - {state: Ready, signal: Seed, next: Done}
`,
	})

	_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

	require.ErrorContains(t, err, `parameter "word" is required`)
	require.ErrorContains(t, err, `instantiates "units/run.yaml"`)
	require.ErrorContains(t, err, "run.yaml")
}

func TestMachineImportsAreRejectedWithTheReason(t *testing.T) {
	t.Parallel()
	root := writeStageFixture(t, map[string]string{
		"machine.yaml": machineHead + "imports: [units/run.yaml]\ntransitions:\n  - {state: Ready, signal: Seed, next: Done}\n",
	})

	_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

	require.ErrorContains(t, err, "a machine imports nothing; a stage fragment is instantiated")
}

func TestParseMachineSpecRefusesInstantiateWithoutAFile(t *testing.T) {
	t.Parallel()
	_, err := core.ParseMachineSpec([]byte(machineHead + "expand:\n  - {fragment: x.yaml, args: {}}\ntransitions:\n  - {state: Ready, signal: Seed, next: Done}\n"))

	require.ErrorContains(t, err, "must be loaded from its file")
}

func TestHalfDeclaredStageFragmentFails(t *testing.T) {
	t.Parallel()
	root := writeStageFixture(t, map[string]string{
		"units/half.yaml": "unit: half\nparams:\n- {name: p, type: string}\n",
		"machine.yaml": machineHead + `expand:
  - {fragment: units/half.yaml, args: {p: x}}
transitions:
  - {state: Ready, signal: Seed, next: Done}
`,
	})

	_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

	require.ErrorContains(t, err, "a stage fragment declares params and a stage body")
}
