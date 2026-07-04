// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecuteRefChecksResolvedInlineReferencesPass(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "See @known and @also-known.\n")
	charter := refCharter("citation-suite", CharterCheck{
		ID:       "citations-resolve",
		Kind:     "ref_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Refs:     map[string]any{"values": []any{"known", "also-known"}},
		Extract:  map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
	})

	findings, err := ExecuteRefChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Empty(t, findings)
}

func TestExecuteRefChecksReportsMissingReferenceWithProvenance(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "See @known.\nBut @missing is absent.\n")
	charter := refCharter("citation-suite", CharterCheck{
		ID:       "citations-resolve",
		Kind:     "ref_check",
		Severity: "warning",
		Include:  []string{"*.md"},
		Refs:     map[string]any{"values": []any{"known"}},
		Extract:  map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
	})

	findings, err := ExecuteRefChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	require.Equal(t, "warning", findings[0].Level)
	require.Equal(t, "citation-suite", findings[0].SuiteID)
	require.Equal(t, "citations-resolve", findings[0].CheckID)
	require.Equal(t, "ref_check", findings[0].Kind)
	require.Equal(t, "paper.md", findings[0].File)
	require.Equal(t, 2, findings[0].Line)
	require.Contains(t, findings[0].Message, "missing")
}

func TestExecuteRefChecksResolvesPathReferencesAgainstDirectory(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.txt", "artifact=results/out.json\nartifact=results/missing.json\n")
	writeTargetFile(t, root, "artifacts/results/out.json", "{}\n")
	charter := refCharter("artifact-suite", CharterCheck{
		ID:       "artifacts-exist",
		Kind:     "ref_check",
		Severity: "error",
		Include:  []string{"manifest.txt"},
		Refs:     map[string]any{"directory": "artifacts"},
		Extract:  map[string]any{"regex": `artifact=([^\s]+)`},
	})

	findings, err := ExecuteRefChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	require.Equal(t, "manifest.txt", findings[0].File)
	require.Equal(t, 2, findings[0].Line)
	require.Contains(t, findings[0].Message, "results/missing.json")
}

func TestExecuteRefChecksLoadsBibtexKeys(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "Cite @Known2026 and @Missing2026.\n")
	writeTargetFile(t, root, "references.bib", "@article{Known2026,\n  title = {Known}\n}\n")
	charter := refCharter("bib-suite", CharterCheck{
		ID:       "bib-citations",
		Kind:     "ref_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Refs:     map[string]any{"file": "references.bib", "format": "bibtex_keys"},
		Extract:  map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
	})

	findings, err := ExecuteRefChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	require.Contains(t, findings[0].Message, "Missing2026")
}

func TestExecuteRefChecksAllowMissingSuppressesNoReferenceFinding(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "No citations here.\n")
	charter := refCharter("citation-suite", CharterCheck{
		ID:           "citations-resolve",
		Kind:         "ref_check",
		Severity:     "error",
		Include:      []string{"*.md"},
		Refs:         map[string]any{"values": []any{"known"}},
		Extract:      map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
		AllowMissing: true,
	})

	findings, err := ExecuteRefChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Empty(t, findings)
}

func TestExecuteRefChecksSortsFindingsDeterministically(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "z.md", "@missing-z\n")
	writeTargetFile(t, root, "a.md", "@missing-a\n")
	charters := []Charter{
		refCharter("suite-b", CharterCheck{ID: "refs", Kind: "ref_check", Severity: "warning", Include: []string{"*.md"}, Refs: map[string]any{"values": []any{"known"}}, Extract: map[string]any{"regex": `@([A-Za-z0-9:_-]+)`}}),
		refCharter("suite-a", CharterCheck{ID: "refs", Kind: "ref_check", Severity: "warning", Include: []string{"*.md"}, Refs: map[string]any{"values": []any{"known"}}, Extract: map[string]any{"regex": `@([A-Za-z0-9:_-]+)`}}),
	}

	findings, err := ExecuteRefChecks(root, charters)

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

func TestExecuteRefChecksNoReferencesFoundIsFindingByDefault(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "No citations here.\n")
	charter := refCharter("citation-suite", CharterCheck{
		ID:       "citations-resolve",
		Kind:     "ref_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Refs:     map[string]any{"values": []any{"known"}},
		Extract:  map[string]any{"regex": `@([A-Za-z0-9:_-]+)`},
	})

	findings, err := ExecuteRefChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	require.Empty(t, findings[0].File)
	require.Contains(t, findings[0].Message, "no references found")
}

func refCharter(id string, check CharterCheck) Charter {
	return Charter{ID: id, Checks: []CharterCheck{check}}
}
