// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package declstyle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// srd051 R6.14 and R6.15: an unsigned boundary or stateful_internal tool is
// untyped, a tool with no category is uncategorized, and a signed boundary tool
// carrying only its signals is neither (GH-2110).
func TestUntypedAndUncategorizedToolClassification(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "agents", "demo", "declarations.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(`tools:
  - {name: unsigned_boundary, category: boundary}
  - {name: unsigned_stateful, category: stateful_internal}
  - {name: signed_boundary, category: boundary, signature: {emits: [ToolDone]}}
  - {name: no_category}
`), 0o644))

	var got []string
	for _, entry := range fileEntries(t, path) {
		got = append(got, entry[:strings.Index(entry, ":")]+":"+entry[strings.LastIndex(entry, ":")+1:])
	}
	require.ElementsMatch(t, []string{
		classUntypedTool + ":unsigned_boundary",
		classUntypedTool + ":unsigned_stateful",
		classUncategorizedTool + ":no_category",
	}, got)
}
