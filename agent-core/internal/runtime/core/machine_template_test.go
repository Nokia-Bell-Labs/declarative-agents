// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Machine templates (srd054): a whole machine as a fragment, and a machine file
// that expands it.

const checkTemplate = `unit: check-template
params:
- {name: word, type: string}
- {name: prefix, type: string, default: Embed}
machine:
  name: check
  purpose: Run one bound word.
  initial_state: Ready
  budget: {command_timeout: 1m, max_iterations: 5}
  states: [Ready, {name: Done, run_status: succeeded}, {name: Failed, run_status: failed}]
  terminal_states: [Done, Failed]
  signals: [Seed, CommandError]
  transitions:
    - {state: Ready, signal: Seed, next: Done, action: $param(word)}
    - {state: Ready, signal: CommandError, next: Failed}
`

func TestMachineTemplateInstanceEqualsTheHandWrittenMachine(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "..", "testdata", "integration")
	control, err := core.LoadMachineClosure(filepath.Join(root, "profiles", "control", "machine.yaml"), nil)
	require.NoError(t, err)
	var visited []string
	instance, err := core.LoadMachineClosure(filepath.Join(root, "profiles", "control-template", "machine.yaml"),
		func(path string, _ []byte) error {
			visited = append(visited, filepath.Base(path))
			return nil
		})
	require.NoError(t, err)

	require.Equal(t, "core-control-template", instance.Name, "the instance's name replaces the template's")
	instance.Name = control.Name
	require.Equal(t, canonicalMachine(t, control), canonicalMachine(t, instance),
		"an expanded machine is the hand-written machine apart from its name")
	require.Equal(t, []string{"machine.yaml", "control-machine-template.yaml"}, visited)
	require.Equal(t, filepath.Join(root, "units", "control-machine-template.yaml"), instance.TemplatePath(),
		"the template path resolves against the instance, as a stage path does")
	expansions := instance.Expansions()
	require.Len(t, expansions, 1)
	require.Equal(t, core.ExpansionKindMachine, expansions[0].Kind)
	require.Equal(t, map[string]string{"launch": "launch_agent_control", "await": "await_agent_control"},
		expansions[0].Args)
	require.Equal(t, []string{"machine/core-control-template"}, expansions[0].Produces)
}

func TestMachineInstanceCarriesOnlyItsHeader(t *testing.T) {
	t.Parallel()
	for name, instance := range map[string]string{
		"extra field": "unit: i\nstates: [Ready]\nexpand:\n  - {fragment: units/check.yaml, args: {word: go}}\n",
		"prefix":      "unit: i\nexpand:\n  - {fragment: units/check.yaml, as: x, args: {word: go}}\n",
		"stage beside template": "unit: i\nexpand:\n  - {fragment: units/check.yaml, args: {word: go}}\n" +
			"  - {fragment: units/run.yaml, args: {prefix: Embed, enter: Seed, word: go}}\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeStageFixture(t, map[string]string{
				"units/check.yaml": checkTemplate, "units/run.yaml": runStage, "machine.yaml": instance,
			})
			_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)
			require.Error(t, err)
			require.Contains(t, err.Error(), filepath.Join(root, "machine.yaml"))
		})
	}

	root := writeStageFixture(t, map[string]string{
		"units/check.yaml": checkTemplate,
		"machine.yaml": "unit: i\nname: renamed\npurpose: Renamed.\nexpand:\n" +
			"  - {fragment: units/check.yaml, args: {word: go}}\n",
	})
	spec, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)
	require.NoError(t, err)
	require.Equal(t, "renamed", spec.Name)
	require.Equal(t, "Renamed.", spec.Purpose)
}

func TestMachineTemplateSplicesItsStagesRelativeToItself(t *testing.T) {
	t.Parallel()
	template := checkTemplate[:len(checkTemplate)-len("  transitions:\n    - {state: Ready, signal: Seed, next: Done, action: $param(word)}\n    - {state: Ready, signal: CommandError, next: Failed}\n")] +
		"  signals: [Seed, Embed, CommandError]\n" +
		"  expand:\n    - {fragment: stages/run.yaml, args: {prefix: $param(prefix), enter: Embed, word: $param(word)}}\n" +
		"  transitions:\n    - {state: Ready, signal: Seed, next: Done}\n"
	template = replaceOnce(t, template, "  signals: [Seed, CommandError]\n", "")
	root := writeStageFixture(t, map[string]string{
		"library/check.yaml":      template,
		"library/stages/run.yaml": runStage,
		"agents/one/machine.yaml": "unit: one\nexpand:\n  - {fragment: ../../library/check.yaml, args: {word: embed_query}}\n",
	})

	spec, err := core.LoadMachineClosure(filepath.Join(root, "agents", "one", "machine.yaml"), nil)

	require.NoError(t, err)
	require.Contains(t, spec.States.Names(), "EmbedRunning", "the stage resolved beside the template")
	var entered core.TransitionSpec
	for _, transition := range spec.Transitions {
		if transition.State == "Ready" && transition.Signal == "Embed" {
			entered = transition
		}
	}
	require.Equal(t, "embed_query", entered.Action, "the template's argument passed through to the stage")
	kinds := []string{}
	for _, expansion := range spec.Expansions() {
		kinds = append(kinds, expansion.Kind)
	}
	require.Equal(t, []string{core.ExpansionKindMachine, core.ExpansionKindStage}, kinds)
}

