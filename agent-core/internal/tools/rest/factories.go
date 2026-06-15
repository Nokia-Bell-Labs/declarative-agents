// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"time"

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
	InitAwaitEvent   = "rest_await_event"
)

// StandardInits lists every REST builtin init name.
var StandardInits = []string{
	InitClientGet, InitClientSet, InitClientCreate, InitClientDelete, InitClientInvoke,
	InitClientSend, InitClientAwait, InitServerLaunch, InitServerAwait, InitServerStop,
	InitAwaitEvent,
}

// FactoryDeps holds REST factory dependencies.
type FactoryDeps struct {
	Definitions        Collection
	ServerState        *ServerState
	AsyncState         *AsyncState
	CredentialResolver CredentialResolver
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

// AwaitEventToolConfig holds REST event fan-in ToolDef config.
type AwaitEventToolConfig struct {
	Sources         []AwaitEventSourceConfig `json:"sources"`
	AllowedSignals  []string                 `json:"allowed_signals"`
	Timeout         string                   `json:"timeout"`
	StoppedBehavior string                   `json:"stopped_behavior"`
}

// AwaitEventSourceConfig selects one REST server source.
type AwaitEventSourceConfig struct {
	Server          string   `json:"server"`
	Routes          []string `json:"routes"`
	Signals         []string `json:"signals"`
	StoppedBehavior string   `json:"stopped_behavior"`
}

// RegisterFactories registers REST builtin factories.
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	if deps.ServerState == nil {
		deps.ServerState = NewServerState()
	}
	if deps.AsyncState == nil {
		deps.AsyncState = NewAsyncState()
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
		case InitAwaitEvent:
			return newAwaitEventBuilder(def, deps)
		default:
			return newClientBuilder(def, init, deps)
		}
	}
}

func newClientBuilder(def catalog.ToolDef, init string, deps FactoryDeps) (core.Builder, error) {
	var cfg ClientToolConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	if err := validateClientToolConfig(def.Name, cfg); err != nil {
		return nil, err
	}
	operation, err := deps.Definitions.ResolveClientOperation(cfg)
	if err != nil {
		return nil, err
	}
	if init == InitClientSend && operation.Operation.Async == nil {
		return nil, fmt.Errorf("tool %q requires async REST operation", def.Name)
	}
	return ClientBuilder{
		ToolName: def.Name, Init: init, Operation: operation,
		AsyncState: deps.AsyncState, Credentials: deps.CredentialResolver,
	}, nil
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

func newAwaitEventBuilder(def catalog.ToolDef, deps FactoryDeps) (core.Builder, error) {
	var cfg AwaitEventToolConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return nil, err
	}
	options, err := awaitAnyOptions(def.Name, cfg, deps.Definitions)
	if err != nil {
		return nil, err
	}
	return AwaitEventBuilder{ToolName: def.Name, Options: options, State: deps.ServerState}, nil
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

func awaitAnyOptions(toolName string, cfg AwaitEventToolConfig, defs Collection) (AwaitAnyOptions, error) {
	if len(cfg.Sources) == 0 {
		return AwaitAnyOptions{}, fmt.Errorf("tool %q config requires sources", toolName)
	}
	timeout, err := awaitTimeout(toolName, cfg.Timeout)
	if err != nil {
		return AwaitAnyOptions{}, err
	}
	stopped, err := stoppedSourceBehavior(toolName, cfg.StoppedBehavior)
	if err != nil {
		return AwaitAnyOptions{}, err
	}
	options := AwaitAnyOptions{Timeout: timeout, StoppedBehavior: stopped}
	for _, source := range cfg.Sources {
		awaitSource, err := awaitSourceConfig(toolName, source, cfg.AllowedSignals, defs)
		if err != nil {
			return AwaitAnyOptions{}, err
		}
		options.Sources = append(options.Sources, awaitSource)
	}
	return options, nil
}

func awaitSourceConfig(
	toolName string,
	cfg AwaitEventSourceConfig,
	allowedSignals []string,
	defs Collection,
) (AwaitSource, error) {
	if cfg.Server == "" {
		return AwaitSource{}, fmt.Errorf("tool %q source requires server", toolName)
	}
	if _, err := defs.ResolveServer(cfg.Server); err != nil {
		return AwaitSource{}, err
	}
	signals, err := signalFilters(toolName, cfg.Signals, allowedSignals)
	if err != nil {
		return AwaitSource{}, err
	}
	stopped, err := stoppedSourceBehavior(toolName, cfg.StoppedBehavior)
	if err != nil {
		return AwaitSource{}, err
	}
	return AwaitSource{
		Server: cfg.Server, Routes: cfg.Routes,
		Signals:         signals,
		StoppedBehavior: stopped,
	}, nil
}

func awaitTimeout(toolName, value string) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("tool %q config has invalid timeout %q", toolName, value)
	}
	return timeout, nil
}

func signalFilters(toolName string, source, allowed []string) ([]string, error) {
	if len(source) == 0 || len(allowed) == 0 {
		return mergeSignals(source, allowed), nil
	}
	signals := intersectSignals(source, allowed)
	if len(signals) == 0 {
		return nil, fmt.Errorf("tool %q source signals do not match allowed_signals", toolName)
	}
	return signals, nil
}

func mergeSignals(source, allowed []string) []string {
	if len(source) > 0 {
		return source
	}
	return allowed
}

func intersectSignals(source, allowed []string) []string {
	seen := map[string]bool{}
	for _, signal := range allowed {
		seen[signal] = true
	}
	var signals []string
	for _, signal := range source {
		if seen[signal] {
			signals = append(signals, signal)
		}
	}
	return signals
}

func stoppedSourceBehavior(toolName, value string) (StoppedSourceBehavior, error) {
	switch value {
	case "":
		return "", nil
	case string(StoppedSourceIgnore):
		return StoppedSourceIgnore, nil
	case string(StoppedSourceCommandError):
		return StoppedSourceCommandError, nil
	case string(StoppedSourceEmitServerStopped):
		return StoppedSourceEmitServerStopped, nil
	default:
		return "", fmt.Errorf("tool %q config has unsupported stopped_behavior %q", toolName, value)
	}
}
