// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package servercmd

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Await is a REST event fan-in command.
type Await struct {
	toolName string
	hooks    AwaitHooks
}

// NewAwait creates one REST event fan-in command.
func NewAwait(toolName string, hooks AwaitHooks) core.Command {
	return Await{toolName: toolName, hooks: hooks}
}

func (c Await) Name() string { return c.toolName }

func (c Await) Execute() core.Result {
	event, signal, err := c.hooks.AwaitAny()
	if err != nil {
		return commandError(c.toolName, err)
	}
	return awaitCommandResult(c.toolName, event, signal)
}

func (c Await) ExecuteContext(ctx context.Context) core.Result {
	event, signal, err := c.hooks.AwaitAnyContext(ctx)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return awaitCommandResult(c.toolName, event, signal)
}

func (c Await) Undo(prior core.Result) core.Result {
	return restoreAwaitReceipt(c.toolName, c.hooks.RestoreEvent, prior.Receipt)
}

func restoreAwaitReceipt(
	commandName string,
	restore func(server string, event Event) error,
	receipt string,
) core.Result {
	if receipt == "" {
		return core.NoopUndo(commandName)
	}
	var decoded awaitReceipt
	if err := json.Unmarshal([]byte(receipt), &decoded); err != nil {
		return commandError(commandName, fmt.Errorf("decode REST await receipt: %w", err))
	}
	if decoded.Server == "" || decoded.Event.Signal == "" {
		return commandError(commandName, fmt.Errorf("decode REST await receipt: server and event signal are required"))
	}
	if err := restore(decoded.Server, decoded.Event); err != nil {
		return commandError(commandName, fmt.Errorf("restore REST await event: %w", err))
	}
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "restored consumed REST event"}
}
