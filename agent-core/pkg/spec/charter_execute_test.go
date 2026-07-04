// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecuteChartersWithoutChartersPreservesSpecCorpus(t *testing.T) {
	graph, corpus := loadTestGraphAndCorpus(t)

	got, err := ExecuteCharters(filepath.Join("testdata", "valid"), graph, corpus, nil)
	want := Validate(graph, corpus)

	require.NoError(t, err)
	require.Equal(t, findingTriples(want), findingTriples(got))
}

func TestExecuteChartersSpecCorpusAddsProvenance(t *testing.T) {
	graph, corpus := loadTestGraphAndCorpus(t)
	charters := []Charter{{
		ID: "builtin-spec-corpus",
		Checks: []CharterCheck{{
			ID:       "spec-corpus",
			Kind:     "spec_corpus",
			Severity: "error",
		}},
	}}

	findings, err := ExecuteCharters(filepath.Join("testdata", "valid"), graph, corpus, charters)

	require.NoError(t, err)
	require.NotEmpty(t, findings)
	require.Equal(t, "builtin-spec-corpus", findings[0].SuiteID)
	require.Equal(t, findings[0].Check, findings[0].CheckID)
	require.Equal(t, "spec_corpus", findings[0].Kind)
}

func TestExecuteChartersSpecCorpusSubset(t *testing.T) {
	graph, corpus := loadTestGraphAndCorpus(t)
	charters := []Charter{{
		ID: "builtin-spec-corpus",
		Checks: []CharterCheck{{
			ID:     "spec-corpus",
			Kind:   "spec_corpus",
			Checks: []string{"orphaned-srd"},
		}},
	}}

	findings, err := ExecuteCharters(filepath.Join("testdata", "valid"), graph, corpus, charters)

	require.NoError(t, err)
	require.NotEmpty(t, findings)
	for _, finding := range findings {
		require.Equal(t, "orphaned-srd", finding.Check)
	}
}

func TestExecuteChartersAggregatesDeterministicallyAcrossSuites(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "z.md", "cobbler\n")
	writeTargetFile(t, root, "a.md", "cobbler\n")
	graph, corpus := loadTestGraphAndCorpus(t)
	charters := []Charter{
		{
			ID: "suite-b",
			Checks: []CharterCheck{{
				ID:       "word",
				Kind:     "grep_check",
				Severity: "warning",
				Include:  []string{"*.md"},
				Patterns: []string{"cobbler"},
			}},
		},
		{
			ID: "suite-a",
			Checks: []CharterCheck{{
				ID:       "word",
				Kind:     "grep_check",
				Severity: "warning",
				Include:  []string{"*.md"},
				Patterns: []string{"cobbler"},
			}},
		},
	}

	findings, err := ExecuteCharters(root, graph, corpus, charters)

	require.NoError(t, err)
	require.Len(t, findings, 4)
	require.Equal(t, "suite-a", findings[0].SuiteID)
	require.Equal(t, "a.md", findings[0].File)
	require.Equal(t, "suite-a", findings[1].SuiteID)
	require.Equal(t, "z.md", findings[1].File)
	require.Equal(t, "suite-b", findings[2].SuiteID)
	require.Equal(t, "a.md", findings[2].File)
	require.Equal(t, "suite-b", findings[3].SuiteID)
	require.Equal(t, "z.md", findings[3].File)
}

func findingTriples(findings []Finding) []Finding {
	triples := make([]Finding, len(findings))
	for i, finding := range findings {
		triples[i] = Finding{
			Check:   finding.Check,
			Level:   finding.Level,
			Message: finding.Message,
		}
	}
	sortFindings(triples)
	return triples
}
