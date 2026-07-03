// Copyright (c) 2026 Nokia. All rights reserved.

package control

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"

type doneCmd struct{}

func (doneCmd) Name() string                   { return "done" }
func (doneCmd) Undo(_ core.Result) core.Result { return core.NoopUndo("done") }

func (doneCmd) Execute() core.Result {
	return core.Result{Signal: core.TaskCompleted, Output: "task completed", CommandName: "done"}
}

// DoneBuilder constructs done commands.
type DoneBuilder struct{}

func (DoneBuilder) Build(_ core.Result) core.Command {
	return doneCmd{}
}
