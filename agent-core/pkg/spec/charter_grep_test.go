// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReduceGrepChecksForbiddenTermMatchesWithProvenance(t *testing.T) {
	root := t.TempDir()
	charter := grepCharter("prose-suite", CharterCheck{
		ID:       "no-internal-vocabulary",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"papers/**/*.md"},
		Patterns: []string{"cobbler"},
		Message:  "Publication prose must not leak internal vocabulary.",
	})

	findings, err := reduceGrepFixtures(root, []Charter{charter},
		rgMatchFixture("papers/main.md", 2, "this has cobbler inside\n"))

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

func TestReduceGrepChecksNoMatchPasses(t *testing.T) {
	root := t.TempDir()
	charter := grepCharter("prose-suite", CharterCheck{
		ID:       "no-internal-vocabulary",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"papers/**/*.md"},
		Patterns: []string{"cobbler"},
	})

	findings, err := reduceGrepFixtures(root, []Charter{charter}, "")

	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestBuildGrepChecksPreservesExcludePolicy(t *testing.T) {
	root := t.TempDir()
	charter := grepCharter("prose-suite", CharterCheck{
		ID:       "no-internal-vocabulary",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"papers/**/*.md"},
		Exclude:  []string{"papers/build/**"},
		Patterns: []string{"cobbler"},
	})

	plans, err := BuildGrepSearchPlans(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "papers/**/*.md", plans[0].IncludeGlob)
	assert.Equal(t, "!papers/build/**", plans[0].ExcludeGlob)
}

func TestReduceGrepChecksUsesCharterTargetDefaultsAndSeverity(t *testing.T) {
	root := t.TempDir()
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

	findings, err := reduceGrepFixtures(root, []Charter{charter},
		rgMatchFixture("docs/a.md", 1, "cobbler\n"))

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "warning", findings[0].Level)
	assert.Equal(t, "docs/a.md", findings[0].File)
}

func TestReduceGrepChecksRegexPattern(t *testing.T) {
	root := t.TempDir()
	charter := grepCharter("regex-suite", CharterCheck{
		ID:       "citation-form",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Patterns: []string{`@[A-Za-z]+_[0-9]+`},
		Regex:    true,
	})

	findings, err := reduceGrepFixtures(root, []Charter{charter},
		rgMatchFixture("paper.md", 1, "citation @Known_123\n"))

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "paper.md", findings[0].File)
	assert.Equal(t, 1, findings[0].Line)
}

func TestReduceGrepChecksSortsFindingsDeterministically(t *testing.T) {
	root := t.TempDir()
	charters := []Charter{
		grepCharter("suite-b", CharterCheck{ID: "word", Kind: "grep_check", Severity: "warning", Include: []string{"*.md"}, Patterns: []string{"cobbler"}}),
		grepCharter("suite-a", CharterCheck{ID: "word", Kind: "grep_check", Severity: "warning", Include: []string{"*.md"}, Patterns: []string{"cobbler"}}),
	}

	output := rgMatchFixture("a.md", 1, "cobbler\n") + rgMatchFixture("z.md", 1, "cobbler\n")
	findings, err := reduceGrepFixtures(root, charters, output, output)

	require.NoError(t, err)
	requireDeterministicCharterOrder(t, findings, ".md")
}

func TestReduceGrepChecksMissingModeEmitsFinding(t *testing.T) {
	root := t.TempDir()
	charter := grepCharter("required-suite", CharterCheck{
		ID:       "must-mention-license",
		Kind:     "grep_check",
		Severity: "error",
		Include:  []string{"*.md"},
		Patterns: []string{"license"},
		Mode:     "missing",
	})

	findings, err := reduceGrepFixtures(root, []Charter{charter}, "")

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Empty(t, findings[0].File)
	require.Zero(t, findings[0].Line)
	assert.Contains(t, findings[0].Message, "not found")
}

func reduceGrepFixtures(root string, charters []Charter, outputs ...string) ([]Finding, error) {
	plans, err := BuildGrepSearchPlans(root, charters)
	if err != nil {
		return nil, err
	}
	if len(plans) != len(outputs) {
		return nil, fmt.Errorf("fixture outputs %d do not match plans %d", len(outputs), len(plans))
	}
	var findings []Finding
	for index, plan := range plans {
		exitCode := 0
		if outputs[index] == "" {
			exitCode = 1
		}
		reduced, err := ReduceGrepSearch(plan, outputs[index], exitCode)
		if err != nil {
			return nil, err
		}
		findings = append(findings, reduced...)
	}
	SortFindings(findings)
	return findings, nil
}

func rgMatchFixture(path string, line int, text string) string {
	event := map[string]interface{}{
		"type": "match",
		"data": map[string]interface{}{
			"path":        map[string]string{"text": path},
			"lines":       map[string]string{"text": text},
			"line_number": line,
		},
	}
	data, _ := json.Marshal(event)
	return string(data) + "\n"
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

func requireDeterministicCharterOrder(t *testing.T, findings []Finding, extension string) {
	t.Helper()
	require.Len(t, findings, 4)
	expected := []struct {
		suite string
		file  string
	}{
		{suite: "suite-a", file: "a" + extension},
		{suite: "suite-a", file: "z" + extension},
		{suite: "suite-b", file: "a" + extension},
		{suite: "suite-b", file: "z" + extension},
	}
	for index, want := range expected {
		assert.Equal(t, want.suite, findings[index].SuiteID)
		assert.Equal(t, want.file, findings[index].File)
	}
}

func writeDeterministicCharterFiles(t *testing.T, root, extension, data string) {
	t.Helper()
	writeTargetFile(t, root, "z"+extension, data)
	writeTargetFile(t, root, "a"+extension, data)
}
