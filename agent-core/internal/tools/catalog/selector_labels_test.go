// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func labelMachine() core.MachineSpec {
	return core.MachineSpec{
		Name:         "labels",
		InitialState: "Start",
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "Begin", Next: "Fetched", Action: "fetch", Label: "fetched"},
			{State: "Fetched", Signal: "Done", Next: "End", Action: "report"},
		},
	}
}

func TestMachineLabelsCollectsEveryPublisher(t *testing.T) {
	t.Parallel()
	spec := labelMachine()
	spec.ExternalLabels = []core.ExternalLabel{{Name: "seeded_request"}}
	spec.Transitions = append(spec.Transitions, core.TransitionSpec{
		State: "Fetched", Signal: "Each", Next: "Joined", Action: "step",
		ForEach: &core.ForEachSpec{
			Items: "$from(fetched).items",
			As:    "item",
			Join:  core.JoinSpec{Next: "Joined", Label: "joined"},
		},
	})

	labels := catalog.MachineLabels(spec)

	// Command names are addresses too: a step stays addressable by the command
	// that ran it whether or not its transition carries a label.
	for _, want := range []string{
		"fetched", "seeded_request", "item", "joined", "fetch", "report", "step",
	} {
		_, ok := labels[want]
		require.Truef(t, ok, "expected %q in the machine label universe", want)
	}
	require.Len(t, labels, 7)
}

func TestSelectorRefsFindsSelectorsAtAnyConfigDepth(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{
		Name:        "deep",
		StdinSource: "$from(fetched).body",
		Config: map[string]interface{}{
			"source":  "$from(fetched).payload",
			"literal": "not a selector",
			"current": "$.output",
			"nested": map[string]interface{}{
				"operands": []interface{}{"$from(joined).total", 7, "plain"},
			},
		},
	}

	require.Equal(t, []string{
		"$from(fetched).body", "$from(fetched).payload", "$from(joined).total",
	}, def.SelectorRefs())
}

func TestSelectorRefsIgnoresNonSelectorStrings(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{
		Name: "plain",
		Config: map[string]interface{}{
			"prompt":  "Describe $from the beginning",
			"current": "$.output",
			"empty":   "",
		},
	}
	require.Empty(t, def.SelectorRefs())
}

func TestValidateSelectorLabelsAcceptsDeclaredLabels(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name:   "report",
		Config: map[string]interface{}{"source": "$from(fetched).body"},
	}}
	require.Empty(t, catalog.ValidateSelectorLabels(labelMachine(), defs))
}

func TestValidateSelectorLabelsReportsUnknownToolLabel(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name:   "report",
		Config: map[string]interface{}{"source": "$from(fetchedd).body"},
	}}

	diagnostics := catalog.ValidateSelectorLabels(labelMachine(), defs)

	require.Len(t, diagnostics, 1)
	require.Equal(t, core.DiagnosticUnresolvedSelectorLabel, diagnostics[0].Code)
	require.Equal(t, "report", diagnostics[0].Tool)
	require.Contains(t, diagnostics[0].Message, `tool "report"`)
	require.Contains(t, diagnostics[0].Message, `"fetchedd"`)
	require.Contains(t, diagnostics[0].Message, `closest declared label is "fetched"`)
}

func TestValidateSelectorLabelsReportsUnknownForEachItems(t *testing.T) {
	t.Parallel()
	spec := labelMachine()
	spec.Transitions = append(spec.Transitions, core.TransitionSpec{
		State: "Fetched", Signal: "Each", Next: "Joined", Action: "step",
		ForEach: &core.ForEachSpec{
			Items: "$from(missing).items",
			As:    "item",
			Join:  core.JoinSpec{Next: "Joined"},
		},
	})

	diagnostics := catalog.ValidateSelectorLabels(spec, nil)

	require.Len(t, diagnostics, 1)
	require.Equal(t, "Fetched", diagnostics[0].State)
	require.Contains(t, diagnostics[0].Message, "machine selector")
	require.Contains(t, diagnostics[0].Message, `"missing"`)
}

