// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	rtcheckpoint "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint"
	doltcheckpoint "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/checkpoint/dolt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	tooldolt "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/dolt"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	doltWordConnectionRef = "DOLT_WORD_INTEGRATION_DSN"
	doltWordDatabase      = "word_store"
	doltCheckpointDB      = "checkpoint_store"
)

// TestDoltWordIntegration exercises declarations through the production
// registry and SQL opener against a throwaway real Dolt server. Unit tests use
// fake Database implementations for fault isolation; this gate is the wire-level
// proof that configured provision, query, and write words compose with Dolt.
func TestDoltWordIntegration(t *testing.T) {
	base := startDoltServer(t)
	connections := map[string]string{doltWordConnectionRef: base}

	provision := configuredDoltWord(t, "provision_records", tooldolt.InitProvision, map[string]interface{}{
		"connection_ref": doltWordConnectionRef,
		"database":       doltWordDatabase,
		"operation":      "provision_records",
		"kind":           "provision",
		"schema_statements": []interface{}{
			"CREATE TABLE IF NOT EXISTS records (record_key VARCHAR(128) PRIMARY KEY, category VARCHAR(64) NOT NULL, quantity BIGINT NOT NULL)",
			"CREATE TABLE IF NOT EXISTS record_notes (note_key VARCHAR(128) PRIMARY KEY, record_key VARCHAR(128) NOT NULL, note_value VARCHAR(255) NOT NULL, FOREIGN KEY (record_key) REFERENCES records(record_key))",
		},
		"parameter_schema": doltWordParameterSchema(map[string]interface{}{}),
		"timeout":          "30s",
		"commit_message":   "Provision generic record schema",
	})
	incompatibleProvision := configuredDoltWord(t, "provision_incompatible_schema", tooldolt.InitProvision, map[string]interface{}{
		"connection_ref":    doltWordConnectionRef,
		"database":          doltWordDatabase,
		"operation":         "provision_incompatible_schema",
		"kind":              "provision",
		"schema_statements": []interface{}{"ALTER TABLE records ADD COLUMN category VARCHAR(64)"},
		"parameter_schema":  doltWordParameterSchema(map[string]interface{}{}),
		"timeout":           "30s",
		"commit_message":    "Apply incompatible record schema",
	})
	insert := configuredDoltWord(t, "insert_record", tooldolt.InitWrite, map[string]interface{}{
		"connection_ref": doltWordConnectionRef,
		"database":       doltWordDatabase,
		"operation":      "insert_record",
		"kind":           "write",
		"statement":      "INSERT INTO records (record_key, category, quantity) VALUES (:record_key, :category, :quantity)",
		"parameter_schema": doltWordParameterSchema(map[string]interface{}{
			"record_key": map[string]interface{}{"type": "string"},
			"category":   map[string]interface{}{"type": "string"},
			"quantity":   map[string]interface{}{"type": "integer"},
		}, "record_key", "category", "quantity"),
		"timeout":        "30s",
		"commit_message": "Store record {{ params.record_key }}",
	})
	query := configuredDoltWord(t, "query_records", tooldolt.InitQuery, map[string]interface{}{
		"connection_ref": doltWordConnectionRef,
		"database":       doltWordDatabase,
		"operation":      "query_records",
		"kind":           "query",
		"statement": strings.Join([]string{
			"SELECT record_key, category, quantity FROM records WHERE record_key = :record_key",
			"UNION ALL",
			"SELECT record_key, category, quantity FROM records WHERE record_key = :record_key",
			"ORDER BY record_key",
		}, " "),
		"parameter_schema": doltWordParameterSchema(map[string]interface{}{
			"record_key": map[string]interface{}{"type": "string"},
		}, "record_key"),
		"max_rows":  1,
		"max_bytes": 2048,
		"timeout":   "30s",
	})
	updateMissing := configuredDoltWord(t, "update_missing_record", tooldolt.InitWrite, map[string]interface{}{
		"connection_ref": doltWordConnectionRef,
		"database":       doltWordDatabase,
		"operation":      "update_missing_record",
		"kind":           "write",
		"statement":      "UPDATE records SET category = :category WHERE record_key = :record_key",
		"parameter_schema": doltWordParameterSchema(map[string]interface{}{
			"record_key": map[string]interface{}{"type": "string"},
			"category":   map[string]interface{}{"type": "string"},
		}, "record_key", "category"),
		"timeout":             "30s",
		"commit_message":      "Update record {{ params.record_key }}",
		"commit_on_no_change": false,
	})

	registry, err := registerConfiguredDoltWords(
		&agentState{doltConnections: connections}, provision, incompatibleProvision, insert, query, updateMissing,
	)
	require.NoError(t, err)

	t.Run("provisions ordered schema idempotently", func(t *testing.T) {
		first := executeRegisteredDoltWord(t, registry, provision.Name, map[string]interface{}{})
		require.Equal(t, tooldolt.SignalDoltProvisioned, first.Signal, first.Output)
		var firstOutput doltProvisionIntegrationOutput
		require.NoError(t, json.Unmarshal([]byte(first.Output), &firstOutput))
		require.Equal(t, "provision_records", firstOutput.Operation)
		require.Equal(t, doltWordDatabase, firstOutput.Database)
		require.True(t, firstOutput.Created)
		require.True(t, firstOutput.SchemaChanged)
		require.NotEmpty(t, firstOutput.CommitHash)
		require.Equal(t, 1, doltCommitMessageCount(
			t, base, doltWordDatabase, "Provision generic record schema",
		))

		require.Equal(t,
			[]string{"record_key", "category", "quantity"},
			doltTableColumns(t, base, doltWordDatabase, "records"),
		)
		require.Equal(t,
			[]string{"note_key", "record_key", "note_value"},
			doltTableColumns(t, base, doltWordDatabase, "record_notes"),
		)

		beforeRepeat := doltCommitCount(t, base, doltWordDatabase)
		second := executeRegisteredDoltWord(t, registry, provision.Name, map[string]interface{}{})
		require.Equal(t, tooldolt.SignalDoltProvisioned, second.Signal, second.Output)
		var secondOutput doltProvisionIntegrationOutput
		require.NoError(t, json.Unmarshal([]byte(second.Output), &secondOutput))
		require.False(t, secondOutput.Created)
		require.False(t, secondOutput.SchemaChanged)
		require.Empty(t, secondOutput.CommitHash)
		require.Equal(t, beforeRepeat, doltCommitCount(t, base, doltWordDatabase))
	})

	t.Run("incompatible schema creates no commit", func(t *testing.T) {
		before := doltCommitCount(t, base, doltWordDatabase)
		result := executeRegisteredDoltWord(t, registry, incompatibleProvision.Name, map[string]interface{}{})
		require.Equal(t, core.CommandError, result.Signal, result.Output)
		require.Equal(t, "schema_apply", doltFailureStage(t, result))
		require.Equal(t, before, doltCommitCount(t, base, doltWordDatabase))
	})

	boundKey := "bound' OR 1=1 --"
	t.Run("binds writes and creates one declared commit", func(t *testing.T) {
		before := doltCommitCount(t, base, doltWordDatabase)
		result := executeRegisteredDoltWord(t, registry, insert.Name, map[string]interface{}{
			"parameters": map[string]interface{}{
				"record_key": boundKey,
				"category":   "general",
				"quantity":   7,
			},
		})
		require.Equal(t, tooldolt.SignalDoltCommitted, result.Signal, result.Output)
		var output doltWriteIntegrationOutput
		require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
		require.Equal(t, int64(1), output.RowsAffected)
		require.True(t, output.Committed)
		require.NotEmpty(t, output.CommitHash)
		require.Equal(t, before+1, doltCommitCount(t, base, doltWordDatabase))

		wantMessage := "Store record " + boundKey
		require.Equal(t, wantMessage, doltCommitMessage(t, base, doltWordDatabase, output.CommitHash))
		require.Equal(t, 1, doltCommitMessageCount(t, base, doltWordDatabase, wantMessage))
		require.Equal(t, 1, doltRecordCount(t, base, doltWordDatabase, boundKey))
	})

	t.Run("returns ordered bounded structured query rows without commit", func(t *testing.T) {
		before := doltCommitCount(t, base, doltWordDatabase)
		result := executeRegisteredDoltWord(t, registry, query.Name, map[string]interface{}{
			"parameters": map[string]interface{}{"record_key": boundKey},
		})
		require.Equal(t, tooldolt.SignalDoltRowsRead, result.Signal, result.Output)
		var output doltQueryIntegrationOutput
		require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
		require.Equal(t, "query_records", output.Operation)
		require.Equal(t, doltWordDatabase, output.Database)
		require.Equal(t, []string{"record_key", "category", "quantity"}, doltColumnNames(output.Columns))
		require.Equal(t, 1, output.RowCount)
		require.True(t, output.Truncated)
		require.Len(t, output.Rows, 1)
		require.Equal(t, boundKey, output.Rows[0]["record_key"])
		require.Equal(t, "general", output.Rows[0]["category"])
		require.Equal(t, float64(7), output.Rows[0]["quantity"])
		require.Equal(t, before, doltCommitCount(t, base, doltWordDatabase))
	})

	t.Run("zero row write reports no change and creates no commit", func(t *testing.T) {
		before := doltCommitCount(t, base, doltWordDatabase)
		result := executeRegisteredDoltWord(t, registry, updateMissing.Name, map[string]interface{}{
			"parameters": map[string]interface{}{
				"record_key": "absent",
				"category":   "unchanged",
			},
		})
		require.Equal(t, tooldolt.SignalDoltNoChange, result.Signal, result.Output)
		var output doltWriteIntegrationOutput
		require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
		require.Zero(t, output.RowsAffected)
		require.False(t, output.Committed)
		require.Empty(t, output.CommitHash)
		require.Equal(t, before, doltCommitCount(t, base, doltWordDatabase))
	})

	t.Run("refuses runtime authority before mutation execution", func(t *testing.T) {
		before := doltCommitCount(t, base, doltWordDatabase)
		for _, field := range []string{"sql", "connection", "database", "schema", "commit_message"} {
			field := field
			t.Run(field, func(t *testing.T) {
				key := "blocked_" + field
				input := map[string]interface{}{
					"parameters": map[string]interface{}{
						"record_key": key,
						"category":   "blocked",
						"quantity":   1,
					},
					field: "caller supplied authority",
				}
				result := executeRegisteredDoltWord(t, registry, insert.Name, input)
				require.Equal(t, core.CommandError, result.Signal, result.Output)
				require.Equal(t, "parameter_validation", doltFailureStage(t, result))
				require.Equal(t, 0, doltRecordCount(t, base, doltWordDatabase, key))
			})
		}
		require.Equal(t, before, doltCommitCount(t, base, doltWordDatabase))
	})

	t.Run("failed mutation rolls back without a commit", func(t *testing.T) {
		before := doltCommitCount(t, base, doltWordDatabase)
		result := executeRegisteredDoltWord(t, registry, insert.Name, map[string]interface{}{
			"parameters": map[string]interface{}{
				"record_key": boundKey,
				"category":   "duplicate",
				"quantity":   99,
			},
		})
		require.Equal(t, core.CommandError, result.Signal, result.Output)
		require.Equal(t, "statement_execute", doltFailureStage(t, result))
		require.Equal(t, before, doltCommitCount(t, base, doltWordDatabase))
		require.Equal(t, 1, doltRecordCount(t, base, doltWordDatabase, boundKey))
	})

	t.Run("accepts separate checkpoint database on same server", func(t *testing.T) {
		requireDoltDatabase(t, base, doltCheckpointDB)
		checkpoint, err := doltcheckpoint.OpenDoltCheckpoint(
			base+doltCheckpointDB,
			"dolt-word-integration",
			func(core.State) bool { return false },
		)
		require.NoError(t, err)
		defer func() { require.NoError(t, checkpoint.Close()) }()
		require.NoError(t, checkpoint.Save(
			core.Position{CurrentState: "Working", LastSignal: core.LLMResponded},
			core.Execution{{
				Iteration: 1, CommandName: "record", FromState: "Start", ToState: "Working",
				Signal: core.LLMResponded,
				Result: core.DigestResult(core.Result{
					Signal: core.LLMResponded, Output: `{"stored":true}`,
				}),
			}},
		))

		separateState := newAgentState(
			runtimeConfig{
				Checkpoint:      rtcheckpoint.Config{DoltDSN: base + doltCheckpointDB},
				DoltConnections: connections,
			},
			agentStateDeps{},
		)
		separateRegistry, err := registerConfiguredDoltWords(separateState, query)
		require.NoError(t, err)
		result := executeRegisteredDoltWord(t, separateRegistry, query.Name, map[string]interface{}{
			"parameters": map[string]interface{}{"record_key": boundKey},
		})
		require.Equal(t, tooldolt.SignalDoltRowsRead, result.Signal, result.Output)
	})

	t.Run("rejects exact checkpoint and word database identity at startup", func(t *testing.T) {
		before := doltCommitCount(t, base, doltWordDatabase)
		collidingState := newAgentState(
			runtimeConfig{
				Checkpoint:      rtcheckpoint.Config{DoltDSN: base + doltWordDatabase},
				DoltConnections: connections,
			},
			agentStateDeps{},
		)
		_, err := registerConfiguredDoltWords(collidingState, query)
		require.Error(t, err)
		require.ErrorContains(t, err, `database "word_store" collides with the active Dolt checkpoint`)
		require.NotContains(t, err.Error(), "root@")
		require.Equal(t, before, doltCommitCount(t, base, doltWordDatabase))
	})
}

