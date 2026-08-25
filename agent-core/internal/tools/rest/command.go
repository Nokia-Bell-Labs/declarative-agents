// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"context"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/servercmd"
)

const serverLaunchReceiptStrategy = servercmd.LaunchReceiptStrategy

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
	return servercmd.NewServer(b.ToolName, b.Init, b.serverHooks())
}

// BuildReverser creates a fresh server command for receipt-driven rollback.
func (b ServerBuilder) BuildReverser() core.Command {
	return servercmd.NewServer(b.ToolName, b.Init, b.serverHooks())
}

// Build creates one REST event fan-in command.
func (b AwaitEventBuilder) Build(_ core.Result) core.Command {
	return servercmd.NewAwait(b.ToolName, b.awaitHooks())
}

// BuildReverser creates a fresh fan-in command for receipt-driven rollback.
func (b AwaitEventBuilder) BuildReverser() core.Command {
	return servercmd.NewAwait(b.ToolName, b.awaitHooks())
}

func (b ServerBuilder) serverHooks() servercmd.ServerHooks {
	return servercmd.ServerHooks{
		ServerName:        b.Server.Name,
		ConfiguredAddress: b.Server.Server.Address,
		Launch:            b.launchHook(),
		Await:             b.awaitHook(),
		AwaitContext:      b.awaitContextHook(),
		Stop:              b.stopHook(),
		UndoLaunch:        b.undoLaunchHook(),
		RestoreEvent:      restoreEventHook(b.State),
	}
}

func (b ServerBuilder) launchHook() func() (map[string]interface{}, servercmd.Identity, error) {
	return func() (map[string]interface{}, servercmd.Identity, error) {
		output, identity, err := b.State.launchOwned(b.Server)
		if err != nil {
			return nil, servercmd.Identity{}, err
		}
		return output, servercmd.Identity{Address: identity.address, Ownership: identity.ownership}, nil
	}
}

func (b ServerBuilder) awaitHook() func() (servercmd.Event, string, error) {
	return func() (servercmd.Event, string, error) {
		event, signal, err := b.State.Await(b.Server.Name)
		return toCmdEvent(event), signal, err
	}
}

func (b ServerBuilder) awaitContextHook() func(context.Context) (servercmd.Event, string, error) {
	return func(ctx context.Context) (servercmd.Event, string, error) {
		event, signal, err := b.State.AwaitContext(ctx, b.Server.Name)
		return toCmdEvent(event), signal, err
	}
}

func (b ServerBuilder) stopHook() func() (map[string]interface{}, error) {
	return func() (map[string]interface{}, error) {
		return b.State.Stop(b.Server.Name)
	}
}

func (b ServerBuilder) undoLaunchHook() func(string, string, string) (map[string]interface{}, error) {
	return func(server, address, ownership string) (map[string]interface{}, error) {
		return b.State.UndoLaunch(server, address, ownership)
	}
}

func (b AwaitEventBuilder) awaitHooks() servercmd.AwaitHooks {
	return servercmd.AwaitHooks{
		AwaitAny: func() (servercmd.Event, string, error) {
			event, signal, err := b.State.AwaitAny(b.Options)
			return toCmdEvent(event), signal, err
		},
		AwaitAnyContext: func(ctx context.Context) (servercmd.Event, string, error) {
			event, signal, err := b.State.AwaitAnyContext(ctx, b.Options)
			return toCmdEvent(event), signal, err
		},
		RestoreEvent: restoreEventHook(b.State),
	}
}

func restoreEventHook(state *ServerState) func(string, servercmd.Event) error {
	return func(server string, event servercmd.Event) error {
		return state.RestoreEvent(server, fromCmdEvent(event))
	}
}

func toCmdEvent(event InboundEvent) servercmd.Event {
	return servercmd.Event{
		Source: event.Source, Queue: event.Queue, Route: event.Route,
		Method: event.Method, Signal: event.Signal, Payload: event.Payload,
		RequestID: event.RequestID,
	}
}

func fromCmdEvent(event servercmd.Event) InboundEvent {
	return InboundEvent{
		Source: event.Source, Queue: event.Queue, Route: event.Route,
		Method: event.Method, Signal: event.Signal, Payload: event.Payload,
		RequestID: event.RequestID,
	}
}

type serverLaunchReceipt = servercmd.LaunchReceipt

func decodeServerLaunchReceipt(value string) (serverLaunchReceipt, error) {
	return servercmd.DecodeLaunchReceipt(value)
}
