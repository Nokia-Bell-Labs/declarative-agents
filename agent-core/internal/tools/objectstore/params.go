// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package objectstore gives agents object read, write, and list against
// cloud object storage through one family whose compiled-in drivers are
// selected by the declared bucket URL scheme (srd059-objectstore-tools).
// Provider switching is configuration: the bucket URL flows through the
// declaration templating pipeline, and the local rig binds through an
// explicit endpoint field, never an environment variable.
package objectstore

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

// ConnectionConfig is one named connection: where the objects live and, for
// the local rig, the explicit endpoint that reaches the emulator. Cloud
// credentials are the platform's ambient identity; no credential field
// exists (srd059 R1.2, R2.2).
type ConnectionConfig struct {
	BucketURL string `yaml:"bucket_url" json:"bucket_url"`
	Endpoint  string `yaml:"endpoint" json:"endpoint"`
}

// Config is a word's declared config block: the connection it uses and the
// named connections it may choose from.
type Config struct {
	Connection  string                      `yaml:"connection" json:"connection"`
	Connections map[string]ConnectionConfig `yaml:"connections" json:"connections"`
	// The destructive delete-prefix word owns these as declaration-only
	// authority. Read/write/list leave them empty.
	Prefix      string `yaml:"prefix" json:"prefix"`
	MaxObjects  int    `yaml:"max_objects" json:"max_objects"`
	Application string `yaml:"application" json:"application"`
	AuditPath   string `yaml:"audit_path" json:"audit_path"`
}

// DecodeConfig decodes and resolves a word's connection at configuration
// time, so a missing connection or bucket URL is a load failure with the
// fault named rather than a surprise at first use (srd059 R2.1).
func DecodeConfig(def catalog.ToolDef) (ConnectionConfig, error) {
	var cfg Config
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return ConnectionConfig{}, err
	}
	return resolveConnectionConfig(def.Name, cfg)
}

func resolveConnectionConfig(toolName string, cfg Config) (ConnectionConfig, error) {
	if cfg.Connection == "" {
		return ConnectionConfig{}, fmt.Errorf("tool %q config: connection is required", toolName)
	}
	connection, ok := cfg.Connections[cfg.Connection]
	if !ok {
		return ConnectionConfig{}, fmt.Errorf(
			"tool %q config: connection %q is not declared under connections", toolName, cfg.Connection)
	}
	if strings.TrimSpace(connection.BucketURL) == "" {
		return ConnectionConfig{}, fmt.Errorf(
			"tool %q config: connection %q declares no bucket_url", toolName, cfg.Connection)
	}
	return connection, nil
}

type DeletePrefixConfig struct {
	Connection  ConnectionConfig
	Prefix      string
	MaxObjects  int
	Application string
	AuditPath   string
}

func DecodeDeletePrefixConfig(def catalog.ToolDef) (DeletePrefixConfig, error) {
	var raw Config
	if err := catalog.DecodeToolConfig(def, &raw); err != nil {
		return DeletePrefixConfig{}, err
	}
	connection, err := resolveConnectionConfig(def.Name, raw)
	if err != nil {
		return DeletePrefixConfig{}, err
	}
	prefix := strings.Trim(strings.TrimSpace(raw.Prefix), "/")
	if prefix == "" || strings.Contains(prefix, "..") {
		return DeletePrefixConfig{}, fmt.Errorf("tool %q config: prefix must be a safe non-empty relative prefix", def.Name)
	}
	if raw.MaxObjects <= 0 {
		return DeletePrefixConfig{}, fmt.Errorf("tool %q config: max_objects must be positive", def.Name)
	}
	if strings.TrimSpace(raw.Application) == "" {
		return DeletePrefixConfig{}, fmt.Errorf("tool %q config: application is required", def.Name)
	}
	if strings.TrimSpace(raw.AuditPath) == "" {
		return DeletePrefixConfig{}, fmt.Errorf("tool %q config: audit_path is required", def.Name)
	}
	return DeletePrefixConfig{
		Connection: connection, Prefix: prefix, MaxObjects: raw.MaxObjects,
		Application: raw.Application, AuditPath: raw.AuditPath,
	}, nil
}

// extractStringParam mirrors the filesystem family's parameter envelope: the
// prior result's Output carries {"parameters": {...}}.
func extractStringParam(jsonOutput, name string) (string, bool) {
	var params struct {
		Parameters map[string]interface{} `json:"parameters"`
	}
	if err := json.Unmarshal([]byte(jsonOutput), &params); err != nil {
		return "", false
	}
	value, ok := params.Parameters[name].(string)
	return value, ok
}

func missingParam(command, name string) core.Command {
	return failedCommand{name: command, output: fmt.Sprintf("%s requires parameter %q", command, name)}
}

// failedCommand carries a configuration-time refusal into the dispatch as a
// ToolFailed result, the way the filesystem family reports a missing
// parameter.
type failedCommand struct {
	name   string
	output string
}

func (f failedCommand) Name() string { return f.name }
func (f failedCommand) Execute() core.Result {
	return core.Result{Output: f.output, Signal: core.ToolFailed, CommandName: f.name}
}
func (f failedCommand) Undo(core.Result) core.Result { return core.NoopUndo(f.name) }

func commandError(name string, err error) core.Result {
	return core.Result{Signal: core.CommandError, Err: err, CommandName: name, Output: err.Error()}
}
