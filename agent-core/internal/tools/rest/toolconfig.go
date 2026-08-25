// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

// ClientToolConfig holds REST client ToolDef config.
type ClientToolConfig struct {
	RestRef   string `json:"rest_ref"`
	Resource  string `json:"resource"`
	Operation string `json:"operation"`
}

// ServerToolConfig holds REST server ToolDef config.
type ServerToolConfig struct {
	RestRef string `json:"rest_ref"`
}

// AwaitEventToolConfig holds REST event fan-in ToolDef config.
type AwaitEventToolConfig struct {
	Sources         []AwaitEventSourceConfig `json:"sources"`
	AllowedSignals  []string                 `json:"allowed_signals"`
	Timeout         string                   `json:"timeout"`
	ReadPolicy      string                   `json:"read_policy"`
	StoppedBehavior string                   `json:"stopped_behavior"`
}

// AwaitEventSourceConfig selects one REST server source.
type AwaitEventSourceConfig struct {
	Server          string   `json:"server"`
	Routes          []string `json:"routes"`
	Signals         []string `json:"signals"`
	StoppedBehavior string   `json:"stopped_behavior"`
}
