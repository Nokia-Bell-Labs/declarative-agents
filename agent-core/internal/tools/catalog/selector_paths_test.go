// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

func pathRegistry(t *testing.T) *typesys.Registry {
	t.Helper()
	registry, err := typesys.Build(typesys.TypeUnitFile{
		Unit: "chat-types",
		Types: []typesys.TypeDecl{
			{Name: "Rows", Schema: map[string]any{
				"type": "array",
				"items": map[string]any{
					"type":       "object",
					"properties": map[string]any{"text": map[string]any{"type": "string"}},
				},
			}},
			{Name: "Request", Schema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"prompt": map[string]any{"type": "string"}},
			}},
		},
	})
	require.NoError(t, err)
	return registry
}

// fetchTool publishes chat-types.Rows, so any label it labels has that shape.
func fetchTool(t *testing.T, registry *typesys.Registry) ToolDef {
	t.Helper()
	defs, _, err := ResolveToolSchemas([]ToolDef{{
		Name: "fetch", Type: "builtin", Init: "compose", Category: "word",
		Description: "Fetch rows.",
		Signature:   &ToolSignature{Output: "chat-types.Rows", Emits: []string{"Fetched"}},
	}}, registry)
	require.NoError(t, err)
	return defs[0]
}

func pathMachine() core.MachineSpec {
	return core.MachineSpec{
		Name: "paths", InitialState: "Start",
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: "Go", Next: "Fetched", Action: "fetch", Label: "fetched"},
			{State: "Fetched", Signal: "Done", Next: "End", Action: "report"},
		},
	}
}

func reportReading(selector string) ToolDef {
	return ToolDef{Name: "report", Config: map[string]interface{}{"source": selector}}
}

func TestLabelTypesDerivesFromThePublishingAction(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	defs := []ToolDef{fetchTool(t, registry)}

	types := LabelTypes(pathMachine(), defs, registry)

	require.Contains(t, types, "fetched", "the transition label takes the action's output type")
	require.Contains(t, types, "fetch", "so does the command name that publishes it")
	require.Equal(t, "array", types["fetched"]["type"])
	require.NotContains(t, types, "report", "an unsigned action publishes an untyped label")
}

func TestValidateSelectorPathsAcceptsAResolvablePath(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	defs := []ToolDef{fetchTool(t, registry), reportReading("$from(fetched).text")}
	require.Empty(t, ValidateSelectorPaths(pathMachine(), defs, registry))
}

func TestValidateSelectorPathsRejectsAFieldTheTypeLacks(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	defs := []ToolDef{fetchTool(t, registry), reportReading("$from(fetched).txet")}

	diagnostics := ValidateSelectorPaths(pathMachine(), defs, registry)

	require.Len(t, diagnostics, 1)
	require.Equal(t, core.DiagnosticSelectorPathMismatch, diagnostics[0].Code)
	require.Equal(t, "report", diagnostics[0].Tool)
	require.Contains(t, diagnostics[0].Message, `has no field "txet"`)
	require.Contains(t, diagnostics[0].Message, `closest declared field is "text"`)
}

// Gradual typing: an unsigned action leaves its label unchecked, so a machine
// converts one tool at a time.
func TestValidateSelectorPathsSkipsUntypedLabels(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	unsigned := ToolDef{Name: "fetch", Type: "builtin", Init: "compose"}
	defs := []ToolDef{unsigned, reportReading("$from(fetched).anything.at.all")}

	require.Empty(t, ValidateSelectorPaths(pathMachine(), defs, registry))
}

func TestValidateSelectorPathsChecksTypedExternalLabels(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	spec := pathMachine()
	spec.ExternalLabels = []core.ExternalLabel{{Name: "seed", Type: "chat-types.Request"}}
	defs := []ToolDef{fetchTool(t, registry), reportReading("$from(seed).promt")}

	diagnostics := ValidateSelectorPaths(spec, defs, registry)

	require.Len(t, diagnostics, 1)
	require.Contains(t, diagnostics[0].Message, `closest declared field is "prompt"`)
}