type doltProvisionIntegrationOutput struct {
	Operation     string `json:"operation"`
	Database      string `json:"database"`
	Created       bool   `json:"created"`
	SchemaChanged bool   `json:"schema_changed"`
	CommitHash    string `json:"commit_hash"`
}

type doltWriteIntegrationOutput struct {
	RowsAffected int64  `json:"rows_affected"`
	CommitHash   string `json:"commit_hash"`
	Committed    bool   `json:"committed"`
}

type doltQueryIntegrationOutput struct {
	Operation string                   `json:"operation"`
	Database  string                   `json:"database"`
	Columns   []tooldolt.Column        `json:"columns"`
	Rows      []map[string]interface{} `json:"rows"`
	RowCount  int                      `json:"row_count"`
	Truncated bool                     `json:"truncated"`
}

func configuredDoltWord(
	t *testing.T,
	name string,
	init string,
	config map[string]interface{},
) catalog.ToolDef {
	t.Helper()
	path := filepath.Join("..", "..", "tools", "builtin", "dolt", "all.yaml")
	defs, err := catalog.LoadToolDeclarations([]string{path})
	require.NoError(t, err)
	for _, def := range defs {
		if def.Init == init {
			def.Name = name
			def.Config = config
			return def
		}
	}
	t.Fatalf("shared Dolt declaration does not contain init %q", init)
	return catalog.ToolDef{}
}

