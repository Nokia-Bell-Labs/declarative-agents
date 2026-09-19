// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Environment-selected stage fragments (srd052 R4.3, GH-2235). The variants
// differ in topology, not only in words: variant a runs a word, variant b
// records a skip and runs nothing.

const envStageSelector = "GH2235_STAGE_VARIANT"

const stageVariantA = `unit: stage-a
params:
- {name: enter, type: string}
stage:
  states:
    - {name: ARunning, meaning: Run the bound word.}
  signals: [ADone]
  transitions:
    - {state: Ready, signal: $param(enter), next: ARunning, action: embed_query, label: a_result}
    - {state: ARunning, signal: ADone, next: Done}
    - {state: ARunning, signal: CommandError, next: Failed}
`

const stageVariantB = `unit: stage-b
params:
- {name: enter, type: string}
stage:
  states:
    - {name: BSkipped, meaning: Record that the stage is skipped.}
  signals: [BDone]
  transitions:
    - {state: Ready, signal: $param(enter), next: BSkipped, action: mark_skipped, label: b_result}
    - {state: BSkipped, signal: BDone, next: Done}
    - {state: BSkipped, signal: CommandError, next: Failed}
`

// stageVariantBroken sends its stage to a state the machine never declares.
const stageVariantBroken = `unit: stage-broken
params:
- {name: enter, type: string}
stage:
  states:
    - {name: BrokenRunning, meaning: Run toward nowhere.}
  signals: [BrokenDone]
  transitions:
    - {state: Ready, signal: $param(enter), next: BrokenRunning, action: embed_query, label: broken_result}
    - {state: BrokenRunning, signal: BrokenDone, next: Nowhere}
    - {state: BrokenRunning, signal: CommandError, next: Failed}
`

func envStageMachine(fragment string) string {
	return machineHead + "instantiate:\n  - fragment: " + fragment + "\n    args: {enter: Embed}\n" +
		"transitions:\n  - {state: Ready, signal: Seed, next: Done}\n"
}

func TestStageFragmentVariantIsSelectedByEnvironment(t *testing.T) {
	root := writeStageFixture(t, map[string]string{
		"stages/s-a.yaml": stageVariantA,
		"stages/s-b.yaml": stageVariantB,
		"machine.yaml":    envStageMachine("stages/s-${" + envStageSelector + ":-a}.yaml"),
	})
	for _, test := range []struct {
		name, value, state, fragment string
		set                          bool
	}{
		{name: "unset takes the default", state: "ARunning", fragment: "s-a.yaml"},
		{name: "set selects its variant", value: "b", set: true, state: "BSkipped", fragment: "s-b.yaml"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.set {
				t.Setenv(envStageSelector, test.value)
			}
			spec, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

			require.NoError(t, err)
			require.Contains(t, spec.States.Names(), test.state)
			require.Len(t, spec.States, 4, "one variant is spliced, never both")
			instantiations := spec.Instantiations()
			require.Len(t, instantiations, 1)
			require.Equal(t, filepath.Join(root, "stages", test.fragment), instantiations[0].Fragment,
				"the dump names the resolved variant")
		})
	}
}

func TestSelectedStageVariantIsValidatedAsOneMachine(t *testing.T) {
	root := writeStageFixture(t, map[string]string{
		"stages/s-a.yaml":      stageVariantA,
		"stages/s-broken.yaml": stageVariantBroken,
		"machine.yaml":         envStageMachine("stages/s-${" + envStageSelector + ":-a}.yaml"),
	})
	t.Setenv(envStageSelector, "broken")

	_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

	require.ErrorContains(t, err, `next "Nowhere" not in states list`)
	require.ErrorContains(t, err, "after splicing")
	require.ErrorContains(t, err, filepath.Join(root, "stages", "s-broken.yaml"))
}

func TestTemplatedStagePathsThatCannotSelectAVariantFail(t *testing.T) {
	for _, test := range []struct {
		name, fragment string
		files          map[string]string
		want           string
	}{
		{name: "no default", fragment: "stages/s-${" + envStageSelector + "}.yaml",
			files: map[string]string{"stages/s-a.yaml": stageVariantA},
			want:  "needs the form ${NAME:-default}"},
		{name: "malformed reference", fragment: "stages/s-${not valid}.yaml",
			files: map[string]string{"stages/s-a.yaml": stageVariantA},
			want:  "is not ${NAME:-default}"},
		{name: "default missing", fragment: "stages/s-${" + envStageSelector + ":-a}.yaml",
			files: map[string]string{"stages/s-b.yaml": stageVariantB},
			want:  "default variant stages/s-a.yaml is missing"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv(envStageSelector, "b")
			files := map[string]string{"machine.yaml": envStageMachine(test.fragment)}
			for name, body := range test.files {
				files[name] = body
			}
			root := writeStageFixture(t, files)

			_, err := core.LoadMachineClosure(filepath.Join(root, "machine.yaml"), nil)

			require.ErrorContains(t, err, test.want)
			require.ErrorContains(t, err, test.fragment, "the error names the path as written")
		})
	}
}
