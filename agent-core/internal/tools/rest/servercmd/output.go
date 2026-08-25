// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package servercmd

import (
	"encoding/json"
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func jsonOutput(value map[string]interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}

func commandError(commandName string, err error) core.Result {
	return core.Result{Signal: core.CommandError, CommandName: commandName, Output: err.Error(), Err: err}
}

func eventOutput(event Event) string {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}

func stringValue(value interface{}) string {
	text, _ := value.(string)
	return text
}

func awaitCommandResult(commandName string, event Event, signal string) core.Result {
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
