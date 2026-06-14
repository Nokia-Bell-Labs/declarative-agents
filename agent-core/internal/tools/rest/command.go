// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// RestBuilder constructs declarative REST boundary commands.
type RestBuilder struct {
	ToolName string
	Init     string
	Signal   core.Signal
}

// Build creates one REST boundary command.
func (b RestBuilder) Build(_ core.Result) core.Command {
	return restCmd{toolName: b.ToolName, init: b.Init, signal: b.Signal}
}

type restCmd struct {
	toolName string
	init     string
	signal   core.Signal
}

func (c restCmd) Name() string { return c.toolName }

func (c restCmd) Execute() core.Result {
	err := fmt.Errorf("%s transport execution is not implemented", c.init)
	return core.Result{Signal: core.CommandError, CommandName: c.toolName, Output: err.Error(), Err: err}
}

func (c restCmd) Undo() core.Result {
	return core.NoopUndo(c.toolName)
}
