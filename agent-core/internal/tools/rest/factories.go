// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	toolregistry "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/registry"
)

const (
	InitClientGet    = "rest_client_get"
	InitClientSet    = "rest_client_set"
	InitClientCreate = "rest_client_create"
	InitClientDelete = "rest_client_delete"
	InitClientInvoke = "rest_client_invoke"
	InitClientSend   = "rest_client_send"
	InitClientAwait  = "rest_client_await"
	InitServerLaunch = "rest_server_launch"
	InitServerAwait  = "rest_server_await"
	InitServerStop   = "rest_server_stop"
)

// StandardInits lists every REST builtin init name.
var StandardInits = []string{
	InitClientGet, InitClientSet, InitClientCreate, InitClientDelete, InitClientInvoke,
	InitClientSend, InitClientAwait, InitServerLaunch, InitServerAwait, InitServerStop,
}

// FactoryDeps holds REST factory dependencies.
type FactoryDeps struct {
	Definitions Collection
	ServerState *ServerState
}

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

// RegisterFactories registers REST builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	if deps.ServerState == nil {
		deps.ServerState = NewServerState()
	}
	for _, init := range StandardInits {
		br.Register(init, factoryFor(init, deps))
	}
}

func factoryFor(init string, deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		switch init {
		case InitServerLaunch, InitServerAwait, InitServerStop:
			return newServerBuilder(def, init, deps)
		default:
			return newClientBuilder(def, init, deps.Definitions)
		}
	}
}

func newClientBuilder(def catalog.ToolDef, init string, definitions Collection) (core.Builder, error) {
	var cfg ClientToolConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if err := validateClientToolConfig(def.Name, cfg); err != nil {
		return nil, err
	}
	operation, err := definitions.ResolveClientOperation(cfg)
	if err != nil {
		return nil, err
	}
	if init == InitClientSend && operation.Operation.Async == nil {
		return nil, fmt.Errorf("tool %q requires async REST operation", def.Name)
	}
	return ClientBuilder{ToolName: def.Name, Init: init, Operation: operation}, nil
}

func newServerBuilder(def catalog.ToolDef, init string, deps FactoryDeps) (core.Builder, error) {
	var cfg ServerToolConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if cfg.RestRef == "" {
		return nil, fmt.Errorf("tool %q config requires rest_ref", def.Name)
	}
	server, err := deps.Definitions.ResolveServer(cfg.RestRef)
	if err != nil {
		return nil, err
	}
	return ServerBuilder{ToolName: def.Name, Init: init, Server: server, State: deps.ServerState}, nil
}

func validateClientToolConfig(toolName string, cfg ClientToolConfig) error {
	if cfg.RestRef == "" {
		return fmt.Errorf("tool %q config requires rest_ref", toolName)
	}
	if cfg.Operation == "" {
		return fmt.Errorf("tool %q config requires operation", toolName)
	}
	return nil
}