func TestMachineTemplateCannotNestOrBeLoadedAsAMachine(t *testing.T) {
	t.Parallel()
	nesting := replaceOnce(t, checkTemplate, "  transitions:\n",
		"  expand:\n    - {fragment: check.yaml, args: {word: go}}\n  transitions:\n")
	root := writeStageFixture(t, map[string]string{
		"units/check.yaml":  checkTemplate,
		"units/nested.yaml": nesting,
		"machine.yaml":      "unit: i\nexpand:\n  - {fragment: units/nested.yaml, args: {word: go}}\n",
	})

	_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)
	require.ErrorContains(t, err, "expands no template")

	_, err = core.LoadMachineSpec(filepath.Join(root, "units", "check.yaml"))
	require.ErrorContains(t, err, "is a machine template")
	require.ErrorContains(t, err, filepath.Join(root, "units", "check.yaml"))
}

func TestExpandTargetBodyKindSelectsReplaceOrSplice(t *testing.T) {
	t.Parallel()
	root := writeStageFixture(t, map[string]string{
		"units/tools.yaml": "unit: words\nparams:\n- {name: word, type: string}\ntools:\n- {name: $param(word), binary: echo}\n",
		"machine.yaml":     machineHead + "expand:\n  - {fragment: units/tools.yaml, args: {word: go}}\ntransitions:\n  - {state: Ready, signal: Seed, next: Done}\n",
	})

	_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

	require.ErrorContains(t, err, `body kind "tools" is neither a stage nor a machine`)
}

func TestMachineTemplateFaultsNameTemplateAndInstance(t *testing.T) {
	t.Parallel()
	unreachable := replaceOnce(t, checkTemplate, "next: Failed}", "next: Nowhere}")
	root := writeStageFixture(t, map[string]string{
		"units/check.yaml":      checkTemplate,
		"units/orphan.yaml":     unreachable,
		"missing/machine.yaml":  "unit: i\nexpand:\n  - {fragment: ../units/check.yaml, args: {}}\n",
		"leftover/machine.yaml": "unit: i\nexpand:\n  - {fragment: ../units/orphan.yaml, args: {word: go}}\n",
	})

	_, err := core.LoadMachineClosure(filepath.Join(root, "missing", "machine.yaml"), nil)
	require.ErrorContains(t, err, "word", "a missing argument names the parameter")
	require.ErrorContains(t, err, filepath.Join(root, "units", "check.yaml"))

	_, err = core.LoadMachineClosure(filepath.Join(root, "leftover", "machine.yaml"), nil)
	require.Error(t, err, "the expanded machine takes the hand-written structural checks")
	require.ErrorContains(t, err, filepath.Join(root, "leftover", "machine.yaml"))
	require.ErrorContains(t, err, filepath.Join(root, "units", "orphan.yaml"))
}

func canonicalMachine(t *testing.T, spec core.MachineSpec) string {
	t.Helper()
	data, err := yaml.Marshal(spec)
	require.NoError(t, err)
	return string(data)
}

func replaceOnce(t *testing.T, text, old, replacement string) string {
	t.Helper()
	index := indexOf(text, old)
	require.GreaterOrEqual(t, index, 0, "fixture edit %q not found", old)
	return text[:index] + replacement + text[index+len(old):]
}

func indexOf(text, fragment string) int {
	for i := 0; i+len(fragment) <= len(text); i++ {
		if text[i:i+len(fragment)] == fragment {
			return i
		}
	}
	return -1
}

func TestMachineTemplateBodyHoldsNoParamsUnitOrImports(t *testing.T) {
	t.Parallel()
	for name, edit := range map[string]string{
		"nested params": "  params: [{name: other, type: string}]\n",
		"unit":          "  unit: inner\n",
		"imports":       "  imports: [other.yaml]\n",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := writeStageFixture(t, map[string]string{
				"units/check.yaml": replaceOnce(t, checkTemplate, "  name: check\n", "  name: check\n"+edit),
				"machine.yaml":     "unit: i\nexpand:\n  - {fragment: units/check.yaml, args: {word: go}}\n",
			})
			_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)
			require.Error(t, err)
			require.ErrorContains(t, err, filepath.Join(root, "units", "check.yaml"))
		})
	}
}
