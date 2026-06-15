// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"fmt"

	"go.opentelemetry.io/otel/attribute"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/undo"
)

// ExitConfig configures the generic exit_agent builtin.
type ExitConfig struct {
	Reason      string `json:"reason"`
	Status      string `json:"status"`
	DrainPolicy string `json:"drain_policy"`
}

// ExitBuilder constructs exit_agent commands.
type ExitBuilder struct {
	Config   ExitConfig
	Shutdown func()
	Tracer   tracing.Tracer
}

func (b ExitBuilder) Build(_ core.Result) core.Command {
	return &exitCmd{config: b.Config, shutdown: b.Shutdown, tracer: b.Tracer}
}

type exitCmd struct {
	config   ExitConfig
	shutdown func()
	tracer   tracing.Tracer
}

func (c *exitCmd) Name() string { return "exit_agent" }

func (c *exitCmd) Execute() core.Result {
	if c.shutdown == nil {
		return commandError(c.Name(), fmt.Errorf("exit_agent requires shutdown dependency"))
	}
	output := c.output()
	if c.tracer != nil {
		c.tracer.Event("lifecycle.exit_requested",
			attribute.String("reason", c.config.Reason),
			attribute.String("status", c.status()),
			attribute.String("drain_policy", c.config.DrainPolicy),
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
	return fmt.Sprintf("status=%s reason=%s drain_policy=%s", c.status(), c.reason(), c.config.DrainPolicy)
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
