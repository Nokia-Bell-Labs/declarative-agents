// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package validation

import (
	"encoding/json"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

const reducerSelector = "$from(results).items"

// reducerView wraps a raw JSON array of joined outcomes as the {"items": ...}
// payload the join step emits, so a $from(results).items selector resolves to
// the array.
func reducerView(t *testing.T, itemsJSON string) core.CommandStateView {
	t.Helper()
	return viewFromPayloads(map[string]string{"results": `{"items":` + itemsJSON + `}`})
}

func marshalItems(t *testing.T, items []map[string]any) string {
	t.Helper()
	data, err := json.Marshal(items)
	require.NoError(t, err)
	return string(data)
}

// structuredResult embeds an rg/scan result as the JSON string the reducer
// expects at outcome.result.output.
func structuredResult(t *testing.T, fields map[string]any) string {
	t.Helper()
	data, err := json.Marshal(fields)
	require.NoError(t, err)
	return string(data)
}

func grepMissingItem(t *testing.T, suiteID, severity string) map[string]any {
	return map[string]any{
		"input": map[string]any{
			"suite_id": suiteID, "check_id": "c1", "kind": "grep_check",
			"severity": severity, "mode": "missing", "patterns": []string{"license"},
		},
		"result": map[string]any{"output": structuredResult(t, map[string]any{"output": "", "exit_code": 1})},
	}
}

func TestReduceGrepChecksCommand(t *testing.T) {
	t.Parallel()

	t.Run("empty results pass with a receipt", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		cmd := (&ReduceGrepChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, "[]"))

		result := cmd.Execute()
		require.Equal(t, core.ValidationPassed, result.Signal, result.Output)
		require.Empty(t, vs.Findings)
		require.False(t, vs.HasErrors)
		require.NotEmpty(t, result.Receipt)
	})

	t.Run("findings accumulate across outcomes, sort by suite, and fail", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		// Both errors so the suite-level tiebreak is observable (sortFindings
		// orders by level first, then suite).
		items := marshalItems(t, []map[string]any{
			grepMissingItem(t, "suite-b", "error"),
			grepMissingItem(t, "suite-a", "error"),
		})
		cmd := (&ReduceGrepChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, items))

		result := cmd.Execute()
		require.Equal(t, core.ValidationFailed, result.Signal, result.Output)
		require.Len(t, vs.Findings, 2)
		require.Equal(t, "suite-a", vs.Findings[0].SuiteID, "findings must be sorted by suite")
		require.Equal(t, "suite-b", vs.Findings[1].SuiteID)
		require.True(t, vs.HasErrors)
	})

	t.Run("all-warning findings pass without errors", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		cmd := (&ReduceGrepChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, marshalItems(t, []map[string]any{
			grepMissingItem(t, "suite-a", "warning"),
		})))

		result := cmd.Execute()
		require.Equal(t, core.ValidationPassed, result.Signal, result.Output)
		require.Len(t, vs.Findings, 1)
		require.False(t, vs.HasErrors, "warnings alone are not errors")
	})

	t.Run("malformed joined JSON is a command error", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		cmd := (&ReduceGrepChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, `{"not":"an array"}`))

		result := cmd.Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.Contains(t, result.Output, "decode joined outcomes")
	})

	t.Run("malformed nested output is a command error", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		items := marshalItems(t, []map[string]any{{
			"input":  map[string]any{"suite_id": "s", "check_id": "c", "kind": "grep_check"},
			"result": map[string]any{"output": "not-json"},
		}})
		cmd := (&ReduceGrepChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, items))

		result := cmd.Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.Contains(t, result.Output, "decode structured rg result")
	})

	t.Run("selector error is a command error", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		cmd := (&ReduceGrepChecksBuilder{VS: vs, ResultsFrom: "$from(absent).items"}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, "[]"))

		result := cmd.Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.Contains(t, result.Output, "reduce grep checks failed")
	})

	t.Run("undo restores prior findings", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		cmd := (&ReduceGrepChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, marshalItems(t, []map[string]any{
			grepMissingItem(t, "suite-b", "error"),
		})))

		result := cmd.Execute()
		require.Equal(t, core.ValidationFailed, result.Signal)
		require.Len(t, vs.Findings, 1)

		undo := cmd.Undo(result)
		require.Equal(t, core.ToolDone, undo.Signal, undo.Output)
		require.Empty(t, vs.Findings)
		require.False(t, vs.HasErrors)
	})
}

