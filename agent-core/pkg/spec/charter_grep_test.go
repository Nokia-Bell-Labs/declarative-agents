// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteGrepChecksForbiddenTermMatchesWithProvenance(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "papers/main.md", "public line\nthis has cobbler inside\n")
	charter := grepCharter("prose-suite", CharterCheck{
		ID:       "no-internal-vocabulary",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"papers/**/*.md"},
		Patterns: []string{"cobbler"},
		Message:  "Publication prose must not leak internal vocabulary.",
	})

	findings, err := ExecuteGrepChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "error", findings[0].Level)
	assert.Equal(t, "prose-suite", findings[0].SuiteID)
	assert.Equal(t, "no-internal-vocabulary", findings[0].CheckID)
	assert.Equal(t, "grep_check", findings[0].Kind)
	assert.Equal(t, "papers/main.md", findings[0].File)
	assert.Equal(t, 2, findings[0].Line)
	assert.Equal(t, "Publication prose must not leak internal vocabulary.", findings[0].Message)
}

func TestExecuteGrepChecksNoMatchPasses(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "papers/main.md", "clean publication prose\n")
	charter := grepCharter("prose-suite", CharterCheck{
		ID:       "no-internal-vocabulary",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"papers/**/*.md"},
		Patterns: []string{"cobbler"},
	})

	findings, err := ExecuteGrepChecks(root, []Charter{charter})

	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExecuteGrepChecksExcludesFiles(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "papers/main.md", "clean\n")
	writeTargetFile(t, root, "papers/build/draft.md", "cobbler\n")
	charter := grepCharter("prose-suite", CharterCheck{
		ID:       "no-internal-vocabulary",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"papers/**/*.md"},
		Exclude:  []string{"papers/build/**"},
		Patterns: []string{"cobbler"},
	})

	findings, err := ExecuteGrepChecks(root, []Charter{charter})

	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExecuteGrepChecksUsesCharterTargetDefaultsAndSeverity(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "docs/a.md", "cobbler\n")
	writeTargetFile(t, root, "docs/private/b.md", "cobbler\n")
	charter := Charter{
		ID: "docs-suite",
		Target: CharterTarget{
			Root:    "docs",
			Include: []string{"**/*.md"},
			Exclude: []string{"private/**"},
		},
		Checks: []CharterCheck{{
			ID:       "warn-word",
			Kind:     "grep_check",
			Severity: "warning",
			Patterns: []string{"cobbler"},
		}},
	}

	findings, err := ExecuteGrepChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "warning", findings[0].Level)
	assert.Equal(t, "docs/a.md", findings[0].File)
}

func TestExecuteGrepChecksRegexPattern(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "citation @Known_123\n")
	charter := grepCharter("regex-suite", CharterCheck{
		ID:       "citation-form",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Patterns: []string{`@[A-Za-z]+_[0-9]+`},
		Regex:    true,
	})

	findings, err := ExecuteGrepChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "paper.md", findings[0].File)
	assert.Equal(t, 1, findings[0].Line)
}

func TestExecuteGrepChecksSortsFindingsDeterministically(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "z.md", "cobbler\n")
	writeTargetFile(t, root, "a.md", "cobbler\n")
	charters := []Charter{
		grepCharter("suite-b", CharterCheck{ID: "word", Kind: "grep_check", Severity: "warning", Include: []string{"*.md"}, Patterns: []string{"cobbler"}}),
		grepCharter("suite-a", CharterCheck{ID: "word", Kind: "grep_check", Severity: "warning", Include: []string{"*.md"}, Patterns: []string{"cobbler"}}),
	}

	findings, err := ExecuteGrepChecks(root, charters)

	require.NoError(t, err)
	require.Len(t, findings, 4)
	assert.Equal(t, "suite-a", findings[0].SuiteID)
	assert.Equal(t, "a.md", findings[0].File)
	assert.Equal(t, "suite-a", findings[1].SuiteID)
	assert.Equal(t, "z.md", findings[1].File)
	assert.Equal(t, "suite-b", findings[2].SuiteID)
	assert.Equal(t, "a.md", findings[2].File)
	assert.Equal(t, "suite-b", findings[3].SuiteID)
	assert.Equal(t, "z.md", findings[3].File)
}

func TestExecuteGrepChecksMissingModeEmitsFinding(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "paper.md", "text without required phrase\n")
	charter := grepCharter("required-suite", CharterCheck{
		ID:       "must-mention-license",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Patterns: []string{"license"},
		Mode:     "missing",
	})

	findings, err := ExecuteGrepChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Empty(t, findings[0].File)
	require.Zero(t, findings[0].Line)
	assert.Contains(t, findings[0].Message, "not found")
}

func grepCharter(id string, check CharterCheck) Charter {
	return Charter{ID: id, Checks: []CharterCheck{check}}
}

func writeTargetFile(t *testing.T, root, rel, data string) {
	t.Helper()
	path := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
}