func TestValidateSelectorLabelsCountsExternalLabels(t *testing.T) {
	t.Parallel()
	spec := labelMachine()
	defs := []catalog.ToolDef{{
		Name:   "report",
		Config: map[string]interface{}{"source": "$from(seeded_request).body"},
	}}

	require.Len(t, catalog.ValidateSelectorLabels(spec, defs), 1,
		"an unseeded label is unresolved")

	spec.ExternalLabels = []core.ExternalLabel{{Name: "seeded_request"}}
	require.Empty(t, catalog.ValidateSelectorLabels(spec, defs),
		"declaring the runtime-seeded label resolves it")
}

func TestValidateSelectorLabelsSkipsUnreachableTools(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name:   "never_dispatched",
		Config: map[string]interface{}{"source": "$from(missing).body"},
	}}
	require.Empty(t, catalog.ValidateSelectorLabels(labelMachine(), defs),
		"a tool no transition names cannot fail the machine")
}

func TestValidateSelectorLabelsOmitsDistantSuggestion(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name:   "report",
		Config: map[string]interface{}{"source": "$from(completely_unrelated).body"},
	}}

	diagnostics := catalog.ValidateSelectorLabels(labelMachine(), defs)

	require.Len(t, diagnostics, 1)
	require.NotContains(t, diagnostics[0].Message, "closest declared label")
}

func TestValidateSelectorLabelsStrictPassesDeclaredLabels(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name:   "report",
		Config: map[string]interface{}{"source": "$from(fetched).body"},
	}}
	require.NoError(t, catalog.ValidateSelectorLabelsStrict(labelMachine(), defs))
}

func TestValidateSelectorLabelsStrictNamesEveryUnresolvedReference(t *testing.T) {
	t.Parallel()
	spec := labelMachine()
	spec.Transitions = append(spec.Transitions, core.TransitionSpec{
		State: "Fetched", Signal: "Each", Next: "Joined", Action: "step",
		ForEach: &core.ForEachSpec{
			Items: "$from(absent).items", As: "item",
			Join: core.JoinSpec{Next: "Joined"},
		},
	})
	defs := []catalog.ToolDef{{
		Name:   "report",
		Config: map[string]interface{}{"source": "$from(fetchedd).body"},
	}}

	err := catalog.ValidateSelectorLabelsStrict(spec, defs)

	require.Error(t, err)
	require.Contains(t, err.Error(), "unresolved selector labels")
	require.Contains(t, err.Error(), `"absent"`)
	require.Contains(t, err.Error(), `"fetchedd"`)
	require.Contains(t, err.Error(), `closest declared label is "fetched"`)
}

func TestMachineLabelsAddressesUnlabeledTransitionByCommandName(t *testing.T) {
	t.Parallel()
	// The chatbot-mesh observer machine reaches $from(discover_mesh_pods)
	// where that name is an action on unlabeled transitions, not a label
	// (srd006 R1.6, srd038 R2.7).
	spec := core.MachineSpec{
		Name:         "unlabeled",
		InitialState: "Start",
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "Go", Next: "Discovered", Action: "discover_pods"},
		},
	}
	defs := []catalog.ToolDef{{
		Name:   "discover_pods",
		Config: map[string]interface{}{"items": "$from(discover_pods).mapped.pods"},
	}}
	require.NoError(t, catalog.ValidateSelectorLabelsStrict(spec, defs))
}

func TestValidateSelectorLabelsAdmitsDynamicDispatchNames(t *testing.T) {
	t.Parallel()
	spec := labelMachine()
	spec.Transitions = append(spec.Transitions, core.TransitionSpec{
		State: "Fetched", Signal: "Pick", Next: "Done", Action: "$tool",
	})
	defs := []catalog.ToolDef{
		{Name: "report", Config: map[string]interface{}{"source": "$from(chosen_at_runtime).body"}},
		{Name: "chosen_at_runtime"},
	}
	require.Empty(t, catalog.ValidateSelectorLabels(spec, defs),
		"a $tool transition can execute any selected tool, so its name is addressable")
}

