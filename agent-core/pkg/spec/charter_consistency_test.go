// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteConsistencyChecksEqualsPassesForMatchingField(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.yaml", "status: done\n")
	charter := consistencyCharter("manifest-suite", CharterCheck{
		ID:       "status-done",
		Kind:     "consistency_check",
		Severity: "error",
		Include:  []string{"manifest.yaml"},
		Source:   map[string]any{"yaml_path": "$.status"},
		Rule:     "equals",
		Target:   map[string]any{"value": "done"},
	})

	findings, err := executeConsistencyFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExecuteConsistencyChecksEqualsReportsMismatchWithProvenance(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.yaml", "status: draft\n")
	charter := consistencyCharter("manifest-suite", CharterCheck{
		ID:       "status-done",
		Kind:     "consistency_check",
		Severity: "warning",
		Include:  []string{"manifest.yaml"},
		Source:   map[string]any{"yaml_path": "$.status"},
		Rule:     "equals",
		Target:   map[string]any{"value": "done"},
	})

	findings, err := executeConsistencyFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "warning", findings[0].Level)
	assert.Equal(t, "manifest-suite", findings[0].SuiteID)
	assert.Equal(t, "status-done", findings[0].CheckID)
	assert.Equal(t, "consistency_check", findings[0].Kind)
	assert.Equal(t, "manifest.yaml", findings[0].File)
	assert.Equal(t, 1, findings[0].Line)
	assert.Contains(t, findings[0].Message, "draft")
}

