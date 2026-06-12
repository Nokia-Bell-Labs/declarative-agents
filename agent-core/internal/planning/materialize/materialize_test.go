// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package materialize

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gopkg.in/yaml.v3"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/telemetry"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/plan"
)

var testPlan = plan.ImplementationPlan{
	Title:   "Implement config parser",
	Summary: "Parse YAML config files.",
	Files: []plan.PlanFile{
		{Path: "internal/config/config.go", Action: "create", Note: "Config struct"},
		{Path: "internal/config/config_test.go", Action: "create"},
	},
	Requirements: []plan.PlanRequirement{
		{ID: "R1", Text: "Define Config struct"},
		{ID: "R2", Text: "Parse YAML files"},
		{ID: "R3", Text: "Validate fields"},
	},
	AcceptanceCriteria: []plan.PlanCriterion{
		{ID: "AC1", Text: "Config loads from file"},
		{ID: "AC2", Text: "Error on invalid YAML"},
	},
}

const testDir = "/tmp/test-repo-planner-run-1"

type bdCall struct {
	args []string
}

func mockBd(output string, err error) (func(context.Context, []string) ([]byte, error), *[]bdCall) {
	calls := &[]bdCall{}
	fn := func(_ context.Context, args []string) ([]byte, error) {
		*calls = append(*calls, bdCall{args: args})
		if err != nil {
			return nil, err
		}
		return []byte(output), nil
	}
	return fn, calls
}

// TestRel00_1_UC002_FormatProducesValidDescription verifies that
// FormatIssueDescription produces valid YAML with all required fields (srd009 AC1).
func TestRel00_1_UC002_FormatProducesValidDescription(t *testing.T) {
	t.Parallel()
	desc, err := FormatIssueDescription(testPlan)
	require.NoError(t, err)

	var parsed issueDescription
	require.NoError(t, yaml.Unmarshal([]byte(desc), &parsed))

	assert.Equal(t, "code", parsed.DeliverableType)
	assert.Len(t, parsed.RequiredReading, 2)
	assert.Len(t, parsed.Files, 2)
	assert.Len(t, parsed.Requirements, 3)
	assert.Len(t, parsed.AcceptanceCriteria, 2)
	assert.Empty(t, parsed.DesignDecisions)
}

// TestRel00_1_UC002_FormatIncludesDesignDecisions verifies the optional
// design_decisions field (srd009 AC2).
func TestRel00_1_UC002_FormatIncludesDesignDecisions(t *testing.T) {
	t.Parallel()

	t.Run("with decisions", func(t *testing.T) {
		t.Parallel()
		p := testPlan
		p.DesignDecisions = []plan.PlanDecision{
			{ID: "D1", Text: "Use yaml.v3"},
			{ID: "D2", Text: "Single file"},
		}
		desc, err := FormatIssueDescription(p)
		require.NoError(t, err)
		assert.Contains(t, desc, "design_decisions")
		assert.Contains(t, desc, "Use yaml.v3")
	})

	t.Run("without decisions", func(t *testing.T) {
		t.Parallel()
		desc, err := FormatIssueDescription(testPlan)
		require.NoError(t, err)
		assert.NotContains(t, desc, "design_decisions")
	})
}

// TestRel00_1_UC002_ExecuteInvokesBdCreateCorrectly verifies the
// bd create flags (srd009 AC3).
func TestRel00_1_UC002_ExecuteInvokesBdCreateCorrectly(t *testing.T) {
	t.Parallel()
	runner, calls := mockBd(`{"id":"planner-abc"}`, nil)
	mt := &MaterializeTask{RunBd: runner}

	_, err := mt.Execute(context.Background(), tracing.NoopTracer{}, testPlan, testDir, nil)
	require.NoError(t, err)
	require.Len(t, *calls, 1)

	args := (*calls)[0].args
	assert.Contains(t, args, "create")
	assert.Contains(t, args, "--json")
	assert.Contains(t, args, "-t")
	assert.Contains(t, args, "task")
	assert.Contains(t, args, "-l")
	assert.Contains(t, args, "code")
	assert.Contains(t, args, "-C")
	assert.Contains(t, args, testDir)
	assert.Contains(t, args, "--title")
	assert.Contains(t, args, testPlan.Title)
	assert.Contains(t, args, "--body-file")
}

// TestRel00_1_UC002_ExecuteWiresDependencies verifies --deps flag
// behavior (srd009 AC4).
func TestRel00_1_UC002_ExecuteWiresDependencies(t *testing.T) {
	t.Parallel()

	t.Run("with deps", func(t *testing.T) {
		t.Parallel()
		runner, calls := mockBd(`{"id":"planner-xyz"}`, nil)
		mt := &MaterializeTask{RunBd: runner}
		deps := map[string]string{"task-A": "planner-abc"}

		_, err := mt.Execute(context.Background(), tracing.NoopTracer{}, testPlan, testDir, deps)
		require.NoError(t, err)

		args := (*calls)[0].args
		joined := strings.Join(args, " ")
		assert.Contains(t, joined, "--deps")
		assert.Contains(t, joined, "planner-abc")
	})

	t.Run("without deps", func(t *testing.T) {
		t.Parallel()
		runner, calls := mockBd(`{"id":"planner-xyz"}`, nil)
		mt := &MaterializeTask{RunBd: runner}

		_, err := mt.Execute(context.Background(), tracing.NoopTracer{}, testPlan, testDir, nil)
		require.NoError(t, err)

		joined := strings.Join((*calls)[0].args, " ")
		assert.NotContains(t, joined, "--deps")
	})
}

