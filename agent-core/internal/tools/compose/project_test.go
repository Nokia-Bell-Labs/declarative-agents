// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func runProject(t *testing.T, rows, field string) core.Result {
	t.Helper()
	cmd := ProjectBuilder{
		ToolName: "project", Items: "$from(rows).items", Field: field, Signal: "Projected",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(core.Entry{
		CommandName: "rows",
		Result:      core.ResultDigest{Output: `{"items":` + rows + `}`},
	}))
	return cmd.Execute()
}

func TestProjectPreservesOrderedValuesAndJSONShapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, rows, field, expected string
	}{
		{
			name: "flat strings and duplicates",
			rows: `[{"id":"b"},{"id":"a"},{"id":"a"}]`, field: "id",
			expected: `{"items":["b","a","a"],"count":3}`,
		},
		{
			name:  "nested object path",
			rows:  `[{"metadata":{"source":{"id":"x"}}},{"metadata":{"source":{"id":"y"}}}]`,
			field: "metadata.source.id", expected: `{"items":["x","y"],"count":2}`,
		},
		{
			name: "numeric path and array values",
			rows: `[{"embeddings":[[1,2]]},{"embeddings":[[3,4]]}]`, field: "embeddings.0",
			expected: `{"items":[[1,2],[3,4]],"count":2}`,
		},
		{
			name: "non-string object values",
			rows: `[{"payload":{"score":1}},{"payload":{"score":2}}]`, field: "payload",
			expected: `{"items":[{"score":1},{"score":2}],"count":2}`,
		},
		{
			name: "empty input",
			rows: `[]`, field: "id", expected: `{"items":[],"count":0}`,
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := runProject(t, tc.rows, tc.field)
			require.NoError(t, result.Err)
			require.Equal(t, core.Signal("Projected"), result.Signal)
			require.JSONEq(t, tc.expected, result.Output)
		})
	}
}

func TestProjectRejectsInvalidInputsAndPaths(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, rows, field, message string
	}{
		{name: "non-array input", rows: `{}`, field: "id", message: "want array"},
		{name: "missing field", rows: `[{"name":"x"}]`, field: "id", message: `field "id" is absent`},
		{name: "scalar row", rows: `["x"]`, field: "id", message: `cannot be read from string`},
		{name: "invalid array index", rows: `[{"ids":["x"]}]`, field: "ids.2", message: "outside array length 1"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result := runProject(t, tc.rows, tc.field)
			require.Equal(t, core.CommandError, result.Signal)
			require.ErrorContains(t, result.Err, tc.message)
			require.Empty(t, result.Diagnostics)
		})
	}
}

func TestProjectPreservesUnresolvedSelectorCause(t *testing.T) {
	t.Parallel()
	cmd := ProjectBuilder{
		ToolName: "project", Items: "$from(missing).items", Field: "id", Signal: "Projected",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom())

	result := cmd.Execute()

	var unresolved *core.UnresolvedLabelError
	require.True(t, errors.As(result.Err, &unresolved))
	require.Equal(t, core.CommandError, result.Signal)
}

func TestValidateProjectConfig(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateProjectConfig(
		"project", "$from(rows).items", "metadata.ids.0", "Projected",
	))
	require.Error(t, ValidateProjectConfig("project", "$.items", "id", "Projected"))
	require.Error(t, ValidateProjectConfig("project", "$from(rows).items", "", "Projected"))
	require.Error(t, ValidateProjectConfig("project", "$from(rows).items", "metadata..id", "Projected"))
	require.Error(t, ValidateProjectConfig("project", "$from(rows).items", "id", ""))
}

func TestProjectUndoIsNoop(t *testing.T) {
	t.Parallel()
	cmd := ProjectBuilder{ToolName: "project"}.Build(core.Result{})
	require.Equal(t, core.ToolDone, cmd.Undo(core.Result{}).Signal)
}
