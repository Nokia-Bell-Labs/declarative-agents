// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

// ServerBuilder constructs REST server launch, await, and stop commands.
type ServerBuilder struct {
	ToolName string
	Init     string
	Server   ServerDefinition
	State    *ServerState
}

// AwaitEventBuilder constructs REST event fan-in commands.
type AwaitEventBuilder struct {
	ToolName string
	Options  AwaitAnyOptions
	State    *ServerState
}

// Build creates one REST server boundary command.
func (b ServerBuilder) Build(_ core.Result) core.Command {
	return serverCmd{toolName: b.ToolName, init: b.Init, server: b.Server, state: b.State}
}

// Build creates one REST event fan-in command.
func (b AwaitEventBuilder) Build(_ core.Result) core.Command {
	return awaitEventCmd{toolName: b.ToolName, options: b.Options, state: b.State}
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

func (c restCmd) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.toolName)
}

type serverCmd struct {
	toolName string
	init     string
	server   ServerDefinition
	state    *ServerState
}

type awaitEventCmd struct {
	toolName string
	options  AwaitAnyOptions
	state    *ServerState
}

func (c serverCmd) Name() string { return c.toolName }

func (c serverCmd) Execute() core.Result {
	switch c.init {
	case InitServerLaunch:
		return c.launch()
	case InitServerAwait:
		return c.await()
	case InitServerStop:
		return c.stop()
	default:
		err := fmt.Errorf("unsupported REST server init %q", c.init)
		return commandError(c.toolName, err)
	}
}

func (c serverCmd) ExecuteContext(ctx context.Context) core.Result {
	if c.init != InitServerAwait {
		return c.Execute()
	}
	event, signal, err := c.state.AwaitContext(ctx, c.server.Name)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{Signal: core.Signal(signal), CommandName: c.toolName, Output: eventOutput(event)}
}

func (c serverCmd) Undo(_ core.Result) core.Result {
	if c.init != InitServerLaunch {
		return core.NoopUndo(c.toolName)
	}
	output, err := c.state.Stop(c.server.Name)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{Signal: core.Signal("ServerStopped"), CommandName: c.toolName, Output: jsonOutput(output)}
}

func (c serverCmd) launch() core.Result {
	output, err := c.state.Launch(c.server)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{Signal: core.Signal("ServerLaunched"), CommandName: c.toolName, Output: jsonOutput(output)}
}

func (c serverCmd) await() core.Result {
	event, signal, err := c.state.Await(c.server.Name)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{Signal: core.Signal(signal), CommandName: c.toolName, Output: eventOutput(event)}
}

func (c serverCmd) stop() core.Result {
	output, err := c.state.Stop(c.server.Name)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{Signal: core.Signal("ServerStopped"), CommandName: c.toolName, Output: jsonOutput(output)}
}

func (c awaitEventCmd) Name() string { return c.toolName }

func (c awaitEventCmd) Execute() core.Result {
	event, signal, err := c.state.AwaitAny(c.options)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{Signal: core.Signal(signal), CommandName: c.toolName, Output: eventOutput(event)}
}

func (c awaitEventCmd) ExecuteContext(ctx context.Context) core.Result {
	event, signal, err := c.state.AwaitAnyContext(ctx, c.options)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{Signal: core.Signal(signal), CommandName: c.toolName, Output: eventOutput(event)}
}

func (c awaitEventCmd) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.toolName)
}

func commandError(commandName string, err error) core.Result {
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
}

func eventOutput(event InboundEvent) string {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}
