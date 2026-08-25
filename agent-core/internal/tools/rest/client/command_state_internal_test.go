// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func objectSchema(required []string, properties map[string]string) map[string]interface{} {
	props := map[string]interface{}{}
	for name, kind := range properties {
		props[name] = map[string]interface{}{"type": kind}
	}
	req := make([]interface{}, 0, len(required))
	for _, name := range required {
		req = append(req, name)
	}
	return map[string]interface{}{"type": "object", "required": req, "properties": props}
}

func commandStateDigest(output string) core.ResultDigest {
	return core.ResultDigest{
		Output:           output,
		RedactionVersion: core.OutputRedactionVersion1,
		RedactionStatus:  core.OutputRedactionApplied,
	}
}

func TestRESTClient_CommandStateDuplicateLabelMostRecentWins(t *testing.T) {
	t.Parallel()
	view := core.NewCommandStateView(core.Execution{
		{CommandName: "embed_query", Result: commandStateDigest(`{"embedding":[1]}`)},
		{CommandName: "embed_query", Result: commandStateDigest(`{"embedding":[2]}`)},
	})
	binding := RequestBinding{
		BodySchema:   objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
		BodySource:   bodySourceCommandState,
		InputMapping: map[string]string{"query_embeddings": "$from(embed_query).embedding"},
	}
	selected, err := selectCommandStateParams(view, binding)
	require.NoError(t, err)
	require.Equal(t, []interface{}{float64(2)}, selected["query_embeddings"])
}

func TestRESTClient_CommandStateMissNamesLabel(t *testing.T) {
	t.Parallel()
	view := core.NewCommandStateView(core.Execution{
		{CommandName: "other_step", Result: commandStateDigest(`{"x":1}`)},
	})
	binding := RequestBinding{
		BodySchema:   objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
		BodySource:   bodySourceCommandState,
		InputMapping: map[string]string{"query_embeddings": "$from(embed_query).embedding"},
	}
	_, err := selectCommandStateParams(view, binding)
	require.ErrorContains(t, err, `no prior step labeled "embed_query"`)
}

func TestRESTClient_CommandStateNoViewConfiguredRejected(t *testing.T) {
	t.Parallel()
	binding := RequestBinding{
		BodySchema:   objectSchema([]string{"query_embeddings"}, map[string]string{"query_embeddings": "array"}),
		BodySource:   bodySourceCommandState,
		InputMapping: map[string]string{"query_embeddings": "$from(embed_query).embedding"},
	}
	_, err := selectCommandStateParams(nil, binding)
	require.ErrorContains(t, err, "not supported until a shared command-state store exists")
}
