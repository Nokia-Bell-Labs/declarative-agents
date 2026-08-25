// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

type restCompensationCmd struct {
	toolName string
	executor CompensationExecutor
}

func (c restCompensationCmd) Name() string { return c.toolName }

var _ core.ContextUndoCommand = restCompensationCmd{}

func (c restCompensationCmd) Execute() core.Result {
	return restCompensationError(c.toolName, "compensation_execute", fmt.Errorf("REST compensation commands are undo-only"))
}

func (c restCompensationCmd) Undo(prior core.Result) core.Result {
	return c.UndoContext(context.Background(), prior)
}

func (c restCompensationCmd) UndoContext(ctx context.Context, prior core.Result) core.Result {
	commandName := prior.CommandName
	if commandName == "" {
		commandName = c.toolName
	}
	return c.executor.CompensateFromReceipt(ctx, commandName, prior.Receipt)
}

// CompensateFromReceipt executes the REST compensation described by an opaque
// receipt captured in Result.Receipt during Execute. This is the receipt-driven
// entry point used by the reverse receipt walk (srd035-checkpoint-port R3; #44 R3).
func (e CompensationExecutor) CompensateFromReceipt(ctx context.Context, commandName, receipt string) core.Result {
	compensation, ok, err := undo.DecodeBoundaryReceipt(receipt)
	if err != nil {
		return restCompensationError(commandName, "compensation_decode", err)
	}
	if !ok {
		return core.NoopUndo(commandName)
	}
	return e.runCompensation(ctx, commandName, compensation)
}

func (e CompensationExecutor) runCompensation(
	ctx context.Context,
	commandName string,
	compensation undo.BoundaryCompensation,
) core.Result {
	operation, err := e.resolveCompensationOperation(compensation)
	if err != nil {
		return restCompensationError(commandName, "compensation_lookup", err)
	}
	cmd := ClientBuilder{
		ToolName:    compensationToolName(commandName),
		Init:        initInvoke,
		Operation:   operation,
		Credentials: e.Credentials,
	}.Build(core.Result{Output: jsonOutput(compensationRuntimeParams(compensation, operation.Operation.Params))})
	contextual, ok := cmd.(core.ContextCommand)
	if !ok {
		return restCompensationError(commandName, "compensation_execute", errors.New("REST compensation command is not context-aware"))
	}
	result := contextual.ExecuteContext(ctx)
	if result.Signal == core.CommandError {
		return result
	}
	result.CommandName = commandName
	return result
}

func (e CompensationExecutor) resolveCompensationOperation(
	compensation undo.BoundaryCompensation,
) (ClientOperationDefinition, error) {
	if e.Definitions == nil {
		return ClientOperationDefinition{}, fmt.Errorf("REST compensation definitions are not configured")
	}
	configured := compensationMap(compensation.Data, "compensation")
	operationName, ok := configured["operation"].(string)
	if !ok || operationName == "" {
		return ClientOperationDefinition{}, fmt.Errorf("REST compensation operation is not configured")
	}
	return e.Definitions.ResolveClientOperation(ClientToolConfig{
		RestRef:  stringMapValue(compensation.Data, "rest_ref"),
		Resource: stringMapValue(compensation.Data, "resource"), Operation: operationName,
	})
}

func compensationToolName(commandName string) string {
	if commandName == "" {
		return "rest_compensation"
	}
	return commandName + "_compensation"
}

func compensationRuntimeParams(compensation undo.BoundaryCompensation, binding RequestBinding) map[string]interface{} {
	params := map[string]interface{}{}
	declared := declaredParamNames(binding)
	copyCompensationParams(params, compensation.Data["parameters"])
	configured := compensationMap(compensation.Data, "compensation")
	copyCompensationParams(params, configured["parameters"])
	resourceID := stringMapValue(compensation.Data, "resource_id")
	setCompensationParam(params, declared, "resource_id", resourceID)
	setCompensationParam(params, declared, "id", resourceID)
	setCompensationParam(params, declared, "number", resourceID)
	copyCompensationParam(params, declared, "request_id", stringMapValue(compensation.Data, "request_id"))
	copyCompensationParam(params, declared, "idempotency_token", stringMapValue(compensation.Data, "idempotency_token"))
	dropUndeclaredCompensationParams(params, declared)
	return map[string]interface{}{"parameters": params}
}

func compensationMap(values map[string]interface{}, key string) map[string]interface{} {
	mapped, _ := values[key].(map[string]interface{})
	return mapped
}

func stringMapValue(values map[string]interface{}, key string) string {
	value, _ := values[key].(string)
	return value
}

func cloneRESTParams(params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}
	clone := make(map[string]interface{}, len(params))
	for name, value := range params {
		clone[name] = value
	}
	return clone
}

func copyCompensationParams(params map[string]interface{}, value interface{}) {
	configured, ok := value.(map[string]interface{})
	if !ok {
		return
	}
	for name, param := range configured {
		params[name] = param
	}
}

func copyCompensationParam(params map[string]interface{}, declared map[string]bool, name, value string) {
	if value == "" || !declared[name] {
		return
	}
	if _, exists := params[name]; !exists {
		params[name] = value
	}
}

func setCompensationParam(params map[string]interface{}, declared map[string]bool, name, value string) {
	if value == "" || !declared[name] {
		return
	}
	params[name] = value
}

func dropUndeclaredCompensationParams(params map[string]interface{}, declared map[string]bool) {
	for name := range params {
		if !declared[name] {
			delete(params, name)
		}
	}
}

func restCompensationError(commandName, stage string, err error) core.Result {
	output := map[string]interface{}{
		"failure_stage": stage,
		"message":       err.Error(),
		"signal":        string(core.CommandError),
	}
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: jsonOutput(output), Err: err}
}
