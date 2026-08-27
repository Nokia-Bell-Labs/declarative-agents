// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func runNormalizeVector(output string) core.Result {
	return NormalizeVectorBuilder{
		ToolName: "normalize_vector", Path: "mapped.embedding", Signal: "Normalized",
	}.Build(core.Result{
		Output: output,
		Redaction: core.OutputRedaction{
			Version: 1,
			Paths:   []core.OutputRedactionPath{{"carried", "token"}},
		},
	}).Execute()
}

func TestNormalizeVectorPreservesFlatVectorAndEnvelope(t *testing.T) {
	t.Parallel()
	result := runNormalizeVector(
		`{"mapped":{"embedding":[0.25,0.75],"model":"embed-v4"},` +
			`"carried":{"input":"document","id":"a.md","token":"secret"}}`,
	)

	require.NoError(t, result.Err)
	require.Equal(t, core.Signal("Normalized"), result.Signal)
	require.Equal(t, "normalize_vector", result.CommandName)
	require.JSONEq(t,
		`{"mapped":{"embedding":[0.25,0.75],"model":"embed-v4"},`+
			`"carried":{"input":"document","id":"a.md","token":"secret"}}`,
		result.Output,
	)
	require.Equal(t, core.OutputRedaction{
		Version: 1,
		Paths:   []core.OutputRedactionPath{{"carried", "token"}},
	}, result.Redaction)
}

func TestNormalizeVectorUnwrapsExactlyOneRow(t *testing.T) {
	t.Parallel()
	result := runNormalizeVector(
		`{"mapped":{"embedding":[[0.25,0.75]]},"carried":{"input":"document","id":"a.md"}}`,
	)

	require.NoError(t, result.Err)
	require.JSONEq(t,
		`{"mapped":{"embedding":[0.25,0.75]},"carried":{"input":"document","id":"a.md"}}`,
		result.Output,
	)
}

func TestNormalizeVectorRejectsInvalidShapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, output, message string
	}{
		{name: "empty vector", output: `{"mapped":{"embedding":[]}}`, message: "vector is empty"},
		{name: "empty row", output: `{"mapped":{"embedding":[[]]}}`, message: "matrix row is empty"},
		{
			name: "multiple rows", output: `{"mapped":{"embedding":[[1],[2]]}}`,
			message: "matrix has 2 rows, want exactly one",
		},
		{
			name: "mixed flat and nested", output: `{"mapped":{"embedding":[1,[2]]}}`,
			message: "vector[1] resolved to []interface {}, want number",
		},
		{
			name: "non-numeric flat member", output: `{"mapped":{"embedding":[1,"two"]}}`,
			message: "vector[1] resolved to string, want number",
		},
		{
			name: "non-numeric row member", output: `{"mapped":{"embedding":[[1,true]]}}`,
			message: "vector[1] resolved to bool, want number",
		},
		{
			name: "scalar", output: `{"mapped":{"embedding":"nope"}}`,
			message: "want numeric vector or single-row matrix",
		},
		{name: "missing field", output: `{"mapped":{}}`, message: `field "mapped.embedding" is absent`},
		{
			name: "non-object component", output: `{"mapped":[]}`,
			message: `component "mapped" resolved to []interface {}, want object`,
		},
		{name: "non-object root", output: `[]`, message: "previous result is not an object"},
		{name: "malformed JSON", output: `{`, message: "decode previous result object"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := runNormalizeVector(tc.output)
			require.Equal(t, core.CommandError, result.Signal)
			require.ErrorContains(t, result.Err, tc.message)
			require.Empty(t, result.Diagnostics)
		})
	}
}

func TestValidateNormalizeVectorConfig(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateNormalizeVectorConfig(
		"normalize_vector", "mapped.embedding", "Normalized",
	))
	require.Error(t, ValidateNormalizeVectorConfig("normalize_vector", "", "Normalized"))
	require.Error(t, ValidateNormalizeVectorConfig("normalize_vector", "mapped..embedding", "Normalized"))
	require.Error(t, ValidateNormalizeVectorConfig("normalize_vector", "mapped.embedding", ""))
}

func TestNormalizeVectorUndoIsNoop(t *testing.T) {
	t.Parallel()
	cmd := NormalizeVectorBuilder{ToolName: "normalize_vector"}.Build(core.Result{})
	require.Equal(t, core.ToolDone, cmd.Undo(core.Result{}).Signal)
}
