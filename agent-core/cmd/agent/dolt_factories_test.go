// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	tooldolt "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/dolt"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestDoltSelectedInitRegistersWholeFactoryFamily(t *testing.T) {
	t.Parallel()

	families := builtinFactoryCatalog(&agentState{})
	require.Len(t, families, 12)
	var doltFamily builtinFactoryCatalogEntry
	for _, family := range families {
		if family.Name == "dolt" {
			doltFamily = family
			break
		}
	}
	require.True(t, doltFamily.selectedBy(map[string]bool{tooldolt.InitQuery: true}))

	builtins := toolregistry.NewBuiltinRegistry()
	registerBuiltinFactories(builtins, &agentState{}, map[string]bool{tooldolt.InitQuery: true})

	require.ElementsMatch(t, []string{
		tooldolt.InitProvision,
		tooldolt.InitQuery,
		tooldolt.InitWrite,
	}, builtins.Names())
}

func TestDoltFactoryRejectsCheckpointDatabaseCollision(t *testing.T) {
	t.Setenv("DOLT_WORD_DSN", "word:secret@tcp(localhost:3306)/ignored")

	err := registerDoltQueryForCheckpoint(t, "Runtime_State")

	require.Error(t, err)
	require.ErrorContains(t, err, `database "runtime_state" collides with the active Dolt checkpoint`)
	require.NotContains(t, err.Error(), "secret")
}

func TestDoltFactoryAllowsSameServerDifferentDatabase(t *testing.T) {
	t.Setenv("DOLT_WORD_DSN", "word:secret@tcp(localhost:3306)/ignored")

	err := registerDoltQueryForCheckpoint(t, "domain_data")

	require.NoError(t, err)
}

func registerDoltQueryForCheckpoint(t *testing.T, database string) error {
	t.Helper()
	builtins := toolregistry.NewBuiltinRegistry()
	state := newAgentState(runtimeConfig{
		DoltDSN: "checkpoint:secret@tcp(LOCALHOST:3306)/runtime_state",
	}, agentStateDeps{})
	registerBuiltinFactories(builtins, state, map[string]bool{tooldolt.InitQuery: true})
	return toolregistry.RegisterSingleBuiltin(
		core.NewRegistry(),
		builtins,
		catalog.ToolDef{
			Name: "lookup_records",
			Type: "builtin",
			Init: tooldolt.InitQuery,
			Config: map[string]interface{}{
				"connection_ref": "DOLT_WORD_DSN",
				"database":       database,
				"operation":      "lookup_records",
				"kind":           "query",
				"statement":      "SELECT 1",
				"parameter_schema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
				"max_rows":  10,
				"max_bytes": 1024,
				"timeout":   "1s",
			},
		},
		nil,
	)
}
