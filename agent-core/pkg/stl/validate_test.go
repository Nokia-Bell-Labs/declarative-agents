// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func TestToolTracker_Record_And_Skipped(t *testing.T) {
	t.Parallel()
	tr := NewToolTracker()
	require.Equal(t, []string{"build", "lint", "test"}, tr.Skipped())

	tr.Record("build")
	require.Equal(t, []string{"lint", "test"}, tr.Skipped())

	tr.Record("lint")
	tr.Record("test")
	require.Nil(t, tr.Skipped())
}

func TestToolTracker_Reset(t *testing.T) {
	t.Parallel()
	tr := NewToolTracker()
	tr.Record("build")
	tr.Record("lint")
	tr.Record("test")
	require.Nil(t, tr.Skipped())

	tr.Reset()
	require.Equal(t, []string{"build", "lint", "test"}, tr.Skipped())
}

func TestValidateCmd_Name(t *testing.T) {
	t.Parallel()
	cmd := &validateCmd{}
	require.Equal(t, "validate", cmd.Name())
}

func TestValidateCmd_NoneSkipped(t *testing.T) {
	t.Parallel()
	cmd := &validateCmd{skipped: nil, builders: nil}
	res := cmd.Execute()
	require.Equal(t, core.ValidationPassed, res.Signal)
	require.Contains(t, res.Output, "no tools were skipped")
}

func TestValidateCmd_AllPass(t *testing.T) {
	t.Parallel()
	cmd := &validateCmd{
		skipped: []string{"build", "lint", "test"},
		builders: map[string]core.Builder{
			"build": &valStubBuilder{signal: core.ToolDone},
			"lint":  &valStubBuilder{signal: core.ToolDone},
			"test":  &valStubBuilder{signal: core.ToolDone},
		},
	}
	res := cmd.Execute()
	require.Equal(t, core.ValidationPassed, res.Signal)
	require.Contains(t, res.Output, "build")
	require.Contains(t, res.Output, "lint")
	require.Contains(t, res.Output, "test")
}

func TestValidateCmd_BuildFails_ShortCircuits(t *testing.T) {
	t.Parallel()
	lintCalled := false
	cmd := &validateCmd{
		skipped: []string{"build", "lint", "test"},
		builders: map[string]core.Builder{
			"build": &valStubBuilder{signal: core.ToolFailed, output: "compilation errors"},
			"lint":  &valCallTracker{called: &lintCalled, signal: core.ToolDone},
			"test":  &valStubBuilder{signal: core.ToolDone},
		},
	}
	res := cmd.Execute()
	require.Equal(t, core.ValidationFailed, res.Signal)
	require.Contains(t, res.Output, "build")
	require.Contains(t, res.Output, "compilation errors")
	require.Nil(t, res.Err)
	require.False(t, lintCalled, "lint should not run after build failure")
}

func TestValidateCmd_LintFails_TestSkipped(t *testing.T) {
	t.Parallel()
	testCalled := false
	cmd := &validateCmd{
		skipped: []string{"build", "lint", "test"},
		builders: map[string]core.Builder{
			"build": &valStubBuilder{signal: core.ToolDone},
			"lint":  &valStubBuilder{signal: core.ToolFailed, output: "lint violations"},
			"test":  &valCallTracker{called: &testCalled, signal: core.ToolDone},
		},
	}
	res := cmd.Execute()
	require.Equal(t, core.ValidationFailed, res.Signal)
	require.Contains(t, res.Output, "lint")
	require.False(t, testCalled)
}

func TestValidateCmd_CommandError_Propagates(t *testing.T) {
	t.Parallel()
	cmd := &validateCmd{
		skipped: []string{"build"},
		builders: map[string]core.Builder{
			"build": &valStubBuilder{
				signal: core.CommandError,
				err:    fmt.Errorf("go not found"),
			},
		},
	}
	res := cmd.Execute()
	require.Equal(t, core.CommandError, res.Signal)
	require.Error(t, res.Err)
	require.Contains(t, res.Err.Error(), "go not found")
}

func TestValidateCmd_DomainFailure_NilErr(t *testing.T) {
	t.Parallel()
	cmd := &validateCmd{
		skipped: []string{"test"},
		builders: map[string]core.Builder{
			"test": &valStubBuilder{signal: core.ToolFailed, output: "tests failed"},
		},
	}
	res := cmd.Execute()
	require.Equal(t, core.ValidationFailed, res.Signal)
	require.Nil(t, res.Err)
}

func TestValidateBuilder_Build(t *testing.T) {
	t.Parallel()
	tracker := NewToolTracker()
	b := &ValidateBuilder{
		Tracker:      tracker,
		BuildBuilder: &valStubBuilder{signal: core.ToolDone},
		LintBuilder:  &valStubBuilder{signal: core.ToolDone},
		TestBuilder:  &valStubBuilder{signal: core.ToolDone},
	}
	cmd := b.Build(core.Result{})
	require.Equal(t, "validate", cmd.Name())

	res := cmd.Execute()
	require.Equal(t, core.ValidationPassed, res.Signal)
}

func TestValidateToolSpec(t *testing.T) {
	t.Parallel()
	spec := ValidateToolSpec()
	require.Equal(t, "validate", spec.Name)
	require.Equal(t, core.Internal, spec.Visibility)
}

// --- test helpers ---

type valStubBuilder struct {
	signal core.Signal
	output string
	err    error
}

func (s *valStubBuilder) Build(_ core.Result) core.Command {
	return &valStubCmd{name: "stub", signal: s.signal, output: s.output, err: s.err}
}

type valStubCmd struct {
	name   string
	signal core.Signal
	output string
	err    error
}

func (s *valStubCmd) Name() string { return s.name }

func (s *valStubCmd) Execute() core.Result {
	return core.Result{
		Output:      s.output,
		Signal:      s.signal,
		Err:         s.err,
		CommandName: s.name,
	}
}

type valCallTracker struct {
	called *bool
	signal core.Signal
}

func (c *valCallTracker) Build(_ core.Result) core.Command {
	return &valCallTrackerCmd{called: c.called, signal: c.signal}
}

type valCallTrackerCmd struct {
	called *bool
	signal core.Signal
}

func (c *valCallTrackerCmd) Name() string { return "tracker" }

func (c *valCallTrackerCmd) Execute() core.Result {
	*c.called = true
	return core.Result{Signal: c.signal, CommandName: "tracker"}
}
