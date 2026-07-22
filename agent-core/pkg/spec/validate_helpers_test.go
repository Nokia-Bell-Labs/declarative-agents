// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func loadTestGraphAndCorpus(t *testing.T) (*Graph, *Corpus) {
	t.Helper()
	c, err := LoadCorpus(filepath.Join("testdata", "valid"))
	require.NoError(t, err)
	g, err := BuildGraph(c)
	require.NoError(t, err)
	return g, c
}

func completeToolDeclaration(name string) ToolDeclaration {
	return ToolDeclaration{
		Name:     name,
		Category: "word",
		Problem:  "The machine needs a complete word contract for audit validation.",
		Goals:    []string{"Run as a declared machine word."},
		Requirements: ToolDeclRequirements{
			Input:  []string{"must accept declared input"},
			Output: []string{"must return declared output"},
			Errors: []string{"must report declared errors"},
		},
		NonGoals: []string{"Does not choose the next machine state."},
		Emits:    []string{"ToolDone"},
		Output: ToolDeclOutput{Schema: map[string]any{
			"type": "object",
		}},
		SideEffects:   ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "filesystem_read"}}},
		Reversibility: ToolDeclReversibility{Classification: "reversible"},
		Undo:          ToolDeclUndo{Strategy: "noop"},
		Errors:        []ToolDeclError{{Signal: "CommandError"}},
		Relationships: ToolDeclRelationships{After: []string{"next_word"}},
	}
}

func countFindings(checks []string, check string) int {
	count := 0
	for _, candidate := range checks {
		if candidate == check {
			count++
		}
	}
	return count
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