func TestExecuteConsistencyChecksRequiredPathExists(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.yaml", `
experiments:
  - artifact: results/ok.json
  - artifact: results/missing.json
`)
	writeTargetFile(t, root, "artifacts/results/ok.json", "{}\n")
	charter := consistencyCharter("artifact-suite", CharterCheck{
		ID:       "artifacts-exist",
		Kind:     "consistency_check",
		Severity: "error",
		Include:  []string{"manifest.yaml"},
		Source:   map[string]any{"yaml_path": "$.experiments[*].artifact"},
		Rule:     "required_path_exists",
		Target:   map[string]any{"root": "artifacts"},
	})

	findings, err := executeConsistencyFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "manifest.yaml", findings[0].File)
	assert.Equal(t, 4, findings[0].Line)
	assert.Contains(t, findings[0].Message, "results/missing.json")
}

func TestBuildConsistencyScanLowersGlobsAndExactSource(t *testing.T) {
	t.Parallel()
	charter := Charter{
		ID: "docs",
		Target: CharterTarget{
			Root: "docs", Include: []string{"**/*.yaml"}, Exclude: []string{"generated/**"},
		},
		Checks: []CharterCheck{{
			ID: "one", Kind: "consistency_check",
			Source: map[string]any{"file": "manifest.yaml", "yaml_path": "$.status"},
			Rule:   "equals", Target: map[string]any{"value": "done"},
		}},
	}

	plans, err := BuildConsistencyScanPlans(".", []Charter{charter})

	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "docs/**/*.yaml", plans[0].IncludeGlob)
	assert.Equal(t, "!docs/generated/**", plans[0].ExcludeGlob)
	assert.Equal(t, "docs/manifest.yaml", plans[0].SourceGlob)
}

func TestConsistencyScanPlanSerializesOptionalCommandStateFields(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(ConsistencyScanPlan{})
	require.NoError(t, err)
	var fields map[string]any
	require.NoError(t, json.Unmarshal(data, &fields))
	for _, field := range []string{"include_glob", "exclude_glob", "source_glob"} {
		require.Contains(t, fields, field)
		require.Equal(t, "", fields[field])
	}
}

func TestBuildConsistencyScanConfinesAbsoluteSourceToRoot(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	check := CharterCheck{
		ID: "one", Kind: "consistency_check",
		Source: map[string]any{
			"file": filepath.Join(root, "manifest.yaml"), "yaml_path": "$.status",
		},
		Rule: "equals", Target: map[string]any{"value": "done"},
	}

	plans, err := BuildConsistencyScanPlans(".", []Charter{{
		ID: "docs", Target: CharterTarget{Root: root}, Checks: []CharterCheck{check},
	}})
	require.NoError(t, err)
	require.Len(t, plans, 1)
	assert.Equal(t, "manifest.yaml", plans[0].SourceGlob)

	check.Source["file"] = filepath.Join(filepath.Dir(root), "outside.yaml")
	_, err = BuildConsistencyScanPlans(".", []Charter{{
		ID: "docs", Target: CharterTarget{Root: root}, Checks: []CharterCheck{check},
	}})
	require.ErrorContains(t, err, "outside target root")
}

func TestExecuteConsistencyChecksRequiredWhenTargetFieldMissing(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.yaml", "publish: true\n")
	charter := consistencyCharter("publish-suite", CharterCheck{
		ID:       "publish-has-artifact",
		Kind:     "consistency_check",
		Severity: "error",
		Include:  []string{"manifest.yaml"},
		Source:   map[string]any{"yaml_path": "$.publish"},
		Rule:     "required_when",
		Target:   map[string]any{"yaml_path": "$.artifact"},
	})

	findings, err := executeConsistencyFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "manifest.yaml", findings[0].File)
	assert.Equal(t, 1, findings[0].Line)
	assert.Contains(t, findings[0].Message, "$.artifact")
}

func TestExecuteConsistencyChecksRequiredWhenFalsePasses(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.yaml", "publish: false\n")
	charter := consistencyCharter("publish-suite", CharterCheck{
		ID:       "publish-has-artifact",
		Kind:     "consistency_check",
		Severity: "error",
		Include:  []string{"manifest.yaml"},
		Source:   map[string]any{"yaml_path": "$.publish"},
		Rule:     "required_when",
		Target:   map[string]any{"yaml_path": "$.artifact"},
	})

	findings, err := executeConsistencyFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExecuteConsistencyChecksSortsFindingsDeterministically(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "z.yaml", "artifact: z.json\n")
	writeTargetFile(t, root, "a.yaml", "artifact: a.json\n")
	charters := []Charter{
		consistencyCharter("suite-b", CharterCheck{ID: "artifacts", Kind: "consistency_check", Severity: "warning", Include: []string{"*.yaml"}, Source: map[string]any{"yaml_path": "$.artifact"}, Rule: "required_path_exists"}),
		consistencyCharter("suite-a", CharterCheck{ID: "artifacts", Kind: "consistency_check", Severity: "warning", Include: []string{"*.yaml"}, Source: map[string]any{"yaml_path": "$.artifact"}, Rule: "required_path_exists"}),
	}

	findings, err := executeConsistencyFixtures(t, root, charters)

	require.NoError(t, err)
	requireDeterministicCharterOrder(t, findings, ".yaml")
}

func TestExecuteConsistencyChecksFilterSelectsOnlyMatchingEntries(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.yaml", `
experiments:
  - status: done
    apparatus:
      artifacts:
        - results/done-present.json
        - results/done-missing.json
  - status: planned
    apparatus:
      artifacts:
        - results/planned-missing.json
  - status: deferred
    apparatus:
      artifacts:
        - results/deferred-missing.json
`)
	writeTargetFile(t, root, "artifacts/results/done-present.json", "{}\n")
	charter := consistencyCharter("evidence-suite", CharterCheck{
		ID:       "done-artifacts-exist",
		Kind:     "consistency_check",
		Severity: "error",
		Include:  []string{"manifest.yaml"},
		Source:   map[string]any{"yaml_path": "$.experiments[?status=done].apparatus.artifacts[*]"},
		Rule:     "required_path_exists",
		Target:   map[string]any{"root": "artifacts"},
	})

	findings, err := executeConsistencyFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "results/done-missing.json")
}

func TestExecuteConsistencyChecksFilterNegationExcludesMatchingEntries(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.yaml", `
experiments:
  - status: done
    artifact: results/done.json
  - status: planned
    artifact: results/planned.json
`)
	writeTargetFile(t, root, "artifacts/results/planned.json", "{}\n")
	charter := consistencyCharter("evidence-suite", CharterCheck{
		ID:       "non-done-artifacts-exist",
		Kind:     "consistency_check",
		Severity: "error",
		Include:  []string{"manifest.yaml"},
		Source:   map[string]any{"yaml_path": "$.experiments[?status!=done].artifact"},
		Rule:     "required_path_exists",
		Target:   map[string]any{"root": "artifacts"},
	})

	findings, err := executeConsistencyFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExecuteConsistencyChecksFilterEqualsEvaluatesMatchingEntriesOnly(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.yaml", `
experiments:
  - status: done
    kind: benchmark
  - status: planned
    kind: draft
`)
	charter := consistencyCharter("evidence-suite", CharterCheck{
		ID:       "done-are-benchmark",
		Kind:     "consistency_check",
		Severity: "error",
		Include:  []string{"manifest.yaml"},
		Source:   map[string]any{"yaml_path": "$.experiments[?status=done].kind"},
		Rule:     "equals",
		Target:   map[string]any{"value": "benchmark"},
	})

	findings, err := executeConsistencyFixtures(t, root, []Charter{charter})

	require.NoError(t, err)
	assert.Empty(t, findings)
}

func TestExecuteConsistencyChecksMalformedFilterReturnsError(t *testing.T) {
	root := t.TempDir()
	writeTargetFile(t, root, "manifest.yaml", "experiments: []\n")
	charter := consistencyCharter("evidence-suite", CharterCheck{
		ID:       "bad-filter",
		Kind:     "consistency_check",
		Severity: "error",
		Include:  []string{"manifest.yaml"},
		Source:   map[string]any{"yaml_path": "$.experiments[?status].artifact"},
		Rule:     "required_path_exists",
	})

	_, err := executeConsistencyFixtures(t, root, []Charter{charter})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "field=value")
}

func TestReduceConsistencyScanRejectsMalformedTaggedRecords(t *testing.T) {
	t.Parallel()
	plan := ConsistencyScanPlan{
		SuiteID: "suite",
		Check: CharterCheck{
			ID: "check", Kind: "consistency_check",
			Source: map[string]any{"yaml_path": "$.status"},
			Rule:   "equals", Target: map[string]any{"value": "done"},
		},
	}
	path := base64.StdEncoding.EncodeToString([]byte("manifest.yaml"))
	tests := []struct {
		name, output, want string
	}{
		{"unknown tag", "X\t" + path, "unknown consistency scan record"},
		{"truncated file", "F\t" + path, "invalid consistency file record"},
		{"bad path", "I\t%%%", "decode scanned path"},
		{"bad content", "F\t" + path + "\t%%%", "decode scanned file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ReduceConsistencyScan(plan, tt.output)
			require.ErrorContains(t, err, tt.want)
		})
	}
}

func executeConsistencyFixtures(t *testing.T, targetDir string, charters []Charter) ([]Finding, error) {
	t.Helper()
	plans, err := BuildConsistencyScanPlans(targetDir, charters)
	if err != nil {
		return nil, err
	}
	var findings []Finding
	for _, plan := range plans {
		root := plan.Path
		if !filepath.IsAbs(root) {
			root = filepath.Join(targetDir, root)
		}
		var scan strings.Builder
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			writeConsistencyRecord(&scan, "I", filepath.ToSlash(rel), nil)
			return nil
		})
		if err != nil {
			return nil, err
		}
		selected, err := testConsistencySelectedPaths(targetDir, root, plan)
		if err != nil {
			return nil, err
		}
		for _, rel := range selected {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			if err != nil {
				return nil, err
			}
			writeConsistencyRecord(&scan, "F", rel, data)
		}
		reduced, err := ReduceConsistencyScan(plan, scan.String())
		if err != nil {
			return nil, err
		}
		findings = append(findings, reduced...)
	}
	SortFindings(findings)
	return findings, nil
}

func testConsistencySelectedPaths(targetDir, root string, plan ConsistencyScanPlan) ([]string, error) {
	args := []string{"--files", "--hidden", "--no-ignore", "--sort", "path"}
	if plan.SourceGlob != "" {
		args = append(args, "--glob", plan.SourceGlob)
	} else {
		if plan.IncludeGlob != "" {
			args = append(args, "--glob", plan.IncludeGlob)
		}
		if plan.ExcludeGlob != "" {
			args = append(args, "--glob", plan.ExcludeGlob)
		}
	}
	cmd := exec.Command("rg", append(args, plan.Path)...)
	cmd.Dir = targetDir
	out, err := cmd.Output()
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var selected []string
	for _, path := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !filepath.IsAbs(path) {
			path = filepath.Join(targetDir, path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		selected = append(selected, filepath.ToSlash(rel))
	}
	return selected, nil
}

func writeConsistencyRecord(out *strings.Builder, kind, rel string, data []byte) {
	out.WriteString(kind)
	out.WriteByte('\t')
	out.WriteString(base64.StdEncoding.EncodeToString([]byte(rel)))
	if kind == "F" {
		out.WriteByte('\t')
		out.WriteString(base64.StdEncoding.EncodeToString(data))
	}
	out.WriteByte('\n')
}

func consistencyCharter(id string, check CharterCheck) Charter {
	return Charter{ID: id, Checks: []CharterCheck{check}}
}
