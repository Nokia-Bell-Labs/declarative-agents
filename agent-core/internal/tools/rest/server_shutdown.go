// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"encoding/json"
	"fmt"
)

func shutdownUnblockSignal(config ShutdownConfig) string {
	if config.UnblockAwaitSignal != "" {
		return config.UnblockAwaitSignal
	}
	return "ServerStopped"
}

func jsonOutput(value map[string]interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}
