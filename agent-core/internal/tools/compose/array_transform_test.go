// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package compose

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestFlatMapFlattensParallelNestedArraysAndCarriesParentFields(t *testing.T) {
	t.Parallel()
	cmd := FlatMapBuilder{
		ToolName: "flatten_chunks",
		Items:    "$from(join_fanout).sources",
		ElementFields: map[string]string{
			"id": "ids.0", "text": "documents.0",
		},
		CarryFields: map[string]string{"source": "name"},
		Signal:      "ChunksFlattened",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(core.Entry{
		CommandName: "join_fanout",
		Result: core.ResultDigest{Output: `{"sources":[
			{"name":"rag0","ids":[["doc-3","doc-7"]],"documents":[["passage three","passage seven"]]},
			{"name":"rag1","ids":[["doc-9"]],"documents":[["passage nine"]]}
		]}`},
	}))

	res := cmd.Execute()

	require.NoError(t, res.Err)
	require.Equal(t, core.Signal("ChunksFlattened"), res.Signal)
	require.JSONEq(t, `{"items":[
		{"id":"doc-3","source":"rag0","text":"passage three"},
		{"id":"doc-7","source":"rag0","text":"passage seven"},
		{"id":"doc-9","source":"rag1","text":"passage nine"}
	],"count":3}`, res.Output)
}

func TestFlatMapEmptyElementArrayProducesEmptyRows(t *testing.T) {
	t.Parallel()
	cmd := FlatMapBuilder{
		ToolName: "flatten", Items: "$from(source).items",
		ElementFields: map[string]string{"id": "ids.0"},
		CarryFields:   map[string]string{"source": "name"},
		Signal:        "Flattened",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(core.Entry{
		CommandName: "source",
		Result:      core.ResultDigest{Output: `{"items":[{"name":"rag0","ids":[[]]}]}`},
	}))

	res := cmd.Execute()

	require.NoError(t, res.Err)
	require.JSONEq(t, `{"items":[],"count":0}`, res.Output)
}

func TestFlatMapRejectsMismatchedParallelArrays(t *testing.T) {
	t.Parallel()
	cmd := FlatMapBuilder{
		ToolName: "flatten", Items: "$from(source).items",
		ElementFields: map[string]string{"id": "ids.0", "text": "documents.0"},
		Signal:        "Flattened",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(core.Entry{
		CommandName: "source",
		Result: core.ResultDigest{Output: `{"items":[{
			"ids":[["a","b"]],"documents":[["only one"]]
		}]}`},
	}))

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.ErrorContains(t, res.Err, `element field "text" has length 1, want 2`)
}

func TestFlatMapPreservesUnresolvedSelectorCause(t *testing.T) {
	t.Parallel()
	cmd := FlatMapBuilder{
		ToolName: "flatten", Items: "$from(missing).items",
		ElementFields: map[string]string{"id": "ids.0"},
		Signal:        "Flattened",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom())

	res := cmd.Execute()

	var unresolved *core.UnresolvedLabelError
	require.True(t, errors.As(res.Err, &unresolved))
	require.Equal(t, core.CommandError, res.Signal)
}

func TestReorderByIndexJoinsRankRowsToCandidates(t *testing.T) {
	t.Parallel()
	cmd := ReorderByIndexBuilder{
		ToolName: "apply_rerank", Items: "$from(flatten).items",
		Order: "$from(rerank).results", IndexField: "index", Signal: "Reranked",
	}.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(
		core.Entry{
			CommandName: "flatten",
			Result: core.ResultDigest{Output: `{"items":[
				{"id":"a","text":"alpha"},{"id":"b","text":"beta"},{"id":"c","text":"gamma"}
			]}`},
		},
		core.Entry{
			CommandName: "rerank",
			Result: core.ResultDigest{Output: `{"results":[
				{"index":2,"relevance_score":0.98},{"index":0,"relevance_score":0.71}
			]}`},
		},
	))

	res := cmd.Execute()

	require.NoError(t, res.Err)
	require.Equal(t, core.Signal("Reranked"), res.Signal)
	require.JSONEq(t, `{
		"items":[{"id":"c","text":"gamma"},{"id":"a","text":"alpha"}],
		"rows":[{"index":2,"relevance_score":0.98},{"index":0,"relevance_score":0.71}],
		"count":2
	}`, res.Output)
}

func TestReorderByIndexRejectsInvalidIndexes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		results string
		message string
	}{
		{name: "fractional", results: `[{"index":0.5}]`, message: "want integer"},
		{name: "outside", results: `[{"index":2}]`, message: "outside candidates length 2"},
		{name: "duplicate", results: `[{"index":1},{"index":1}]`, message: "repeats candidate index 1"},
		{name: "missing", results: `[{"score":1}]`, message: `field "index" is absent`},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cmd := ReorderByIndexBuilder{
				ToolName: "reorder", Items: "$from(candidates).items",
				Order: "$from(order).results", IndexField: "index", Signal: "Reordered",
			}.Build(core.Result{})
			cmd.(core.CommandStateAware).SetCommandState(viewFrom(
				core.Entry{
					CommandName: "candidates",
					Result:      core.ResultDigest{Output: `{"items":["a","b"]}`},
				},
				core.Entry{
					CommandName: "order",
					Result:      core.ResultDigest{Output: `{"results":` + tt.results + `}`},
				},
			))

			res := cmd.Execute()

			require.Equal(t, core.CommandError, res.Signal)
			require.ErrorContains(t, res.Err, tt.message)
		})
	}
}

func TestValidateArrayTransformConfig(t *testing.T) {
	t.Parallel()
	require.NoError(t, ValidateFlatMapConfig(
		"flatten", "$from(source).items",
		map[string]string{"id": "ids.0"}, map[string]string{"source": "name"}, "Flattened",
	))
	require.Error(t, ValidateFlatMapConfig(
		"flatten", "$.items", map[string]string{"id": "ids.0"}, nil, "Flattened",
	))
	require.Error(t, ValidateFlatMapConfig(
		"flatten", "$from(source).items", nil, nil, "Flattened",
	))
	require.Error(t, ValidateFlatMapConfig(
		"flatten", "$from(source).items",
		map[string]string{"id": "ids.0"}, map[string]string{"id": "name"}, "Flattened",
	))
	require.Error(t, ValidateFlatMapConfig(
		"flatten", "$from(source).items", map[string]string{"id": "ids..0"}, nil, "Flattened",
	))

	require.NoError(t, ValidateReorderByIndexConfig(
		"reorder", "$from(candidates).items", "$from(rerank).results", "index", "Reordered",
	))
	require.Error(t, ValidateReorderByIndexConfig(
		"reorder", "$.items", "$from(rerank).results", "index", "Reordered",
	))
	require.Error(t, ValidateReorderByIndexConfig(
		"reorder", "$from(candidates).items", "$from(rerank).results", "", "Reordered",
	))
	require.Error(t, ValidateReorderByIndexConfig(
		"reorder", "$from(candidates).items", "$from(rerank).results", "index", "",
	))
}