// TestRel00_1_UC002_ExecuteReturnsIssueID verifies the returned
// issue ID from bd create JSON output (srd009 AC5).
func TestRel00_1_UC002_ExecuteReturnsIssueID(t *testing.T) {
	t.Parallel()
	runner, _ := mockBd(`{"id":"planner-xyz"}`, nil)
	mt := &MaterializeTask{RunBd: runner}

	id, err := mt.Execute(context.Background(), tracing.NoopTracer{}, testPlan, testDir, nil)
	require.NoError(t, err)
	assert.Equal(t, "planner-xyz", id)
}

// TestRel00_1_UC002_ExecuteReturnsMaterializeFailedOnError verifies
// error handling for bd failures (srd009 AC6).
func TestRel00_1_UC002_ExecuteReturnsMaterializeFailedOnError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		output      string
		err         error
		errContains string
	}{
		{
			name:        "bd exit failure",
			output:      "",
			err:         fmt.Errorf("database locked"),
			errContains: "database locked",
		},
		{
			name:        "unparseable JSON",
			output:      "not json at all",
			err:         nil,
			errContains: "parse bd output",
		},
		{
			name:        "missing id field",
			output:      `{"status":"ok"}`,
			err:         nil,
			errContains: "empty issue ID",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner, _ := mockBd(tc.output, tc.err)
			mt := &MaterializeTask{RunBd: runner}

			_, err := mt.Execute(context.Background(), tracing.NoopTracer{}, testPlan, testDir, nil)
			require.Error(t, err)

			var mf *MaterializeFailed
			require.True(t, errors.As(err, &mf), "expected MaterializeFailed, got %T", err)
			assert.Contains(t, mf.Error(), tc.errContains)
		})
	}
}

// TestRel00_1_UC002_TempFileCleanedUp verifies the temp description
// file is removed after Execute (srd009 AC7).
func TestRel00_1_UC002_TempFileCleanedUp(t *testing.T) {
	t.Parallel()

	t.Run("after success", func(t *testing.T) {
		t.Parallel()
		var capturedFile string
		runner := func(_ context.Context, args []string) ([]byte, error) {
			for i, a := range args {
				if a == "--body-file" && i+1 < len(args) {
					capturedFile = args[i+1]
				}
			}
			return []byte(`{"id":"x"}`), nil
		}
		mt := &MaterializeTask{RunBd: runner}

		_, err := mt.Execute(context.Background(), tracing.NoopTracer{}, testPlan, testDir, nil)
		require.NoError(t, err)
		require.NotEmpty(t, capturedFile)
		_, statErr := os.Stat(capturedFile)
		assert.True(t, os.IsNotExist(statErr), "temp file should be removed after success")
	})

	t.Run("after failure", func(t *testing.T) {
		t.Parallel()
		var capturedFile string
		runner := func(_ context.Context, args []string) ([]byte, error) {
			for i, a := range args {
				if a == "--body-file" && i+1 < len(args) {
					capturedFile = args[i+1]
				}
			}
			return nil, fmt.Errorf("bd failed")
		}
		mt := &MaterializeTask{RunBd: runner}

		_, err := mt.Execute(context.Background(), tracing.NoopTracer{}, testPlan, testDir, nil)
		require.Error(t, err)
		require.NotEmpty(t, capturedFile)
		_, statErr := os.Stat(capturedFile)
		assert.True(t, os.IsNotExist(statErr), "temp file should be removed after failure")
	})
}

// TestRel00_1_UC002_ExecuteCreatesOTelSpan verifies the materialize_task
// span with correct attributes (srd009 AC8).
func TestRel00_1_UC002_ExecuteCreatesOTelSpan(t *testing.T) {
	tracer, exporter := telemetry.NewTestTracer(t, "test")

	runner, _ := mockBd(`{"id":"planner-span"}`, nil)
	mt := &MaterializeTask{RunBd: runner}
	deps := map[string]string{"t1": "planner-dep1"}

	id, err := mt.Execute(context.Background(), tracer, testPlan, testDir, deps)
	require.NoError(t, err)
	assert.Equal(t, "planner-span", id)

	spans := exporter.GetSpans()
	var matSpan *tracetest.SpanStub
	for i := range spans {
		if spans[i].Name == "materialize_task" {
			matSpan = &spans[i]
			break
		}
	}
	require.NotNil(t, matSpan, "expected materialize_task span")

	assertMatSpanAttr(t, matSpan, "plan_title", testPlan.Title)
	assertMatSpanAttr(t, matSpan, "issue_id", "planner-span")

	var foundDepCount bool
	for _, a := range matSpan.Attributes {
		if string(a.Key) == "dep_count" {
			assert.Equal(t, int64(1), a.Value.AsInt64())
			foundDepCount = true
		}
	}
	assert.True(t, foundDepCount, "span missing dep_count attribute")
}

func assertMatSpanAttr(t *testing.T, span *tracetest.SpanStub, key, want string) {
	t.Helper()
	for _, a := range span.Attributes {
		if string(a.Key) == key {
			assert.Equal(t, want, a.Value.AsString(), "span %s attr %s", span.Name, key)
			return
		}
	}
	t.Errorf("span %s missing attribute %s", span.Name, key)
}
