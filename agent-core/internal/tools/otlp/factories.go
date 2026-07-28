// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// StandardInits lists the receiver lifecycle init names.
var StandardInits = []string{
	InitReceiverLaunch, InitAwaitSpans, InitLoadOTLPBatch, InitSpoolSpans, InitRelaySpans, InitReceiverStop,
}

// ReceiverToolConfig is the YAML ToolDef config shared by launch and stop.
type ReceiverToolConfig struct {
	Receiver        string `json:"receiver"`
	Address         string `json:"address"`
	QueueCapacity   int    `json:"queue_capacity"`
	OverflowPolicy  string `json:"overflow_policy"`
	ShutdownTimeout string `json:"shutdown_timeout"`
	DrainPolicy     string `json:"drain_policy"`
}

// AwaitToolConfig is the declared await_spans configuration.
type AwaitToolConfig struct {
	Receiver string `json:"receiver"`
	Timeout  string `json:"timeout"`
}

// SpoolToolConfig is the declared spool_spans configuration.
type SpoolToolConfig struct {
	Path        string `json:"path"`
	BatchSource string `json:"batch_source"`
	MaxBytes    int64  `json:"max_bytes"`
	MaxFiles    int    `json:"max_files"`
}

// LoadToolConfig is the declared load_otlp_batch configuration.
type LoadToolConfig struct {
	Path string `json:"path"`
}

// RelayToolConfig is the declared relay_spans configuration.
type RelayToolConfig struct {
	Endpoint        string `json:"endpoint"`
	ReceiverAddress string `json:"receiver_address"`
	BatchSource     string `json:"batch_source"`
	Timeout         string `json:"timeout"`
}

// RegisterFactories registers receiver lifecycle factories over one shared state.
func RegisterFactories(br *toolregistry.BuiltinRegistry, state *State) {
	if state == nil {
		state = NewState()
	}
	for _, init := range StandardInits {
		switch init {
		case InitAwaitSpans:
			br.Register(init, awaitFactory(state))
		case InitLoadOTLPBatch:
			br.Register(init, loadFactory())
		case InitSpoolSpans:
			br.Register(init, spoolFactory())
		case InitRelaySpans:
			br.Register(init, relayFactory())
		default:
			br.Register(init, receiverFactory(init, state))
		}
	}
}

func loadFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw LoadToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		path := raw.Path
		if !filepath.IsAbs(path) && vars["directory"] != "" {
			path = filepath.Join(vars["directory"], path)
		}
		return LoadBuilder{ToolName: def.Name, Config: LoadConfig{Path: path}}, nil
	}
}

func relayFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var raw RelayToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		timeout := defaultRelayTimeout
		if raw.Timeout != "" {
			parsed, err := time.ParseDuration(raw.Timeout)
			if err != nil {
				return nil, fmt.Errorf("tool %q config has invalid timeout %q", def.Name, raw.Timeout)
			}
			timeout = parsed
		}
		source := raw.BatchSource
		if source == "" {
			source = defaultBatchSource
		}
		config := RelayConfig{
			Endpoint: raw.Endpoint, ReceiverAddress: raw.ReceiverAddress,
			BatchSource: source, Timeout: timeout,
		}
		if err := validateRelayConfig(def.Name, config); err != nil {
			return nil, err
		}
		return RelayBuilder{ToolName: def.Name, Config: config}, nil
	}
}

func receiverFactory(init string, state *State) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var raw ReceiverToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		cfg, err := decodeReceiverConfig(def.Name, raw)
		if err != nil {
			return nil, err
		}
		if init == InitReceiverLaunch {
			if err := validateReceiverConfig(withReceiverDefaults(cfg)); err != nil {
				return nil, fmt.Errorf("tool %q config: %w", def.Name, err)
			}
		}
		return ReceiverBuilder{ToolName: def.Name, Init: init, Config: cfg, State: state}, nil
	}
}

func awaitFactory(state *State) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var raw AwaitToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Receiver == "" {
			return nil, fmt.Errorf("tool %q config requires receiver", def.Name)
		}
		timeout := defaultBatchAwaitTimeout
		if raw.Timeout != "" {
			parsed, err := time.ParseDuration(raw.Timeout)
			if err != nil || parsed <= 0 {
				return nil, fmt.Errorf("tool %q config has invalid timeout %q", def.Name, raw.Timeout)
			}
			timeout = parsed
		}
		return AwaitBuilder{
			ToolName: def.Name, Config: AwaitConfig{Receiver: raw.Receiver, Timeout: timeout},
			State: state,
		}, nil
	}
}

func spoolFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		var raw SpoolToolConfig
		if err := catalog.DecodeToolConfig(def, &raw); err != nil {
			return nil, err
		}
		if raw.Path == "" {
			return nil, fmt.Errorf("tool %q config requires path", def.Name)
		}
		source := raw.BatchSource
		if source == "" {
			source = defaultBatchSource
		}
		if _, ok := core.ParseSelector(source); !ok {
			return nil, fmt.Errorf("tool %q config has invalid batch_source %q", def.Name, source)
		}
		if raw.MaxBytes < 0 {
			return nil, fmt.Errorf("tool %q config max_bytes must not be negative", def.Name)
		}
		if raw.MaxFiles < 0 {
			return nil, fmt.Errorf("tool %q config max_files must not be negative", def.Name)
		}
		path := raw.Path
		if !filepath.IsAbs(path) && vars["directory"] != "" {
			path = filepath.Join(vars["directory"], path)
		}
		return SpoolBuilder{
			ToolName: def.Name,
			Config: SpoolConfig{
				Path: path, BatchSource: source, MaxBytes: raw.MaxBytes, MaxFiles: raw.MaxFiles,
			},
		}, nil
	}
}

func decodeReceiverConfig(toolName string, raw ReceiverToolConfig) (ReceiverConfig, error) {
	if raw.Receiver == "" {
		return ReceiverConfig{}, fmt.Errorf("tool %q config requires receiver", toolName)
	}
	timeout := defaultShutdownTimeout
	if raw.ShutdownTimeout != "" {
		parsed, err := time.ParseDuration(raw.ShutdownTimeout)
		if err != nil {
			return ReceiverConfig{}, fmt.Errorf(
				"tool %q config has invalid shutdown_timeout %q", toolName, raw.ShutdownTimeout,
			)
		}
		timeout = parsed
	}
	return ReceiverConfig{
		Name: raw.Receiver, Address: raw.Address, QueueCapacity: raw.QueueCapacity,
		OverflowPolicy: OverflowPolicy(raw.OverflowPolicy), ShutdownTimeout: timeout,
		DrainPolicy: DrainPolicy(raw.DrainPolicy),
	}, nil
}
