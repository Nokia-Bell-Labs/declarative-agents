// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func structuredView(label, raw string) core.CommandStateView {
	output, _ := json.Marshal(map[string]interface{}{"response": raw})
	return core.NewCommandStateView(core.Execution{{
		CommandName: label,
		Result: core.ResultDigest{
			Output: string(output), RedactionVersion: core.OutputRedactionVersion1,
			RedactionStatus: core.OutputRedactionApplied,
		},
	}})
}

func structuredSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	schema, err := CompileStructuredSchema("parse_structured", map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"names": map[string]interface{}{
				"type": "array", "items": map[string]interface{}{"type": "string"},
			},
		},
		"required": []interface{}{"names"},
	})
	require.NoError(t, err)
	return schema
}

func TestParseStructuredValidatesModelOutput(t *testing.T) {
	schema := structuredSchema(t)
	cmd := ParseStructuredBuilder{
		ToolName: "parse_structured", Source: "$from(route).response",
		Schema: schema, Parsed: "Parsed", Unparsed: "Unparsed",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(structuredView("route", `{"names":["a","b"]}`))
	res := cmd.Execute()
	require.NoError(t, res.Err)
	require.Equal(t, core.Signal("Parsed"), res.Signal)
	require.JSONEq(t, `{"names":["a","b"]}`, res.Output)
}

func TestParseStructuredValidatesAdjacentSeedOutput(t *testing.T) {
	cmd := ParseStructuredBuilder{
		ToolName: "validate_request", Source: "$.output",
		Schema: structuredSchema(t), Parsed: "Parsed", Unparsed: "Unparsed",
	}.Build(core.Result{Signal: core.Seed, Output: `{"names":["seed"]}`})

	res := cmd.Execute()

	require.NoError(t, res.Err)
	require.Equal(t, core.Signal("Parsed"), res.Signal)
	require.JSONEq(t, `{"names":["seed"]}`, res.Output)
}

func TestParseStructuredRejectsInvalidAdjacentOutput(t *testing.T) {
	for _, raw := range []string{`{"names":`, `{"names":[1]}`, `{}`} {
		cmd := ParseStructuredBuilder{
			ToolName: "validate_request", Source: "$.output",
			Schema: structuredSchema(t), Parsed: "Parsed", Unparsed: "Unparsed",
		}.Build(core.Result{Signal: core.Seed, Output: raw})

		res := cmd.Execute()

		require.Equal(t, core.Signal("Unparsed"), res.Signal)
		require.NoError(t, res.Err, "invalid adjacent content is a modeled outcome")
		require.NotEmpty(t, res.Output)
	}
}

func TestParseStructuredMalformedAndInvalidUseFailureSignal(t *testing.T) {
	for _, raw := range []string{`{"names":`, `{"names":[1]}`, `{}`} {
		cmd := ParseStructuredBuilder{
			ToolName: "parse_structured", Source: "$from(route).response",
			Schema: structuredSchema(t), Parsed: "Parsed", Unparsed: "Unparsed",
		}.Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(structuredView("route", raw))
		res := cmd.Execute()
		require.Equal(t, core.Signal("Unparsed"), res.Signal)
		require.NoError(t, res.Err, "content failures are modeled outcomes, not CommandError")
		require.NotEmpty(t, res.Output)
	}
}

func TestParseStructuredUnresolvedSelectorIsCommandError(t *testing.T) {
	cmd := ParseStructuredBuilder{
		ToolName: "parse_structured", Source: "$from(missing).response",
		Schema: structuredSchema(t), Parsed: "Parsed", Unparsed: "Unparsed",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(structuredView("other", `{}`))
	res := cmd.Execute()
	require.Equal(t, core.CommandError, res.Signal)
	var unresolved *core.UnresolvedLabelError
	require.True(t, errors.As(res.Err, &unresolved))
}

func TestCompileStructuredSchemaRejectsMalformedSchema(t *testing.T) {
	_, err := CompileStructuredSchema("parse_structured", map[string]interface{}{"type": 7})
	require.Error(t, err)
}
