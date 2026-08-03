// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClassify_Clean(t *testing.T) {
	t.Parallel()

	snaps := []ToolSnapshot{
		{Tool: "test", Signal: "ToolDone", Total: 4, Passed: 4, Failed: 0},
		{Tool: "build", Signal: "ToolDone", Total: 0, Passed: 0, Failed: 0},
	}
	prog := Classify(snaps, true)
	assert.Equal(t, Clean, prog.Overall)
}

func TestClassify_Converged(t *testing.T) {
	t.Parallel()

	snaps := []ToolSnapshot{
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 1, Failed: 3},
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 3, Failed: 1},
		{Tool: "test", Signal: "ToolDone", Total: 4, Passed: 4, Failed: 0},
	}
	prog := Classify(snaps, true)
	assert.Equal(t, Converged, prog.Overall)
}

func TestClassify_Improving(t *testing.T) {
	t.Parallel()

	snaps := []ToolSnapshot{
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 0, Failed: 4},
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 2, Failed: 2},
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 3, Failed: 1},
	}
	prog := Classify(snaps, false)
	assert.Equal(t, Improving, prog.Overall)
}

func TestClassify_Flat(t *testing.T) {
	t.Parallel()

	snaps := []ToolSnapshot{
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 2, Failed: 2},
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 2, Failed: 2},
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 2, Failed: 2},
	}
	prog := Classify(snaps, false)
	assert.Equal(t, Flat, prog.Overall)
}

func TestClassify_Regressing(t *testing.T) {
	t.Parallel()

	snaps := []ToolSnapshot{
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 3, Failed: 1},
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 1, Failed: 3},
	}
	prog := Classify(snaps, false)
	assert.Equal(t, Regressing, prog.Overall)
}

func TestClassify_NoData(t *testing.T) {
	t.Parallel()

	prog := Classify(nil, false)
	assert.Equal(t, NoData, prog.Overall)
}

func TestClassify_MultiTool(t *testing.T) {
	t.Parallel()

	snaps := []ToolSnapshot{
		{Tool: "build", Signal: "ToolFailed", Total: 2, Passed: 0, Failed: 2},
		{Tool: "build", Signal: "ToolDone", Total: 0, Passed: 0, Failed: 0},
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 1, Failed: 3},
		{Tool: "test", Signal: "ToolDone", Total: 4, Passed: 4, Failed: 0},
	}
	prog := Classify(snaps, true)
	assert.Equal(t, Converged, prog.Overall)
	assert.Len(t, prog.Tools, 2)
}

// multiToolSnapshots exercises three tools whose alphabetical order differs
// from their first appearance, so a stable result cannot come from insertion
// order alone.
func multiToolSnapshots() []ToolSnapshot {
	return []ToolSnapshot{
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 1, Failed: 3},
		{Tool: "test", Signal: "ToolDone", Total: 4, Passed: 4, Failed: 0},
		{Tool: "build", Signal: "ToolFailed", Total: 2, Passed: 0, Failed: 2},
		{Tool: "build", Signal: "ToolFailed", Total: 2, Passed: 1, Failed: 1},
		{Tool: "lint", Signal: "ToolFailed", Total: 1, Passed: 0, Failed: 1},
		{Tool: "lint", Signal: "ToolFailed", Total: 1, Passed: 0, Failed: 1},
	}
}

// TestClassify_MultiToolDeterministicOrder asserts tool progressions come out
// sorted by tool name regardless of snapshot arrival order.
func TestClassify_MultiToolDeterministicOrder(t *testing.T) {
	t.Parallel()
	prog := Classify(multiToolSnapshots(), false)
	require.Len(t, prog.Tools, 3)
	names := []string{prog.Tools[0].Tool, prog.Tools[1].Tool, prog.Tools[2].Tool}
	assert.Equal(t, []string{"build", "lint", "test"}, names)
}

// TestClassify_ByteStableAcrossRepeatedRuns is the GH-1358 regression guard: an
// identical snapshot set must serialize to identical bytes -- tools array and
// summary -- across many runs, despite Go's randomized map iteration order.
func TestClassify_ByteStableAcrossRepeatedRuns(t *testing.T) {
	t.Parallel()
	snaps := multiToolSnapshots()

	first, err := json.Marshal(Classify(snaps, false))
	require.NoError(t, err)
	firstSummary := Classify(snaps, false).Summary

	for i := 0; i < 1000; i++ {
		got, err := json.Marshal(Classify(snaps, false))
		require.NoError(t, err)
		require.Equal(t, string(first), string(got), "identical input must serialize identically")
		require.Equal(t, firstSummary, Classify(snaps, false).Summary, "summary must be stable")
	}
}

func TestFormatTimeline(t *testing.T) {
	t.Parallel()

	snaps := []ToolSnapshot{
		{Tool: "test", Signal: "ToolFailed", Total: 0, Passed: 0, Failed: 0},
		{Tool: "test", Signal: "ToolFailed", Total: 4, Passed: 1, Failed: 3},
		{Tool: "test", Signal: "ToolDone", Total: 4, Passed: 4, Failed: 0},
	}
	tl := formatTimeline("test", snaps)
	assert.Equal(t, "BUILD_FAIL → 1ok/3fail → PASS", tl)
}
