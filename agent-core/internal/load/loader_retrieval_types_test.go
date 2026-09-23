// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package load

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

// The shipped retrieval types (srd060 R2.1): an application reaches QueryResult
// through /opt/agent-core with no declared root. The install root is
// process-scoped, so this test does not run in parallel.

func TestLoadClosureResolvesTheShippedQueryResult(t *testing.T) {
	corepath.SetInstallRoot(agentCoreRoot(t))
	t.Cleanup(func() { corepath.SetInstallRoot("") })
	root := writeUsednessClosureFixture(t, "selected")
	writeLoadFixture(t, root, "declarations.yaml", `unit: root
imports: [/opt/agent-core/tools/units/types-retrieval.yaml]
tools:
  - name: selected
    binary: echo
    output:
      schema: {$type: builtin-types-retrieval.QueryResult}
`)

	closure, err := LoadClosure(filepath.Join(root, "profile.yaml"), Options{})

	require.NoError(t, err)
	schema := closure.Selected[0].Output.Schema
	require.Equal(t, "object", schema["type"])
	require.ElementsMatch(t, []any{"ids", "documents", "distances", "metadatas", "embedding_model"}, schema["required"],
		"srd060 R1.2: QueryResult carries all five fields")
}
