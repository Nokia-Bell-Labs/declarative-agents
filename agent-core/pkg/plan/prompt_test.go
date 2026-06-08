// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package plan

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testTask = TaskContext{
	ID:    "srd004-R1.1..R1.3",
	SRDID: "srd004",
	Items: []TaskItem{
		{ID: "R1.1", Text: "Define Config struct with typed fields"},
		{ID: "R1.2", Text: "Parse YAML files into Config"},
	},
}

var testSRD = SRDContext{
	Problem: "Configuration files are parsed ad-hoc with no validation.",
	Goals: []string{
		"G1: Provide typed configuration loading",
		"G2: Validate all fields at load time",
	},
	AcceptanceCriteria: []string{
		"AC1: Config loads from a valid YAML file",
		"AC2: Error returned for missing required fields",
	},
}

// TestRel00_1_UC001_PromptContainsSRDContext verifies that the assembled
// prompt includes the SRD problem, goals, and requirement items (srd008 AC1).
func TestRel00_1_UC001_PromptContainsSRDContext(t *testing.T) {
	t.Parallel()
	prompt, err := AssemblePrompt(testTask, testSRD, nil, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, prompt)

	assert.Contains(t, prompt, testSRD.Problem)
	for _, g := range testSRD.Goals {
		assert.Contains(t, prompt, g)
	}
	for _, ac := range testSRD.AcceptanceCriteria {
		assert.Contains(t, prompt, ac)
	}
	for _, item := range testTask.Items {
		assert.Contains(t, prompt, item.ID)
		assert.Contains(t, prompt, item.Text)
	}
	assert.Contains(t, prompt, testTask.ID)
	assert.Contains(t, prompt, testTask.SRDID)
}

// TestRel00_1_UC001_PromptIncludesDependencySection verifies that
// dependency context toggles a dependency section (srd008 AC2).
func TestRel00_1_UC001_PromptIncludesDependencySection(t *testing.T) {
	t.Parallel()

	deps := []DepItem{
		{ID: "R0.1", Files: []string{"internal/loader/loader.go"}},
		{ID: "R0.2", Files: []string{"internal/loader/validate.go", "internal/loader/types.go"}},
	}

	t.Run("with deps", func(t *testing.T) {
		t.Parallel()
		prompt, err := AssemblePrompt(testTask, testSRD, deps, nil)
		require.NoError(t, err)
		assert.Contains(t, prompt, "Already Implemented")
		assert.Contains(t, prompt, "R0.1")
		assert.Contains(t, prompt, "loader.go")
		assert.Contains(t, prompt, "R0.2")
		assert.Contains(t, prompt, "validate.go")
	})

	t.Run("without deps", func(t *testing.T) {
		t.Parallel()
		prompt, err := AssemblePrompt(testTask, testSRD, nil, nil)
		require.NoError(t, err)
		assert.NotContains(t, prompt, "Already Implemented")
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		prompt, err := AssemblePrompt(testTask, testSRD, []DepItem{}, nil)
		require.NoError(t, err)
		assert.NotContains(t, prompt, "Already Implemented")
	})
}

// TestRel00_1_UC001_PromptIncludesRetrySection verifies that failure
// context toggles a retry section (srd008 AC3).
func TestRel00_1_UC001_PromptIncludesRetrySection(t *testing.T) {
	t.Parallel()

	failures := []string{
		`build error: undefined: ConfigParser`,
		`test failed: TestLoad expected 3 fields, got 0`,
	}

	t.Run("with failures", func(t *testing.T) {
		t.Parallel()
		prompt, err := AssemblePrompt(testTask, testSRD, nil, failures)
		require.NoError(t, err)
		assert.Contains(t, prompt, "Retry Context")
		assert.Contains(t, prompt, "ConfigParser")
		assert.Contains(t, prompt, "TestLoad")
	})

	t.Run("without failures", func(t *testing.T) {
		t.Parallel()
		prompt, err := AssemblePrompt(testTask, testSRD, nil, nil)
		require.NoError(t, err)
		assert.NotContains(t, prompt, "Retry Context")
	})

	t.Run("empty slice", func(t *testing.T) {
		t.Parallel()
		prompt, err := AssemblePrompt(testTask, testSRD, nil, []string{})
		require.NoError(t, err)
		assert.NotContains(t, prompt, "Retry Context")
	})
}

// TestRel00_1_UC001_PromptDeterministic verifies the same inputs produce
// identical output across calls (srd008 AC4).
func TestRel00_1_UC001_PromptDeterministic(t *testing.T) {
	t.Parallel()
	deps := []DepItem{{ID: "R0.1", Files: []string{"a.go"}}}
	failures := []string{"error: missing field"}

	first, err := AssemblePrompt(testTask, testSRD, deps, failures)
	require.NoError(t, err)

	second, err := AssemblePrompt(testTask, testSRD, deps, failures)
	require.NoError(t, err)

	assert.Equal(t, first, second)
}

// TestRel00_1_UC001_PromptSections verifies section ordering and presence
// using table-driven cases (issue AC5).
func TestRel00_1_UC001_PromptSections(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		deps       []DepItem
		failures   []string
		mustHave   []string
		mustNotHave []string
	}{
		{
			name:     "normal first attempt",
			deps:     nil,
			failures: nil,
			mustHave: []string{
				"Problem Statement",
				"Goals",
				"Acceptance Criteria",
				"Requirement Items to Implement",
				"Output Format",
			},
			mustNotHave: []string{"Already Implemented", "Retry Context"},
		},
		{
			name:     "with deps and failures",
			deps:     []DepItem{{ID: "R0.1", Files: []string{"a.go"}}},
			failures: []string{"build failed"},
			mustHave: []string{
				"Problem Statement",
				"Already Implemented",
				"Retry Context",
			},
		},
		{
			name:     "deps only",
			deps:     []DepItem{{ID: "R0.1", Files: []string{"a.go"}}},
			failures: nil,
			mustHave:    []string{"Already Implemented"},
			mustNotHave: []string{"Retry Context"},
		},
		{
			name:     "failures only",
			deps:     nil,
			failures: []string{"test failed"},
			mustHave:    []string{"Retry Context"},
			mustNotHave: []string{"Already Implemented"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prompt, err := AssemblePrompt(testTask, testSRD, tc.deps, tc.failures)
			require.NoError(t, err)

			for _, s := range tc.mustHave {
				assert.True(t, strings.Contains(prompt, s),
					"prompt should contain %q", s)
			}
			for _, s := range tc.mustNotHave {
				assert.False(t, strings.Contains(prompt, s),
					"prompt should not contain %q", s)
			}
		})
	}
}
