// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package dolt

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParameterValidatorRequiredUnknownAndNestedSchema(t *testing.T) {
	t.Parallel()

	schema := objectSchema(map[string]interface{}{
		"id": map[string]interface{}{"type": "integer", "minimum": 1},
		"metadata": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"labels": map[string]interface{}{
					"type":  "array",
					"items": map[string]interface{}{"type": "string"},
				},
			},
			"required":             []interface{}{"labels"},
			"additionalProperties": false,
		},
	}, "id", "metadata")
	validator, err := CompileParameterSchema("nested", schema)
	require.NoError(t, err)
	require.NoError(t, validator.Validate(map[string]interface{}{
		"id": float64(7),
		"metadata": map[string]interface{}{
			"labels": []interface{}{"safe", "bound"},
		},
	}))

	err = validator.Validate(map[string]interface{}{"id": float64(7)})
	require.ErrorContains(t, err, "parameter_schema")
	err = validator.Validate(map[string]interface{}{
		"id":       float64(7),
		"metadata": map[string]interface{}{"labels": []interface{}{"ok"}, "extra": true},
	})
	require.ErrorContains(t, err, "additional properties")
	err = validator.Validate(map[string]interface{}{
		"id": float64(7), "metadata": map[string]interface{}{"labels": []interface{}{"ok"}},
		"extra": true,
	})
	require.ErrorContains(t, err, `runtime parameter "extra" is not declared`)
}

func TestRuntimeInputRejectsAuthorityFields(t *testing.T) {
	t.Parallel()

	for _, field := range []string{
		"statement", "sql", "query", "dsn", "connection", "database", "schema",
		"commit_message", "host", "port", "user", "password", "tls", "client_cert",
	} {
		field := field
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			err := ValidateRuntimeInput(map[string]interface{}{field: "caller-value"})
			require.ErrorContains(t, err, "cannot set Dolt authority")
		})
	}

	err := ValidateRuntimeInput(map[string]interface{}{
		"metadata": map[string]interface{}{"tls_policy": "skip-verify"},
	})
	require.ErrorContains(t, err, "runtime input.metadata.tls_policy")
}

func TestParameterSchemaCannotDeclareAuthority(t *testing.T) {
	t.Parallel()

	_, err := CompileParameterSchema("authority", objectSchema(map[string]interface{}{
		"password": map[string]interface{}{"type": "string"},
	}))
	require.ErrorContains(t, err, "declares Dolt authority")

	_, err = CompileParameterSchema("untyped", objectSchema(map[string]interface{}{
		"value": map[string]interface{}{"description": "no runtime type"},
	}))
	require.ErrorContains(t, err, "must declare a type")
}

func TestStructuredParamsBindAsOrderedJSONArguments(t *testing.T) {
	t.Parallel()

	schema := objectSchema(map[string]interface{}{
		"payload": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"name": map[string]interface{}{"type": "string"},
			},
			"required": []interface{}{"name"},
		},
	}, "payload")
	cfg := baseConfig(KindWrite)
	cfg.ParameterSchema = schema
	cfg.Statement = "INSERT INTO events (payload, copy) VALUES (:payload, :payload)"
	cfg.CommitMessage = "Insert event"
	prepared, err := PrepareConfig("structured", cfg)
	require.NoError(t, err)

	query, args, err := prepared.Bind(map[string]interface{}{
		"payload": map[string]interface{}{"name": "quoted ' value"},
	})
	require.NoError(t, err)
	require.Equal(t, "INSERT INTO events (payload, copy) VALUES (?, ?)", query)
	require.Equal(t, []byte(`{"name":"quoted ' value"}`), args[0])
	require.Equal(t, args[0], args[1])
}

func TestCommitTemplateUsesDeclaredValidatedParamsOnly(t *testing.T) {
	t.Parallel()

	cfg := baseConfig(KindWrite)
	cfg.CommitMessage = "Delete {{ id }}"
	prepared, err := PrepareConfig("commit", cfg)
	require.NoError(t, err)
	message, err := prepared.RenderCommitMessage(map[string]interface{}{"id": "T-42"})
	require.NoError(t, err)
	require.Equal(t, "Delete T-42", message)

	cfg.CommitMessage = "Delete {{ params.unknown }}"
	_, err = PrepareConfig("unknown", cfg)
	require.ErrorContains(t, err, `placeholder "unknown" is not declared`)

	cfg.CommitMessage = "Delete {{ params.id | printf }}"
	_, err = PrepareConfig("malformed", cfg)
	require.ErrorContains(t, err, "malformed placeholder")
}