func TestValidateSelectorPathsLeavesBareExternalLabelsUntyped(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	spec := pathMachine()
	spec.ExternalLabels = []core.ExternalLabel{{Name: "seed"}}
	defs := []ToolDef{fetchTool(t, registry), reportReading("$from(seed).whatever")}

	require.Empty(t, ValidateSelectorPaths(spec, defs, registry),
		"the bare form stays untyped so existing machines are unaffected")
}

func TestValidateSelectorPathsTypesForEachItemLabel(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	spec := pathMachine()
	spec.Transitions = append(spec.Transitions, core.TransitionSpec{
		State: "Fetched", Signal: "Each", Next: "Joined", Action: "fetch",
		ForEach: &core.ForEachSpec{
			Items: "$from(fetched).text", As: "row",
			Join: core.JoinSpec{Next: "Joined", Label: "joined"},
		},
	})
	defs := []ToolDef{fetchTool(t, registry), reportReading("$from(row).txet")}

	diagnostics := ValidateSelectorPaths(spec, defs, registry)

	require.Len(t, diagnostics, 1)
	require.Contains(t, diagnostics[0].Message, `has no field "txet"`,
		"an item label takes the element type of the array it iterates")
}

func TestValidateSelectorPathsTypesTheJoinEnvelope(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	spec := pathMachine()
	spec.Transitions = append(spec.Transitions, core.TransitionSpec{
		State: "Fetched", Signal: "Each", Next: "Joined", Action: "fetch",
		ForEach: &core.ForEachSpec{
			Items: "$from(fetched).text", As: "row",
			Join: core.JoinSpec{Next: "Joined", Label: "joined"},
		},
	})

	types := LabelTypes(spec, []ToolDef{fetchTool(t, registry)}, registry)

	require.Contains(t, types, "joined")
	// The envelope mirrors iteratorJoinResult: aggregate counts beside one
	// outcome per dispatched item, the action's own output under
	// result.structured_output.
	require.NoError(t, typesys.CheckPath(types["joined"], []string{"succeeded"}))
	require.NoError(t, typesys.CheckPath(types["joined"], []string{"items", "command_name"}))
	require.NoError(t, typesys.CheckPath(types["joined"], []string{"items", "0", "result", "signal"}))
	require.NoError(t, typesys.CheckPath(types["joined"],
		[]string{"items", "result", "structured_output", "text"}))
	require.Error(t, typesys.CheckPath(types["joined"], []string{"itmes"}),
		"the envelope is a real shape, not an escape hatch")
}

func TestValidateSelectorPathsStrictNamesEveryMismatch(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	defs := []ToolDef{fetchTool(t, registry), reportReading("$from(fetched).txet")}

	err := ValidateSelectorPathsStrict(pathMachine(), defs, registry)

	require.ErrorContains(t, err, "selector path mismatches")
	require.ErrorContains(t, err, `$from(fetched).txet`)
}

// reportBindingParameter is the request-binding shape: the word takes its
// argument from a command-state selector rather than from config.
func reportBindingParameter(selector string) ToolDef {
	return ToolDef{Name: "report", Parameters: map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text": map[string]interface{}{
				"type": "string", "positional": true, "position": 1, "source": selector,
			},
		},
	}}
}

func TestValidateSelectorPathsAcceptsAResolvableParameterSource(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	defs := []ToolDef{fetchTool(t, registry), reportBindingParameter("$from(fetched).text")}
	require.Empty(t, ValidateSelectorPaths(pathMachine(), defs, registry))
}

// TestValidateSelectorPathsRejectsABadParameterSource is the GH-2057
// regression for the path half: the selector was never collected, so a field
// the type does not have went unreported.
func TestValidateSelectorPathsRejectsABadParameterSource(t *testing.T) {
	t.Parallel()
	registry := pathRegistry(t)
	defs := []ToolDef{fetchTool(t, registry), reportBindingParameter("$from(fetched).txet")}

	diagnostics := ValidateSelectorPaths(pathMachine(), defs, registry)

	require.Len(t, diagnostics, 1)
	require.Equal(t, core.DiagnosticSelectorPathMismatch, diagnostics[0].Code)
	require.Equal(t, "report", diagnostics[0].Tool)
	require.Contains(t, diagnostics[0].Message, `has no field "txet"`)
}
