// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"encoding/json"
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

// ServerBuilder constructs REST server launch, await, and stop commands.
type ServerBuilder struct {
	ToolName string
	Init     string
	Server   ServerDefinition
	State    *ServerState
}

// Build creates one REST server boundary command.
func (b ServerBuilder) Build(_ core.Result) core.Command {
	return serverCmd{toolName: b.ToolName, init: b.Init, server: b.Server, state: b.State}
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

type serverCmd struct {
	toolName string
	init     string
	server   ServerDefinition
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

func (c serverCmd) Undo() core.Result {
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
