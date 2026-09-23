// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The shipped retrieval query client (srd060 R3). Applications that serve
// retrieval differ only in the route prefix and where the base URL and vector
// come from, so the client is one expansion. installedAgentCore sets a
// process-scoped root, so these tests do not run in parallel.

const retrievalQueryClientFragment = "/opt/agent-core/tools/rest/units/retrieval-query-client-fragment.yaml"

func retrievalExpansion(t *testing.T, args string) string {
	t.Helper()
	return writeImportFixture(t, t.TempDir(), "rest.yaml", `unit: probe-retrieval-rest
expand:
- fragment: `+retrievalQueryClientFragment+`
  args: `+args+`
rest:
  version: v1
  limits:
    probe_retrieval: {max_request_bytes: 4096}
`)
}

func TestRetrievalQueryClientFragmentFollowsTheRoutePrefix(t *testing.T) {
	installedAgentCore(t)
	for prefix, args := range map[string]string{
		"rag":       `{route_prefix: rag, base_url_selector: "$from(rag_unit).base_url", input_selector: "$from(normalize_query_embedding).mapped.embedding", limits_ref: probe_retrieval}`,
		"knowledge": `{route_prefix: knowledge, base_url_selector: "$from(knowledge_unit).base_url", input_selector: "$from(normalize_query_embedding).mapped.embedding", n_results: 8, limits_ref: probe_retrieval}`,
	} {
		def, err := LoadDefinitionClosure([]string{retrievalExpansion(t, args)}, nil)

		require.NoError(t, err, prefix)
		client, ok := def.Clients["retrieval"]
		require.True(t, ok, "the fragment produces a client named retrieval")
		require.Equal(t, "probe_retrieval", client.LimitsRef)
		op := client.Operations["query"]
		require.Equal(t, "POST", op.Method)
		require.Equal(t, "/api/v1/"+prefix+"/query", op.Path)
		require.Equal(t, "command_state", op.BaseURLSource)
		require.Equal(t, "$from("+prefix+"_unit).base_url", op.BaseURLSelector)
	}
}

func TestRetrievalQueryClientFragmentReturnsTheWholeQueryResult(t *testing.T) {
	installedAgentCore(t)
	def, err := LoadDefinitionClosure([]string{retrievalExpansion(t,
		`{route_prefix: rag, base_url_selector: "$from(rag_unit).base_url", input_selector: "$.embedding", limits_ref: probe_retrieval}`)}, nil)

	require.NoError(t, err)
	op := def.Clients["retrieval"].Operations["query"]
	require.Equal(t, map[string]string{
		"ids":             "$.ids",
		"documents":       "$.documents",
		"distances":       "$.distances",
		"metadatas":       "$.metadatas",
		"embedding_model": "$.embedding_model",
	}, op.Response.Output, "srd060 R1.2: every QueryResult field reaches the caller")
	require.EqualValues(t, 5, op.Body["n_results"], "n_results defaults to five")
	require.Equal(t, "QueryResponded", op.Success.Signal)
	require.Len(t, op.Failures, 1)
	require.Equal(t, []int{400}, op.Failures[0].Status)
	require.Equal(t, "QueryRejected", op.Failures[0].Signal)
	require.Len(t, op.SideEffects, 1)
	require.Equal(t, "external_api", op.SideEffects[0].Kind)
	require.Equal(t, "read_only", op.SideEffects[0].State)
}

func TestRetrievalQueryClientFragmentRequiresItsRoutePrefix(t *testing.T) {
	installedAgentCore(t)
	_, err := LoadDefinitionClosure([]string{retrievalExpansion(t,
		`{base_url_selector: "$from(rag_unit).base_url", input_selector: "$.embedding", limits_ref: probe_retrieval}`)}, nil)

	require.Error(t, err)
	require.Contains(t, err.Error(), "route_prefix", "the error names the parameter that was not supplied")
}
