// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// doneCmd signals task completion. The parse_response command handles
// the "done" tool specially — it extracts the summary and returns
// TaskCompleted. This builder exists so the tool validates in the
// registry but is never actually dispatched.
type doneCmd struct{}

func (doneCmd) Name() string      { return "done" }
func (doneCmd) Undo() core.Result { return core.NoopUndo("done") }

func (doneCmd) Execute() core.Result {
	return core.Result{
		Signal:      core.TaskCompleted,
		Output:      "task completed",
		CommandName: "done",
	}
}

// DoneBuilder constructs done commands.
type DoneBuilder struct{}

func (DoneBuilder) Build(_ core.Result) core.Command {
	return doneCmd{}
}
