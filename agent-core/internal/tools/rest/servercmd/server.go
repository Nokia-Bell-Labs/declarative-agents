// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package servercmd

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

// Server is a REST server launch, await, or stop command.
type Server struct {
	toolName string
	init     string
	hooks    ServerHooks
}

// NewServer creates one REST server boundary command.
func NewServer(toolName, init string, hooks ServerHooks) core.Command {
	return Server{toolName: toolName, init: init, hooks: hooks}
}

func (c Server) Name() string { return c.toolName }

func (c Server) Execute() core.Result {
	switch c.init {
	case initLaunch:
		return c.launch()
	case initAwait:
		return c.await()
	case initStop:
		return c.stop()
	default:
		return commandError(c.toolName, fmt.Errorf("unsupported REST server init %q", c.init))
	}
}

func (c Server) ExecuteContext(ctx context.Context) core.Result {
	if c.init != initAwait {
		return c.Execute()
	}
	event, signal, err := c.hooks.AwaitContext(ctx)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return awaitCommandResult(c.toolName, event, signal)
}

func (c Server) Undo(prior core.Result) core.Result {
	switch c.init {
	case initAwait:
		return restoreAwaitReceipt(c.toolName, c.hooks.RestoreEvent, prior.Receipt)
	case initStop:
		return c.undoStop(prior.Receipt)
	case initLaunch:
		return c.undoLaunch(prior)
	default:
		return core.NoopUndo(c.toolName)
	}
}

func (c Server) launch() core.Result {
	output, identity, err := c.hooks.Launch()
	if err != nil {
		return commandError(c.toolName, err)
	}
	receipt, err := json.Marshal(LaunchReceipt{
		Strategy: LaunchReceiptStrategy, Declaration: c.toolName,
		Server: c.hooks.ServerName, Address: identity.Address, Ownership: identity.Ownership,
	})
	if err != nil {
		return commandError(c.toolName, fmt.Errorf("encode REST server launch receipt: %w", err))
	}
	return core.Result{
		Signal: core.Signal("ServerLaunched"), CommandName: c.toolName,
		Output: jsonOutput(output), Receipt: string(receipt),
	}
}

func (c Server) await() core.Result {
	event, signal, err := c.hooks.Await()
	if err != nil {
		return commandError(c.toolName, err)
	}
	return awaitCommandResult(c.toolName, event, signal)
}

func (c Server) stop() core.Result {
	output, err := c.hooks.Stop()
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
				"rest_ref":    c.hooks.ServerName, "compensation": output,
			},
		},
	})
	return core.Result{
		Signal: core.Signal("ServerStopped"), CommandName: c.toolName,
		Output: jsonOutput(output), Receipt: receipt,
	}
}

func (c Server) undoLaunch(prior core.Result) core.Result {
	receipt, err := c.decodeLaunchReceipt(prior)
	if err != nil {
		return commandError(c.toolName, err)
	}
	output, err := c.hooks.UndoLaunch(receipt.Server, receipt.Address, receipt.Ownership)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{Signal: core.Signal("ServerStopped"), CommandName: c.toolName, Output: jsonOutput(output)}
}

func (c Server) undoStop(receipt string) core.Result {
	compensation, ok, err := undo.DecodeBoundaryReceipt(receipt)
	if err != nil {
		return commandError(c.toolName, err)
	}
	if !ok || compensation.Strategy != "server_shutdown_or_user_action_compensation" {
		return commandError(c.toolName, fmt.Errorf("REST stop receipt has no server relaunch compensation"))
	}
	restRef := stringValue(compensation.Data["rest_ref"])
	if restRef != c.hooks.ServerName {
		return commandError(c.toolName, fmt.Errorf(
			"REST stop receipt server %q does not match configured server %q",
			restRef, c.hooks.ServerName,
		))
	}
	compensationData, _ := compensation.Data["compensation"].(map[string]interface{})
	return undo.BoundaryCompensationUndo(c.toolName, fmt.Sprintf(
		"MachineSpec must relaunch server %q at %q; stop drained %v queued events",
		restRef, stringValue(compensation.Data["server_addr"]), compensationData["drained_events"],
	))
}

func (c Server) decodeLaunchReceipt(prior core.Result) (LaunchReceipt, error) {
	if prior.CommandName != "" && prior.CommandName != c.toolName {
		return LaunchReceipt{}, fmt.Errorf(
			"REST server launch receipt command %q does not match declaration %q",
			prior.CommandName, c.toolName,
		)
	}
	receipt, err := DecodeLaunchReceipt(prior.Receipt)
	if err != nil {
		return LaunchReceipt{}, fmt.Errorf("decode REST server launch receipt: %w", err)
	}
	if err := c.matchLaunchReceipt(receipt); err != nil {
		return LaunchReceipt{}, err
	}
	return receipt, nil
}

func (c Server) matchLaunchReceipt(receipt LaunchReceipt) error {
	if receipt.Strategy != LaunchReceiptStrategy {
		return fmt.Errorf(
			"REST server launch receipt strategy %q does not match %q",
			receipt.Strategy, LaunchReceiptStrategy,
		)
	}
	if receipt.Declaration != c.toolName {
		return fmt.Errorf(
			"REST server launch receipt declaration %q does not match %q",
			receipt.Declaration, c.toolName,
		)
	}
	if receipt.Server != c.hooks.ServerName {
		return fmt.Errorf(
			"REST server launch receipt server %q does not match configured server %q",
			receipt.Server, c.hooks.ServerName,
		)
	}
	if err := validateLaunchAddress(c.hooks.ConfiguredAddress, receipt.Address); err != nil {
		return fmt.Errorf("REST server launch receipt address: %w", err)
	}
	return validLaunchOwnership(receipt.Ownership)
}

func validLaunchOwnership(ownership string) error {
	decoded, err := hex.DecodeString(ownership)
	if err != nil || len(decoded) != OwnershipBytes || hex.EncodeToString(decoded) != ownership {
		return fmt.Errorf("REST server launch receipt ownership is invalid")
	}
	return nil
}
