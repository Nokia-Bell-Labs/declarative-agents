// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"fmt"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// StandardInits lists the receiver lifecycle init names.
var StandardInits = []string{InitReceiverLaunch, InitReceiverStop}

// ReceiverToolConfig is the YAML ToolDef config shared by launch and stop.
type ReceiverToolConfig struct {
	Receiver        string `json:"receiver"`
	Address         string `json:"address"`
	QueueCapacity   int    `json:"queue_capacity"`
	OverflowPolicy  string `json:"overflow_policy"`
	ShutdownTimeout string `json:"shutdown_timeout"`
	DrainPolicy     string `json:"drain_policy"`
}

// RegisterFactories registers receiver lifecycle factories over one shared state.
func RegisterFactories(br *toolregistry.BuiltinRegistry, state *State) {
	if state == nil {
		state = NewState()
	}
	for _, init := range StandardInits {
		br.Register(init, receiverFactory(init, state))
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
