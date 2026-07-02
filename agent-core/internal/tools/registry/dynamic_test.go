// Copyright (c) 2026 Nokia. All rights reserved.

package registry

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type recordingTracker struct {
	names []string
}

func (r *recordingTracker) Record(name string) {
	r.names = append(r.names, name)
}

type namedBuilder struct {
	name     string
	executed *bool
}

func (b namedBuilder) Build(core.Result) core.Command {
	if b.executed != nil {
		*b.executed = true
	}
	return namedCmd{name: b.name}
}

type namedCmd struct {
	name string
}

func (c namedCmd) Name() string { return c.name }
func (c namedCmd) Execute() core.Result {
	return core.Result{Signal: core.ToolDone, CommandName: c.name}
}
func (c namedCmd) Undo() core.Result { return core.NoopUndo(c.name) }

func TestBuildDynamicToolActionDispatchesAndTracks(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	reg.Register(core.ToolSpec{Name: "read"}, namedBuilder{name: "read"})
	tracker := &recordingTracker{}
	action := BuildDynamicToolAction(DynamicToolActionDeps{Registry: reg, Tracker: tracker})

	cmd := action(core.Result{Output: `{"tool":"read","parameters":{"path":"x"}}`})
	res := cmd.Execute()

	require.Equal(t, "read", cmd.Name())
	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, []string{"read"}, tracker.names)
}

func TestBuildDynamicToolActionUnknownToolReturnsCommandError(t *testing.T) {
	t.Parallel()
	action := BuildDynamicToolAction(DynamicToolActionDeps{Registry: core.NewRegistry()})

	res := action(core.Result{Output: `{"tool":"missing","parameters":{}}`}).Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "no builder")
}

func TestBuildDynamicToolActionRejectsInternalTool(t *testing.T) {
	t.Parallel()
	reg := core.NewRegistry()
	var executed bool
	reg.Register(core.ToolSpec{Name: "launch_monitor_rest", Visibility: core.Internal}, namedBuilder{
		name: "launch_monitor_rest", executed: &executed,
	})
	tracker := &recordingTracker{}
	action := BuildDynamicToolAction(DynamicToolActionDeps{Registry: reg, Tracker: tracker})

	cmd := action(core.Result{Output: `{"tool":"launch_monitor_rest","parameters":{}}`})
	res := cmd.Execute()

	require.Equal(t, "fail", cmd.Name())
	require.Equal(t, core.CommandError, res.Signal)
	require.Contains(t, res.Output, "not available for dynamic dispatch")
	require.False(t, executed)
	require.Empty(t, tracker.names)
}