func registerConfiguredDoltWords(
	state *agentState,
	defs ...catalog.ToolDef,
) (*core.Registry, error) {
	builtins := toolregistry.NewBuiltinRegistry()
	registerBuiltinFactories(builtins, state, selectedBuiltinInits(defs))
	registry := core.NewRegistry()
	for _, def := range defs {
		if err := toolregistry.RegisterSingleBuiltin(registry, builtins, def, nil); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func executeRegisteredDoltWord(
	t *testing.T,
	registry *core.Registry,
	name string,
	input map[string]interface{},
) core.Result {
	t.Helper()
	builder, ok := registry.Resolve(name)
	require.Truef(t, ok, "registered Dolt word %q was not found", name)
	data, err := json.Marshal(input)
	require.NoError(t, err)
	return builder.Build(core.Result{Output: string(data)}).Execute()
}

func doltWordParameterSchema(
	properties map[string]interface{},
	required ...string,
) map[string]interface{} {
	requiredValues := make([]interface{}, len(required))
	for i, name := range required {
		requiredValues[i] = name
	}
	return map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"required":             requiredValues,
		"additionalProperties": false,
	}
}

func doltFailureStage(t *testing.T, result core.Result) string {
	t.Helper()
	var output struct {
		FailureStage string `json:"failure_stage"`
	}
	require.NoError(t, json.Unmarshal([]byte(result.Output), &output))
	return output.FailureStage
}

func doltColumnNames(columns []tooldolt.Column) []string {
	names := make([]string, len(columns))
	for i, column := range columns {
		names[i] = column.Name
	}
	return names
}

func requireDoltDatabase(t *testing.T, base, database string) {
	t.Helper()
	db := openDoltIntegrationDB(t, base)
	defer func() { require.NoError(t, db.Close()) }()
	_, err := db.ExecContext(context.Background(), "CREATE DATABASE IF NOT EXISTS `"+database+"`")
	require.NoError(t, err)
}

func doltCommitCount(t *testing.T, base, database string) int {
	t.Helper()
	db := openDoltIntegrationDB(t, base+database)
	defer func() { require.NoError(t, db.Close()) }()
	var count int
	require.NoError(t, db.QueryRowContext(
		context.Background(), "SELECT COUNT(*) FROM dolt_log",
	).Scan(&count))
	return count
}

func doltCommitMessageCount(t *testing.T, base, database, message string) int {
	t.Helper()
	db := openDoltIntegrationDB(t, base+database)
	defer func() { require.NoError(t, db.Close()) }()
	var count int
	require.NoError(t, db.QueryRowContext(
		context.Background(), "SELECT COUNT(*) FROM dolt_log WHERE message = ?", message,
	).Scan(&count))
	return count
}

func doltCommitMessage(t *testing.T, base, database, hash string) string {
	t.Helper()
	db := openDoltIntegrationDB(t, base+database)
	defer func() { require.NoError(t, db.Close()) }()
	var message string
	require.NoError(t, db.QueryRowContext(
		context.Background(), "SELECT message FROM dolt_log WHERE commit_hash = ?", hash,
	).Scan(&message))
	return message
}

func doltTableColumns(t *testing.T, base, database, table string) []string {
	t.Helper()
	db := openDoltIntegrationDB(t, base+database)
	defer func() { require.NoError(t, db.Close()) }()
	rows, err := db.QueryContext(
		context.Background(),
		"SELECT column_name FROM information_schema.columns WHERE table_schema = ? AND table_name = ? ORDER BY ordinal_position",
		database,
		table,
	)
	require.NoError(t, err)
	defer func() { require.NoError(t, rows.Close()) }()
	var columns []string
	for rows.Next() {
		var column string
		require.NoError(t, rows.Scan(&column))
		columns = append(columns, column)
	}
	require.NoError(t, rows.Err())
	return columns
}

func doltRecordCount(t *testing.T, base, database, key string) int {
	t.Helper()
	db := openDoltIntegrationDB(t, base+database)
	defer func() { require.NoError(t, db.Close()) }()
	var count int
	require.NoError(t, db.QueryRowContext(
		context.Background(), "SELECT COUNT(*) FROM records WHERE record_key = ?", key,
	).Scan(&count))
	return count
}

func openDoltIntegrationDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("dolt", dsn)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, db.PingContext(ctx))
	return db
}
