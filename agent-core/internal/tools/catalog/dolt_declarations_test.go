// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestDoltDeclarationsExposeCompleteDistinctWords(t *testing.T) {
	t.Parallel()

	defs, err := LoadToolDeclarations([]string{doltDeclarationPath(t)})
	require.NoError(t, err)
	require.Len(t, defs, 3)

	expectedEmits := map[string][]string{
		"dolt_provision": {"DoltProvisioned", "CommandError"},
		"dolt_query":     {"DoltRowsRead", "CommandError"},
		"dolt_write":     {"DoltCommitted", "DoltNoChange", "CommandError"},
	}
	for name, emits := range expectedEmits {
		def := toolDefByName(t, defs, name)
		require.Equal(t, name, def.Init)
		require.Equal(t, "boundary", def.Category)
		require.ElementsMatch(t, emits, def.Emits)
		require.Equal(t, "object", def.Parameters["type"])
		require.Equal(t, "object", def.Output.Schema["type"])
		require.NotEmpty(t, def.Output.Schema["required"])
		require.NotEmpty(t, def.SideEffects.Items)
		require.NotEmpty(t, def.Errors)
		require.NotEmpty(t, def.Relationships)
	}
}

func TestDoltDeclarationsPassContractAndReceiptValidation(t *testing.T) {
	t.Parallel()

	defs, err := LoadToolDeclarations([]string{doltDeclarationPath(t)})
	require.NoError(t, err)

	findings := ValidateToolContracts(defs, ContractValidationOptions{
		Strict:                       true,
		MinimumLevel:                 ContractSeverityError,
		RequireStructuredSideEffects: true,
	})
	require.Empty(t, findings)
	require.NoError(t, ValidateReceiptContracts(defs))

	for _, name := range []string{"dolt_provision", "dolt_write"} {
		def := toolDefByName(t, defs, name)
		require.Equal(t, "compensatable", def.Reversibility.Classification)
		require.Equal(t, "compensating_action", def.Undo.Strategy)
		require.ElementsMatch(t, []string{"operation", "database", "server", "commit_hash"}, def.Undo.Captures)
		require.Contains(t, def.Undo.Requires, "receipt")
	}
	query := toolDefByName(t, defs, "dolt_query")
	require.Equal(t, "reversible", query.Reversibility.Classification)
	require.Equal(t, "noop", query.Undo.Strategy)
}

func TestDoltDeclarationsRemainOptIn(t *testing.T) {
	t.Parallel()

	root := filepath.Dir(builtinBundlePath(t))
	allPath := filepath.Join(root, "builtin", "all.yaml")
	data, err := os.ReadFile(allPath)
	require.NoError(t, err)
	var all ToolDefsFile
	require.NoError(t, yaml.Unmarshal(data, &all))
	require.NotContains(t, all.Includes, "dolt/all.yaml")

	defs, err := LoadToolDeclarations([]string{builtinBundlePath(t)})
	require.NoError(t, err)
	for _, name := range []string{"dolt_provision", "dolt_query", "dolt_write"} {
		for _, def := range defs {
			require.NotEqual(t, name, def.Name)
		}
	}
}

func doltDeclarationPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(
		filepath.Dir(builtinBundlePath(t)),
		"builtin", "dolt", "all.yaml",
	)
}
