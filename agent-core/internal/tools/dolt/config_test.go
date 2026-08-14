// Copyright (c) 2026 Nokia. All rights reserved.

package dolt

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestDecodeConfigStrictAndValid(t *testing.T) {
	t.Parallel()

	def := catalog.ToolDef{Name: "find_ticket", Config: map[string]interface{}{
		"connection_ref": "DOLT_DOMAIN_DSN",
		"database":       "agent_domain",
		"operation":      "find_ticket",
		"kind":           "query",
		"statement":      "SELECT id FROM tickets WHERE status = :status",
		"parameter_schema": objectSchema(map[string]interface{}{
			"status": map[string]interface{}{"type": "string"},
		}, "status"),
		"max_rows":  100,
		"max_bytes": 65536,
		"timeout":   "5s",
	}}
	cfg, err := DecodeConfig(def)
	require.NoError(t, err)
	require.Equal(t, KindQuery, cfg.Kind)
	require.Equal(t, "SELECT id FROM tickets WHERE status = ?", cfg.SQL.Query)
	require.Equal(t, []string{"status"}, cfg.SQL.Names)

	def.Config["unexpected"] = true
	_, err = DecodeConfig(def)
	require.ErrorContains(t, err, `unknown field "unexpected"`)
}

func TestPrepareConfigRejectsUnsafeStatements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		kind      OperationKind
		statement string
		want      string
	}{
		{name: "empty", kind: KindQuery, statement: " ", want: "must not be empty"},
		{name: "multiple", kind: KindQuery, statement: "SELECT 1; DELETE FROM t", want: "multiple SQL statements"},
		{name: "query mutation", kind: KindQuery, statement: "DELETE FROM t WHERE id = :id", want: "operation-kind mismatch"},
		{name: "write read", kind: KindWrite, statement: "SELECT * FROM t WHERE id = :id", want: "operation-kind mismatch"},
		{name: "select outfile", kind: KindQuery, statement: "SELECT * INTO OUTFILE '/tmp/x' FROM t", want: "potentially mutating"},
		{name: "mutating cte", kind: KindQuery, statement: "WITH x AS (SELECT 1) DELETE FROM t", want: "mutating common-table"},
		{name: "dolt control function", kind: KindQuery, statement: "SELECT DOLT_COMMIT('-am', 'unsafe')", want: "dolt control function"},
		{name: "executable comment", kind: KindQuery, statement: "SELECT 1 /*! INTO OUTFILE '/tmp/x' */", want: "executable SQL comments"},
		{name: "unnamed placeholder", kind: KindQuery, statement: "SELECT * FROM t WHERE id = ?", want: "unnamed SQL placeholders"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := baseConfig(tc.kind)
			cfg.Statement = tc.statement
			_, err := PrepareConfig("unsafe", cfg)
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestPrepareConfigValidatesPlaceholdersAndRequiredParams(t *testing.T) {
	t.Parallel()

	cfg := baseConfig(KindQuery)
	cfg.Statement = "SELECT * FROM tickets WHERE id = :missing"
	_, err := PrepareConfig("undeclared", cfg)
	require.ErrorContains(t, err, `placeholder "missing" is not declared`)

	cfg.Statement = "SELECT * FROM tickets"
	_, err = PrepareConfig("unused", cfg)
	require.ErrorContains(t, err, "required parameters are unused")
}

func TestPrepareConfigProvisionSchemaKind(t *testing.T) {
	t.Parallel()

	cfg := baseConfig(KindProvision)
	prepared, err := PrepareConfig("provision", cfg)
	require.NoError(t, err)
	require.Nil(t, prepared.SQL.Names)

	cfg.SchemaStatements = []string{"DROP DATABASE other"}
	_, err = PrepareConfig("provision", cfg)
	require.ErrorContains(t, err, "unsupported SQL statement")
}

func TestNamedPlaceholderRewriteIgnoresTrustedLiterals(t *testing.T) {
	t.Parallel()

	cfg := baseConfig(KindQuery)
	cfg.Statement = "SELECT ':not_a_param', id FROM tickets WHERE id = :id OR parent_id = :id;"
	prepared, err := PrepareConfig("literal", cfg)
	require.NoError(t, err)
	require.Equal(t,
		"SELECT ':not_a_param', id FROM tickets WHERE id = ? OR parent_id = ?;",
		prepared.SQL.Query,
	)

	query, args, err := prepared.Bind(map[string]interface{}{"id": "x' OR 1=1 --"})
	require.NoError(t, err)
	require.Equal(t, prepared.SQL.Query, query)
	require.Equal(t, []interface{}{"x' OR 1=1 --", "x' OR 1=1 --"}, args)
	require.NotContains(t, query, "OR 1=1")

	cfg.Statement = "SELECT id /* ; :ignored */ FROM tickets WHERE id = :id -- ; :ignored\n;"
	prepared, err = PrepareConfig("comments", cfg)
	require.NoError(t, err)
	require.Equal(t, []string{"id"}, prepared.SQL.Names)
	require.Contains(t, prepared.SQL.Query, "/* ; :ignored */")
}

func baseConfig(kind OperationKind) OperationConfig {
	cfg := OperationConfig{
		ConnectionRef: "DOLT_DOMAIN_DSN",
		Database:      "agent_domain",
		Operation:     "operation_one",
		Kind:          kind,
		Timeout:       "5s",
		ParameterSchema: objectSchema(map[string]interface{}{
			"id": map[string]interface{}{"type": "string"},
		}, "id"),
	}
	switch kind {
	case KindProvision:
		cfg.ParameterSchema = objectSchema(map[string]interface{}{})
		cfg.SchemaStatements = []string{"CREATE TABLE IF NOT EXISTS tickets (id VARCHAR(64) PRIMARY KEY)"}
		cfg.CommitMessage = "Provision tickets"
	case KindQuery:
		cfg.Statement = "SELECT * FROM tickets WHERE id = :id"
		cfg.MaxRows = 100
		cfg.MaxBytes = 65536
	case KindWrite:
		cfg.Statement = "DELETE FROM tickets WHERE id = :id"
		cfg.CommitMessage = "Delete {{ params.id }}"
	}
	return cfg
}

func objectSchema(properties map[string]interface{}, required ...string) map[string]interface{} {
	items := make([]interface{}, len(required))
	for i := range required {
		items[i] = required[i]
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   items,
	}
}
