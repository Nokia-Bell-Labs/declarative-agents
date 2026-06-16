// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
