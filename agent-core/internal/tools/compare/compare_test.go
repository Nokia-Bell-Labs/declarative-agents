// Copyright (c) 2026 Nokia. All rights reserved.

package compare

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func viewFrom(entries ...core.Entry) core.CommandStateView {
	return core.NewCommandStateView(core.Execution(entries))
}

// embeddingModelEntries stands in for the chatbot's case: the query embedding
// reports the model it was produced with, and a RAG source reports the model
// its collection was built with (srd002 R3.3).
func embeddingModelEntries(queryModel, ragModel string) []core.Entry {
	return []core.Entry{
		{CommandName: "embed_query", Result: core.ResultDigest{
			Output: `{"mapped":{"model":"` + queryModel + `"}}`}},
		{CommandName: "rag_query0", Result: core.ResultDigest{
			Output: `{"mapped":{"embedding_model":"` + ragModel + `"}}`}},
	}
}

func embeddingModelCompare() core.Command {
	return Builder{
		ToolName: "compare_embedding_model",
		Left:     "$from(embed_query).mapped.model",
		Right:    "$from(rag_query0).mapped.embedding_model",
		Matched:  "ModelMatched",
		Differed: "ModelDiffered",
	}.Build(core.Result{})
}

// TestCompareEmitsMatchedOnEqualValues proves a compatible source produces the
// configured matched signal.
func TestCompareEmitsMatchedOnEqualValues(t *testing.T) {
	cmd := embeddingModelCompare()
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(
		embeddingModelEntries("qwen3-embedding:8b", "qwen3-embedding:8b")...))

	res := cmd.Execute()
	require.Equal(t, core.Signal("ModelMatched"), res.Signal)
	require.NoError(t, res.Err)
	require.Contains(t, res.Output, `"verdict":"matched"`)
}

// TestCompareEmitsDifferedOnUnequalValues proves the case the mapped-400 path
// cannot catch: identical vector dimensions, different model identities.
func TestCompareEmitsDifferedOnUnequalValues(t *testing.T) {
	cmd := embeddingModelCompare()
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(
		embeddingModelEntries("qwen3-embedding:8b", "nomic-embed-text:v1.5")...))

	res := cmd.Execute()
	require.Equal(t, core.Signal("ModelDiffered"), res.Signal)
	require.NoError(t, res.Err)
	require.Contains(t, res.Output, `"verdict":"differed"`)
	require.Contains(t, res.Output, "qwen3-embedding:8b")
	require.Contains(t, res.Output, "nomic-embed-text:v1.5")
}

// TestCompareUnresolvedSelectorEmitsCommandError proves a missing operand is
// distinct from a differed verdict. A comparison that cannot resolve an operand
// has no defensible verdict, so it must not read as "these differ".
func TestCompareUnresolvedSelectorEmitsCommandError(t *testing.T) {
	for _, tt := range []struct {
		name    string
		entries []core.Entry
	}{
		{name: "left missing", entries: []core.Entry{
			{CommandName: "rag_query0", Result: core.ResultDigest{
				Output: `{"mapped":{"embedding_model":"qwen3-embedding:8b"}}`}},
		}},
		{name: "right missing", entries: []core.Entry{
			{CommandName: "embed_query", Result: core.ResultDigest{
				Output: `{"mapped":{"model":"qwen3-embedding:8b"}}`}},
		}},
		{name: "both missing"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := embeddingModelCompare()
			cmd.(core.CommandStateAware).SetCommandState(viewFrom(tt.entries...))

			res := cmd.Execute()
			require.Equal(t, core.CommandError, res.Signal)
			require.Error(t, res.Err)
			require.NotEqual(t, core.Signal("ModelDiffered"), res.Signal)
		})
	}
}

// TestCompareEmptySelectorEmitsCommandError proves an unconfigured operand
// fails loudly rather than comparing against the empty string.
func TestCompareEmptySelectorEmitsCommandError(t *testing.T) {
	cmd := Builder{ToolName: "compare_embedding_model", Right: "$from(rag_query0).mapped.embedding_model"}.
		Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFrom(
		embeddingModelEntries("qwen3-embedding:8b", "qwen3-embedding:8b")...))

	res := cmd.Execute()
	require.Equal(t, core.CommandError, res.Signal)
	require.ErrorContains(t, res.Err, "left selector is empty")
}

// TestCompareDefaultSignals proves a word that configures no signal names still
// emits a usable verdict pair.
func TestCompareDefaultSignals(t *testing.T) {
	build := func() core.Command {
		return Builder{
			ToolName: "compare_embedding_model",
			Left:     "$from(embed_query).mapped.model",
			Right:    "$from(rag_query0).mapped.embedding_model",
		}.Build(core.Result{})
	}

	matched := build()
	matched.(core.CommandStateAware).SetCommandState(viewFrom(
		embeddingModelEntries("a", "a")...))
	require.Equal(t, defaultMatchedSignal, matched.Execute().Signal)

	differed := build()
	differed.(core.CommandStateAware).SetCommandState(viewFrom(
		embeddingModelEntries("a", "b")...))
	require.Equal(t, defaultDifferedSignal, differed.Execute().Signal)
}

// TestCompareStructuredValues proves non-string operands compare by content
// rather than by Go identity, so the word is not limited to string fields.
func TestCompareStructuredValues(t *testing.T) {
	build := func() core.Command {
		return Builder{
			ToolName: "compare_dims",
			Left:     "$from(a).mapped.dims",
			Right:    "$from(b).mapped.dims",
			Matched:  "DimsMatched",
			Differed: "DimsDiffered",
		}.Build(core.Result{})
	}

	same := build()
	same.(core.CommandStateAware).SetCommandState(viewFrom(
		core.Entry{CommandName: "a", Result: core.ResultDigest{Output: `{"mapped":{"dims":[1,2,3]}}`}},
		core.Entry{CommandName: "b", Result: core.ResultDigest{Output: `{"mapped":{"dims":[1,2,3]}}`}},
	))
	require.Equal(t, core.Signal("DimsMatched"), same.Execute().Signal)

	differs := build()
	differs.(core.CommandStateAware).SetCommandState(viewFrom(
		core.Entry{CommandName: "a", Result: core.ResultDigest{Output: `{"mapped":{"dims":[1,2,3]}}`}},
		core.Entry{CommandName: "b", Result: core.ResultDigest{Output: `{"mapped":{"dims":[1,2,4]}}`}},
	))
	require.Equal(t, core.Signal("DimsDiffered"), differs.Execute().Signal)
}

// TestCompareUndoIsNoop proves comparing state has no durable effect to undo.
func TestCompareUndoIsNoop(t *testing.T) {
	cmd := embeddingModelCompare()
	require.Equal(t, core.NoopUndo("compare_embedding_model"), cmd.Undo(core.Result{}))
}