func TestReduceRefChecksCommand(t *testing.T) {
	t.Parallel()

	t.Run("empty results pass with a receipt", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		cmd := (&ReduceRefChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, "[]"))

		result := cmd.Execute()
		require.Equal(t, core.ValidationPassed, result.Signal, result.Output)
		require.NotEmpty(t, result.Receipt)
	})

	t.Run("reduce error surfaces as a command error", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		items := marshalItems(t, []map[string]any{{
			"input":  map[string]any{"suite_id": "s", "check_id": "c", "kind": "ref_check", "severity": "error"},
			"result": map[string]any{"output": structuredResult(t, map[string]any{"output": "garbage without markers"})},
		}})
		cmd := (&ReduceRefChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, items))

		result := cmd.Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.Contains(t, result.Output, "reduce ref checks failed")
	})

	t.Run("malformed nested output is a command error", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		items := marshalItems(t, []map[string]any{{
			"input":  map[string]any{"suite_id": "s", "check_id": "c", "kind": "ref_check"},
			"result": map[string]any{"output": "not-json"},
		}})
		cmd := (&ReduceRefChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, items))

		result := cmd.Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.Contains(t, result.Output, "decode structured ref scan")
	})

	t.Run("selector error and undo", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		bad := (&ReduceRefChecksBuilder{VS: vs, ResultsFrom: "$from(absent).items"}).Build(core.Result{})
		bad.(core.CommandStateAware).SetCommandState(reducerView(t, "[]"))
		require.Equal(t, core.CommandError, bad.Execute().Signal)

		ok := (&ReduceRefChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		ok.(core.CommandStateAware).SetCommandState(reducerView(t, "[]"))
		result := ok.Execute()
		require.Equal(t, core.ToolDone, ok.Undo(result).Signal)
	})
}

func TestReduceConsistencyChecksCommand(t *testing.T) {
	t.Parallel()

	t.Run("empty results pass with a receipt", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		cmd := (&ReduceConsistencyChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, "[]"))

		result := cmd.Execute()
		require.Equal(t, core.ValidationPassed, result.Signal, result.Output)
		require.NotEmpty(t, result.Receipt)
	})

	t.Run("reduce error surfaces as a command error", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		items := marshalItems(t, []map[string]any{{
			"input":  map[string]any{"suite_id": "s", "check": map[string]any{"id": "c"}},
			"result": map[string]any{"output": structuredResult(t, map[string]any{"output": "record-without-tab"})},
		}})
		cmd := (&ReduceConsistencyChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, items))

		result := cmd.Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.Contains(t, result.Output, "reduce consistency checks failed")
	})

	t.Run("malformed nested output is a command error", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		items := marshalItems(t, []map[string]any{{
			"input":  map[string]any{"suite_id": "s", "check": map[string]any{"id": "c"}},
			"result": map[string]any{"output": "not-json"},
		}})
		cmd := (&ReduceConsistencyChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		cmd.(core.CommandStateAware).SetCommandState(reducerView(t, items))

		result := cmd.Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.Contains(t, result.Output, "decode structured consistency scan")
	})

	t.Run("selector error and undo", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{}
		bad := (&ReduceConsistencyChecksBuilder{VS: vs, ResultsFrom: "$from(absent).items"}).Build(core.Result{})
		bad.(core.CommandStateAware).SetCommandState(reducerView(t, "[]"))
		require.Equal(t, core.CommandError, bad.Execute().Signal)

		ok := (&ReduceConsistencyChecksBuilder{VS: vs, ResultsFrom: reducerSelector}).Build(core.Result{})
		ok.(core.CommandStateAware).SetCommandState(reducerView(t, "[]"))
		require.Equal(t, core.ToolDone, ok.Undo(ok.Execute()).Signal)
	})
}

func TestFormatReportCommand(t *testing.T) {
	t.Parallel()

	t.Run("clean report is ToolDone", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{
			Stderr: io.Discard,
			Corpus: &spec.Corpus{},
			Findings: []spec.Finding{
				{Check: "x", Level: "warning", SuiteID: "s", Message: "heads up"},
			},
		}
		result := (&FormatReportBuilder{VS: vs}).Build(core.Result{}).Execute()
		require.Equal(t, core.ToolDone, result.Signal)
		require.Contains(t, result.Output, "validate:")
		require.Contains(t, result.Output, "OK")
	})

	t.Run("report with errors is ToolFailed", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{
			Stderr:    io.Discard,
			Corpus:    &spec.Corpus{},
			HasErrors: true,
			Findings: []spec.Finding{
				{Check: "x", Level: "error", SuiteID: "s", Message: "boom"},
			},
		}
		result := (&FormatReportBuilder{VS: vs}).Build(core.Result{}).Execute()
		require.Equal(t, core.ToolFailed, result.Signal)
		require.Contains(t, result.Output, "error(s)")
	})

	t.Run("undo is a noop", func(t *testing.T) {
		t.Parallel()
		vs := &SpecState{Stderr: io.Discard, Corpus: &spec.Corpus{}}
		undo := (&FormatReportBuilder{VS: vs}).Build(core.Result{}).Undo(core.Result{})
		require.Equal(t, core.ToolDone, undo.Signal)
	})
}
