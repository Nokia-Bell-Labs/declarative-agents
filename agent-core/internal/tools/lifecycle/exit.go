// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"encoding/json"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

// ExitConfig configures the generic exit_agent builtin.
type ExitConfig struct {
	Reason       string `json:"reason"`
	Status       string `json:"status"`
	DrainPolicy  string `json:"drain_policy"`
	DrainResult  string `json:"drain_result"`
	CheckpointID string `json:"checkpoint_id"`
}

// ExitBuilder constructs exit_agent commands.
type ExitBuilder struct {
	Config   ExitConfig
	Shutdown func()
	Tracer   tracing.Tracer
}

type exitRuntimeField struct {
	key   string
	apply func(*ExitConfig, string)
}

func (b ExitBuilder) Build(previous core.Result) core.Command {
	cfg, err := configWithRuntimePayload(b.Config, previous.Output)
	return &exitCmd{config: cfg, buildErr: err, shutdown: b.Shutdown, tracer: b.Tracer}
}

type exitCmd struct {
	config   ExitConfig
	buildErr error
	shutdown func()
	tracer   tracing.Tracer
}

func (c *exitCmd) Name() string { return "exit_agent" }

func (c *exitCmd) Execute() core.Result {
	if c.buildErr != nil {
		return commandError(c.Name(), c.buildErr)
	}
	if c.shutdown == nil {
		return commandError(c.Name(), fmt.Errorf("exit_agent requires shutdown dependency"))
	}
	output := c.output()
	if c.tracer != nil {
		c.tracer.Event("lifecycle.exit_requested",
			attribute.String("reason", c.reason()),
			attribute.String("status", c.status()),
			attribute.String("drain_policy", c.drainPolicy()),
		)
	}
	c.shutdown()
	return core.Result{Signal: core.Signal("AgentExited"), CommandName: c.Name(), Output: output}
}

func (c *exitCmd) Undo() core.Result {
	return undo.BoundaryCompensationUndo(c.Name(), "operator can restart the agent or resume from a checkpoint")
}

func (c *exitCmd) UndoMemento() (core.UndoMemento, error) {
	payload := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy: "operator_restart_or_checkpoint_resume",
		Reason:   c.config.Reason,
		Requires: []string{"operator_decision", "profile", "checkpoint_id"},
	}}
	return undo.BoundaryCompensationMemento(c.Name(), payload, "operator can restart the agent or resume from a checkpoint")
}

func (c *exitCmd) output() string {
	output := map[string]string{
		"status":       c.status(),
		"reason":       c.reason(),
		"drain_policy": c.drainPolicy(),
		"signal":       "AgentExited",
	}
	addIfSet(output, "drain_result", c.config.DrainResult)
	addIfSet(output, "checkpoint_id", c.config.CheckpointID)
	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}

func (c *exitCmd) status() string {
	if c.config.Status == "" {
		return "success"
	}
	return c.config.Status
}

func (c *exitCmd) reason() string {
	if c.config.Reason == "" {
		return "operator requested shutdown"
	}
	return c.config.Reason
}

func (c *exitCmd) drainPolicy() string {
	return c.config.DrainPolicy
}

func configWithRuntimePayload(cfg ExitConfig, previousOutput string) (ExitConfig, error) {
	if strings.TrimSpace(previousOutput) == "" {
		return cfg, nil
	}
	payload, err := runtimePayload(previousOutput)
	if err != nil {
		return ExitConfig{}, err
	}
	return applyRuntimePayload(cfg, payload)
}

func runtimePayload(previousOutput string) (map[string]interface{}, error) {
	var event struct {
		Payload map[string]interface{} `json:"payload"`
	}
	if err := json.Unmarshal([]byte(previousOutput), &event); err != nil {
		return nil, fmt.Errorf("decode exit_agent previous result: %w", err)
	}
	return event.Payload, nil
}

func applyRuntimePayload(cfg ExitConfig, payload map[string]interface{}) (ExitConfig, error) {
	for _, field := range exitRuntimeFields() {
		value, ok, err := stringPayloadField(payload, field.key)
		if err != nil {
			return ExitConfig{}, err
		}
		if ok {
			field.apply(&cfg, value)
		}
	}
	return cfg, nil
}

func stringPayloadField(payload map[string]interface{}, key string) (string, bool, error) {
	value, ok := payload[key]
	if !ok {
		return "", false, nil
	}
	text, ok := value.(string)
	if !ok {
		return "", false, fmt.Errorf("exit_agent payload field %q must be a string", key)
	}
	return text, true, nil
}

func addIfSet(output map[string]string, key, value string) {
	if value != "" {
		output[key] = value
	}
}

func exitRuntimeFields() []exitRuntimeField {
	return []exitRuntimeField{
		{key: "reason", apply: func(c *ExitConfig, v string) { c.Reason = v }},
		{key: "status", apply: func(c *ExitConfig, v string) { c.Status = v }},
		{key: "drain_policy", apply: func(c *ExitConfig, v string) { c.DrainPolicy = v }},
		{key: "drain_result", apply: func(c *ExitConfig, v string) { c.DrainResult = v }},
		{key: "checkpoint_id", apply: func(c *ExitConfig, v string) { c.CheckpointID = v }},
	}
}
