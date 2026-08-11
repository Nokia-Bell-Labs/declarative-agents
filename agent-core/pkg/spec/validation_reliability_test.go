// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestToolDeclSideEffectsParsesLegacyOrRejectsInvalidShape(t *testing.T) {
	t.Parallel()
	var legacy ToolDeclaration
	require.NoError(t, yaml.Unmarshal([]byte(
		"name: read\nside_effects: reads one file\n",
	), &legacy))
	require.Equal(t, "reads one file", legacy.SideEffects.LegacyText)
	require.NotContains(t, missingToolContractFields(legacy), "side_effects")

	var invalid ToolDeclaration
	err := yaml.Unmarshal([]byte("name: read\nside_effects: {kind: filesystem_read}\n"), &invalid)
	require.ErrorContains(t, err, "side_effects must be a string or list")
}

func TestToolSelectionParseErrorsNameFileAndDuplicatePathIsOneConsumer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "tools.yaml")
	require.NoError(t, os.WriteFile(path, []byte("tools: [[[\n"), 0o600))
	selections, seen := map[string][]string{}, map[string]bool{}
	err := addToolSelection(selections, seen, "agent", path)
	require.ErrorContains(t, err, "parse tool selection")
	require.ErrorContains(t, err, path)

	require.NoError(t, os.WriteFile(path, []byte("tools: [read]\n"), 0o600))
	require.NoError(t, addToolSelection(selections, seen, "agent", path))
	require.NoError(t, addToolSelection(selections, seen, "agent:tools", path))
	require.Equal(t, map[string][]string{"agent": {"read"}}, selections)
}

func TestDocSpecFlexibleFieldsRejectUnsupportedShapes(t *testing.T) {
	t.Parallel()
	var doc DocSpec
	err := yaml.Unmarshal([]byte(
		"id: example\nrequirements_source: invalid\nimplementation: {path: x}\n",
	), &doc)
	require.Error(t, err)
}

func TestPublicToolDeclarationVocabularyIsValidated(t *testing.T) {
	t.Parallel()
	corpus := &Corpus{ToolDeclarations: map[string]ToolDeclaration{
		"bad": {
			Name: "bad", Type: "builtin", Init: "done", Visibility: "internl",
			Reversibility: ToolDeclReversibility{Classification: "reversable"},
		},
	}}
	findings := checkToolDeclarationVocabulary(corpus)
	require.NotEmpty(t, findings)
	require.Equal(t, "tool-declaration-invalid", findings[0].Check)
	require.Equal(t, "error", findings[0].Level)
}

func TestDiagnosticSeverityAndSelectableCodesStayDerivedFromCore(t *testing.T) {
	t.Parallel()
	require.Equal(t, "warning", machineDiagnosticLevel(core.MachineDiagnosticWarning))
	require.Equal(t, "error", machineDiagnosticLevel(core.MachineDiagnosticSeverity("error")))
	for _, code := range core.MachineDiagnosticCodes() {
		require.True(t, supportedSpecCorpusCheckIDs["machine-diagnostic-"+code], code)
	}
}
