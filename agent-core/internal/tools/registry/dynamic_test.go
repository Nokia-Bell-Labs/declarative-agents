// Copyright (c) 2026 Nokia. All rights reserved.

package registry

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

type recordingTracker struct {
	names []string
}

func (r *recordingTracker) Record(name string) {
	r.names = append(r.names, name)
}

type namedBuilder struct {
	name string
}

func (b namedBuilder) Build(core.Result) core.Command {
	return namedCmd(b)
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
