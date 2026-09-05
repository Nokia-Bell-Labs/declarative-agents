// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	tooldolt "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/dolt"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestDoltSelectedInitRegistersWholeFactoryFamily(t *testing.T) {
	t.Parallel()

	families := standardCatalog(&agentState{})
	require.Len(t, families, 13)
	var doltFamily toolregistry.StandardFactoryCatalogEntry
	for _, family := range families {
		if family.Name == "dolt" {
			doltFamily = family
			break
		}
	}
	require.True(t, doltFamily.SelectedBy(map[string]bool{tooldolt.InitQuery: true}))

	builtins := toolregistry.NewBuiltinRegistry()
	registerBuiltinFactories(builtins, &agentState{}, map[string]bool{tooldolt.InitQuery: true})

	require.ElementsMatch(t, []string{
		tooldolt.InitProvision,
		tooldolt.InitQuery,
		tooldolt.InitWrite,
	}, builtins.Names())
}

func TestDoltFactoryRejectsCheckpointDatabaseCollision(t *testing.T) {
	t.Parallel()

	err := registerDoltQueryForCheckpoint(t, "Runtime_State")

	require.Error(t, err)
	require.ErrorContains(t, err, `database "runtime_state" collides with the active Dolt checkpoint`)
	require.NotContains(t, err.Error(), "secret")
}

func TestDoltFactoryAllowsSameServerDifferentDatabase(t *testing.T) {
	t.Parallel()

	err := registerDoltQueryForCheckpoint(t, "domain_data")

	require.NoError(t, err)
}

func TestDoltFactoryDefersBadCheckpointDSNUntilBuild(t *testing.T) {
	t.Parallel()

	builtins := toolregistry.NewBuiltinRegistry()
	require.NotPanics(t, func() {
		registerBuiltinFactories(builtins, &agentState{doltDSN: "not-a-dsn"}, map[string]bool{tooldolt.InitQuery: true})
	})
	require.ElementsMatch(t, []string{
		tooldolt.InitProvision,
		tooldolt.InitQuery,
		tooldolt.InitWrite,
	}, builtins.Names())

	err := toolregistry.RegisterSingleBuiltin(
		core.NewRegistry(),
		builtins,
		catalog.ToolDef{
			Name: "lookup_records",
			Type: "builtin",
			Init: tooldolt.InitQuery,
			Config: map[string]interface{}{
				"connection_ref": "DOLT_WORD_DSN",
				"database":       "domain_data",
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
	require.Error(t, err)
	require.ErrorContains(t, err, "resolve active Dolt checkpoint identity")
}

func registerDoltQueryForCheckpoint(t *testing.T, database string) error {
	t.Helper()
	builtins := toolregistry.NewBuiltinRegistry()
	state := newAgentState(runtimeConfig{
		Checkpoint: checkpoint.Config{DoltDSN: "checkpoint:secret@tcp(LOCALHOST:3306)/runtime_state"},
		DoltConnections: map[string]string{
			"DOLT_WORD_DSN": "word:secret@tcp(localhost:3306)/ignored",
		},
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
