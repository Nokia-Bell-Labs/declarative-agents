// Copyright (c) 2026 Nokia. All rights reserved.

package ragmerge

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func viewFrom(entries ...core.Entry) core.CommandStateView {
	return core.NewCommandStateView(core.Execution(entries))
}

type mergedOutput struct {
	Mapped struct {
		Documents []string `json:"documents"`
		Degraded  []string `json:"degraded"`
		Excluded  []string `json:"excluded"`
	} `json:"mapped"`
}

func run(t *testing.T, b Builder, view core.CommandStateView) (mergedOutput, core.Result) {
	t.Helper()
	cmd := b.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(view)
	res := cmd.Execute()
	var out mergedOutput
	require.NoError(t, json.Unmarshal([]byte(res.Output), &out))
	return out, res
}

func ragEntry(label, docsJSON string) core.Entry {
	return core.Entry{CommandName: label, Result: core.ResultDigest{Output: docsJSON}}
}

func TestRagMergeOrdersByDistanceWithTags(t *testing.T) {
	b := Builder{
		ToolName:               "rag_merge",
		Sources:                []Source{{Label: "rag0", Tag: "rag0"}, {Label: "rag1", Tag: "rag1"}},
		ExpectedEmbeddingModel: "qwen3-embedding:8b",
		MaxChunks:              8,
		Signal:                 "Merged",
	}
	out, res := run(t, b, viewFrom(
		ragEntry("rag0", `{"mapped":{"documents":[["a-far","a-near"]],"distances":[[0.9,0.1]],"embedding_model":"qwen3-embedding:8b"}}`),
		ragEntry("rag1", `{"mapped":{"documents":[["b-mid"]],"distances":[[0.5]],"embedding_model":"qwen3-embedding:8b"}}`),
	))
	require.Equal(t, core.Signal("Merged"), res.Signal)
	require.Equal(t, []string{"[rag0] a-near", "[rag1] b-mid", "[rag0] a-far"}, out.Mapped.Documents)
	require.Empty(t, out.Mapped.Degraded)
	require.Empty(t, out.Mapped.Excluded)
}

func TestRagMergeCaps(t *testing.T) {
	b := Builder{
		ToolName:  "rag_merge",
		Sources:   []Source{{Label: "rag0", Tag: "rag0"}},
		MaxChunks: 1,
	}
	out, _ := run(t, b, viewFrom(
		ragEntry("rag0", `{"mapped":{"documents":[["near","far"]],"distances":[[0.1,0.9]]}}`),
	))
	require.Equal(t, []string{"[rag0] near"}, out.Mapped.Documents)
}

func TestRagMergeDegradesMissingSource(t *testing.T) {
	b := Builder{
		ToolName: "rag_merge",
		Sources:  []Source{{Label: "rag0", Tag: "rag0"}, {Label: "rag1", Tag: "rag1"}},
	}
	// rag1 is absent from command state (its query failed and the machine degraded).
	out, _ := run(t, b, viewFrom(
		ragEntry("rag0", `{"mapped":{"documents":[["a"]],"distances":[[0.2]]}}`),
	))
	require.Equal(t, []string{"[rag0] a"}, out.Mapped.Documents)
	require.Equal(t, []string{"rag1"}, out.Mapped.Degraded)
}

func TestRagMergeExcludesEmbeddingMismatch(t *testing.T) {
	b := Builder{
		ToolName:               "rag_merge",
		Sources:                []Source{{Label: "rag0", Tag: "rag0"}, {Label: "rag1", Tag: "rag1"}},
		ExpectedEmbeddingModel: "qwen3-embedding:8b",
	}
	out, _ := run(t, b, viewFrom(
		ragEntry("rag0", `{"mapped":{"documents":[["a"]],"distances":[[0.2]],"embedding_model":"qwen3-embedding:8b"}}`),
		ragEntry("rag1", `{"mapped":{"documents":[["b"]],"distances":[[0.1]],"embedding_model":"nomic-embed-text"}}`),
	))
	require.Equal(t, []string{"[rag0] a"}, out.Mapped.Documents)
	require.Equal(t, []string{"rag1"}, out.Mapped.Excluded)
	require.Empty(t, out.Mapped.Degraded)
}
