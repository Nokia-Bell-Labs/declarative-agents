// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
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

	findings, err := ExecuteConsistencyChecks(root, []Charter{charter})

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

	findings, err := ExecuteConsistencyChecks(root, []Charter{charter})

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

	findings, err := ExecuteConsistencyChecks(root, []Charter{charter})

	require.NoError(t, err)
	require.Len(t, findings, 1)
	assert.Equal(t, "manifest.yaml", findings[0].File)
	assert.Equal(t, 4, findings[0].Line)
	assert.Contains(t, findings[0].Message, "results/missing.json")
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

	findings, err := ExecuteConsistencyChecks(root, []Charter{charter})

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

	findings, err := ExecuteConsistencyChecks(root, []Charter{charter})

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

	findings, err := ExecuteConsistencyChecks(root, charters)

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

	findings, err := ExecuteConsistencyChecks(root, []Charter{charter})

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

	findings, err := ExecuteConsistencyChecks(root, []Charter{charter})

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

	findings, err := ExecuteConsistencyChecks(root, []Charter{charter})

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

	_, err := ExecuteConsistencyChecks(root, []Charter{charter})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "field=value")
}

func consistencyCharter(id string, check CharterCheck) Charter {
	return Charter{ID: id, Checks: []CharterCheck{check}}
}
