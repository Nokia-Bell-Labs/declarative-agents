// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

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

// BuildReverser creates a fresh server command for receipt-driven rollback.
func (b ServerBuilder) BuildReverser() core.Command {
	return serverCmd{toolName: b.ToolName, init: b.Init, server: b.Server, state: b.State}
}

// Build creates one REST event fan-in command.
func (b AwaitEventBuilder) Build(_ core.Result) core.Command {
	return awaitEventCmd{toolName: b.ToolName, options: b.Options, state: b.State}
}

// BuildReverser creates a fresh fan-in command for receipt-driven rollback.
func (b AwaitEventBuilder) BuildReverser() core.Command {
	return awaitEventCmd{toolName: b.ToolName, options: b.Options, state: b.State}
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
	return c.awaitResult(event, signal)
}

func (c serverCmd) Undo(prior core.Result) core.Result {
	if c.init == InitServerAwait {
		return restoreAwaitReceipt(c.toolName, c.state, prior.Receipt)
	}
	if c.init == InitServerStop {
		return c.undoStop(prior.Receipt)
	}
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
	return c.awaitResult(event, signal)
}

func (c serverCmd) stop() core.Result {
	output, err := c.state.Stop(c.server.Name)
	if err != nil {
		return commandError(c.toolName, err)
	}
	receipt := undo.EncodeBoundaryReceipt(undo.BoundaryCompensationPayload{
		BoundaryCompensation: undo.BoundaryCompensation{
			Strategy: "server_shutdown_or_user_action_compensation",
			Reason:   "server listener stopped and queued events drained",
			Requires: []string{"machine_owned_server_relaunch"},
			Data: map[string]interface{}{
				"server_addr": stringValue(output["address"]),
				"rest_ref":    c.server.Name, "compensation": output,
			},
		},
	})
	return core.Result{
		Signal: core.Signal("ServerStopped"), CommandName: c.toolName,
		Output: jsonOutput(output), Receipt: receipt,
	}
}

func (c awaitEventCmd) Name() string { return c.toolName }

func (c awaitEventCmd) Execute() core.Result {
	event, signal, err := c.state.AwaitAny(c.options)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return awaitCommandResult(c.toolName, event, signal)
}

func (c awaitEventCmd) ExecuteContext(ctx context.Context) core.Result {
	event, signal, err := c.state.AwaitAnyContext(ctx, c.options)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return awaitCommandResult(c.toolName, event, signal)
}

func (c awaitEventCmd) Undo(prior core.Result) core.Result {
	return restoreAwaitReceipt(c.toolName, c.state, prior.Receipt)
}

type awaitReceipt struct {
	Server string       `json:"server"`
	Event  InboundEvent `json:"event"`
}

func (c serverCmd) awaitResult(event InboundEvent, signal string) core.Result {
	return awaitCommandResult(c.toolName, event, signal)
}

func awaitCommandResult(commandName string, event InboundEvent, signal string) core.Result {
	result := core.Result{Signal: core.Signal(signal), CommandName: commandName, Output: eventOutput(event)}
	if event.Signal == "" {
		return result
	}
	receipt, err := json.Marshal(awaitReceipt{Server: event.Source, Event: event})
	if err != nil {
		return commandError(commandName, fmt.Errorf("encode REST await receipt: %w", err))
	}
	result.Receipt = string(receipt)
	return result
}

func restoreAwaitReceipt(commandName string, state *ServerState, receipt string) core.Result {
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
	if err := state.RestoreEvent(decoded.Server, decoded.Event); err != nil {
		return commandError(commandName, fmt.Errorf("restore REST await event: %w", err))
	}
	return core.Result{Signal: core.ToolDone, CommandName: commandName, Output: "restored consumed REST event"}
}

func (c serverCmd) undoStop(receipt string) core.Result {
	compensation, ok, err := undo.DecodeBoundaryReceipt(receipt)
	if err != nil {
		return commandError(c.toolName, err)
	}
	if !ok || compensation.Strategy != "server_shutdown_or_user_action_compensation" {
		return commandError(c.toolName, fmt.Errorf("REST stop receipt has no server relaunch compensation"))
	}
	restRef := stringValue(compensation.Data["rest_ref"])
	if restRef != c.server.Name {
		return commandError(c.toolName, fmt.Errorf(
			"REST stop receipt server %q does not match configured server %q",
			restRef, c.server.Name,
		))
	}
	compensationData, _ := compensation.Data["compensation"].(map[string]interface{})
	return undo.BoundaryCompensationUndo(c.toolName, fmt.Sprintf(
		"MachineSpec must relaunch server %q at %q; stop drained %v queued events",
		restRef, stringValue(compensation.Data["server_addr"]), compensationData["drained_events"],
	))
}

func stringValue(value interface{}) string {
	text, _ := value.(string)
	return text
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
