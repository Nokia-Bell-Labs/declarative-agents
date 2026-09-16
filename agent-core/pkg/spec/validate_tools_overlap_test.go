// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func overlapCorpus(overlap ToolDeclRelationshipRef) *Corpus {
	return &Corpus{ToolDeclarations: map[string]ToolDeclaration{
		"collect_metrics": {
			Name: "collect_metrics", Type: "builtin", Init: "collect_metrics", Category: "word",
			Relationships: ToolDeclRelationships{Overlaps: []ToolDeclRelationshipRef{overlap}},
		},
	}}
}

// TestOverlapMustStateItsDifference: an overlap exists to tell an agent which
// of two similar words to pick, so one naming the neighbor and nothing else
// is a declaration the agent cannot act on. This is also the interpreter's
// read of ToolOverlap.difference (pattern invariant P2.3); the authoring
// reviewer that read it before had no caller (GH-2071).
func TestOverlapMustStateItsDifference(t *testing.T) {
	t.Parallel()
	findings := checkToolDeclarationVocabulary(overlapCorpus(
		ToolDeclRelationshipRef{Tool: "dump_config", Difference: "  "}))

	require.Len(t, findings, 1)
	require.Equal(t, "tool-declaration-invalid", findings[0].Check)
	require.Contains(t, findings[0].Message, `overlaps "dump_config" without stating the difference`)
}

func TestOverlapNamingToolAndDifferencePasses(t *testing.T) {
	t.Parallel()
	findings := checkToolDeclarationVocabulary(overlapCorpus(ToolDeclRelationshipRef{
		Tool: "dump_config", Difference: "collect_metrics samples counters; dump_config prints the resolved profile.",
	}))

	require.Empty(t, findings)
}

func TestOverlapWithNoToolNameIsReportedOnce(t *testing.T) {
	t.Parallel()
	findings := checkToolDeclarationVocabulary(overlapCorpus(
		ToolDeclRelationshipRef{Difference: "states a difference from nothing"}))

	require.Len(t, findings, 1)
	require.Contains(t, findings[0].Message, "overlap with no tool name")
}