func TestValidateSelectorLabelsReportsRepeatedSelectorOnce(t *testing.T) {
	t.Parallel()
	spec := labelMachine()
	for i := 0; i < 3; i++ {
		spec.Transitions = append(spec.Transitions, core.TransitionSpec{
			State: "Fetched", Signal: "Each", Next: "Joined", Action: "step",
			ForEach: &core.ForEachSpec{
				Items: "$from(absent).items", As: "item",
				Join: core.JoinSpec{Next: "Joined"},
			},
		})
	}
	require.Len(t, catalog.ValidateSelectorLabels(spec, nil), 1,
		"one selector repeated across transitions is one finding")
}

// requestBoundParameters is the shape a request binding declares: the word's
// arguments, each taking its value from a command-state selector. Four
// documentation-curator words carry their only selectors here (GH-2057).
func requestBoundParameters(selector string) map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type": "string", "positional": true, "position": 1, "source": selector,
			},
		},
		"required": []interface{}{"query"},
	}
}

func TestValidateSelectorLabelsAcceptsADeclaredParameterSource(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name: "report", Parameters: requestBoundParameters("$from(fetched).body"),
	}}
	require.Empty(t, catalog.ValidateSelectorLabels(labelMachine(), defs))
}

// TestValidateSelectorLabelsReportsAnUnknownParameterSourceLabel is the
// GH-2057 regression: a selector reachable only through parameters was never
// collected, so the check passed on a label nothing publishes.
func TestValidateSelectorLabelsReportsAnUnknownParameterSourceLabel(t *testing.T) {
	t.Parallel()
	defs := []catalog.ToolDef{{
		Name: "report", Parameters: requestBoundParameters("$from(fetchedd).body"),
	}}

	diagnostics := catalog.ValidateSelectorLabels(labelMachine(), defs)

	require.Len(t, diagnostics, 1)
	require.Equal(t, core.DiagnosticUnresolvedSelectorLabel, diagnostics[0].Code)
	require.Equal(t, "report", diagnostics[0].Tool)
	require.Contains(t, diagnostics[0].Message, `"fetchedd"`)
}

// TestSelectorRefsReadsEveryCarrier keeps the doc comment honest: it named the
// parameter sources before the walk reached them.
func TestSelectorRefsReadsEveryCarrier(t *testing.T) {
	t.Parallel()
	def := catalog.ToolDef{
		StdinSource: "$from(wrapped).payload",
		Config:      map[string]interface{}{"items": "$from(rows).items"},
		Parameters:  requestBoundParameters("$from(request).query"),
	}

	require.Equal(t, []string{
		"$from(request).query", "$from(rows).items", "$from(wrapped).payload",
	}, def.SelectorRefs())
}

// seedReadingTool is what a request-scoped word does with the entry the
// runtime publishes for it: reads a request field out of the seed.
func seedReadingTool() []catalog.ToolDef {
	return []catalog.ToolDef{{
		Name:   "report",
		Config: map[string]interface{}{"left": "$from(seed).parameters.group_by"},
	}}
}

// TestValidateSelectorLabelsAcceptsTheRequestSeedLabel covers the reason five
// correct profiles were rejected: srd038 R2.10 reserves the machine_request
// entry under seed, which the runtime publishes and no transition does.
func TestValidateSelectorLabelsAcceptsTheRequestSeedLabel(t *testing.T) {
	t.Parallel()
	require.Empty(t, catalog.ValidateSelectorLabels(
		labelMachine(), seedReadingTool(), catalog.RequestSeedLabel))
}

// TestValidateSelectorLabelsRejectsSeedOnAMachineNothingSeeds is the other
// half. Accepting seed everywhere would have hidden a real unresolved label on
// the machine a profile runs, where nothing publishes it.
func TestValidateSelectorLabelsRejectsSeedOnAMachineNothingSeeds(t *testing.T) {
	t.Parallel()
	diagnostics := catalog.ValidateSelectorLabels(labelMachine(), seedReadingTool())

	require.Len(t, diagnostics, 1)
	require.Equal(t, core.DiagnosticUnresolvedSelectorLabel, diagnostics[0].Code)
	require.Contains(t, diagnostics[0].Message, `"seed"`)
}

// TestRequestSeedLabelIsTheReservedName keeps the constant and srd038 R2.10
// from drifting apart; the runtime publishes this exact name.
func TestRequestSeedLabelIsTheReservedName(t *testing.T) {
	t.Parallel()
	require.Equal(t, "seed", catalog.RequestSeedLabel)
}
