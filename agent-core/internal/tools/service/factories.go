// Copyright (c) 2026 Nokia. All rights reserved.

package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	InitStartService  = "start_service"
	InitAwaitHealthy  = "await_healthy"
	InitStopService   = "stop_service"
	InitRunValidators = "run_validators"
	InitListScenarios = "list_scenarios"
)

// StandardInits lists every service builtin init name.
var StandardInits = []string{
	InitStartService, InitAwaitHealthy, InitStopService, InitRunValidators, InitListScenarios,
}

// Result signals. Healthy and HealthTimeout are distinct so a machine can
// route teardown on a failed wait (srd040 R2.3); ValidatorsCompleted and
// ValidatorsIncomplete separate "all ran" from "one timed out or failed to
// spawn" (R4.5).
const (
	SignalServiceStarted       core.Signal = "ServiceStarted"
	SignalServiceStopped       core.Signal = "ServiceStopped"
	SignalHealthy              core.Signal = "Healthy"
	SignalHealthTimeout        core.Signal = "HealthTimeout"
	SignalValidatorsCompleted  core.Signal = "ValidatorsCompleted"
	SignalValidatorsIncomplete core.Signal = "ValidatorsIncomplete"
	SignalScenariosListed      core.Signal = "ScenariosListed"
)

// ToolConfig is the declared config for every service word. Each word reads
// the fields it needs; unrelated fields stay empty.
type ToolConfig struct {
	Service   string   `yaml:"service,omitempty"`
	Binary    string   `yaml:"binary,omitempty"`
	Profile   string   `yaml:"profile,omitempty"`
	Directory string   `yaml:"directory,omitempty"`
	Request   string   `yaml:"request,omitempty"`
	Address   string   `yaml:"address,omitempty"`
	Env       []string `yaml:"env,omitempty"`

	URL      string `yaml:"url,omitempty"`
	Timeout  string `yaml:"timeout,omitempty"`
	Interval string `yaml:"interval,omitempty"`
	Grace    string `yaml:"grace,omitempty"`

	Validators []ValidatorSpec `yaml:"validators,omitempty"`
	Roots      []string        `yaml:"roots,omitempty"`
}

// FactoryDeps holds service factory dependencies.
type FactoryDeps struct {
	State *State
}

// RegisterBuiltins registers every service builtin factory.
func RegisterBuiltins(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	if deps.State == nil {
		deps.State = NewState()
	}
	for _, init := range StandardInits {
		br.Register(init, factoryFor(init, deps))
	}
}

func factoryFor(init string, deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg ToolConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := validateToolConfig(def.Name, init, cfg); err != nil {
			return nil, err
		}
		return Builder{ToolName: def.Name, Init: init, Config: cfg, State: deps.State}, nil
	}
}

func validateToolConfig(name, init string, cfg ToolConfig) error {
	switch init {
	case InitStartService:
		if cfg.Profile == "" {
			return fmt.Errorf("tool %q (%s) requires a profile", name, init)
		}
		if cfg.Service == "" {
			return fmt.Errorf("tool %q (%s) requires a service name", name, init)
		}
	case InitAwaitHealthy:
		if cfg.URL == "" {
			return fmt.Errorf("tool %q (%s) requires a url", name, init)
		}
	case InitStopService:
		if cfg.Service == "" {
			return fmt.Errorf("tool %q (%s) requires a service name", name, init)
		}
	case InitRunValidators:
		if len(cfg.Validators) == 0 {
			return fmt.Errorf("tool %q (%s) requires at least one validator", name, init)
		}
		for i, validator := range cfg.Validators {
			if validator.Profile == "" {
				return fmt.Errorf("tool %q (%s) validator %d requires a profile", name, init, i)
			}
		}
	case InitListScenarios:
		if len(cfg.Roots) == 0 {
			return fmt.Errorf("tool %q (%s) requires at least one root", name, init)
		}
	}
	return nil
}

// Builder constructs one service boundary command.
type Builder struct {
	ToolName string
	Init     string
	Config   ToolConfig
	State    *State
}

// Build creates one service command.
func (b Builder) Build(_ core.Result) core.Command {
	return command{toolName: b.ToolName, init: b.Init, cfg: b.Config, state: b.State}
}

type command struct {
	toolName string
	init     string
	cfg      ToolConfig
	state    *State
}

func (c command) Name() string { return c.toolName }

func (c command) Execute() core.Result { return c.ExecuteContext(context.Background()) }

func (c command) ExecuteContext(ctx context.Context) core.Result {
	switch c.init {
	case InitStartService:
		return c.start()
	case InitAwaitHealthy:
		return c.awaitHealthy()
	case InitStopService:
		return c.stop()
	case InitRunValidators:
		return c.runValidators(ctx)
	case InitListScenarios:
		return c.listScenarios()
	default:
		return commandError(c.toolName, fmt.Errorf("unsupported service init %q", c.init))
	}
}

// Undo reverses start_service by stopping the service it started; every other
// word is read-only or already terminal, so its undo is a noop (srd040 R1.5,
// R3.3). The declarations must match this, or the corpus audit reports a
// tool-undo mismatch.
func (c command) Undo(_ core.Result) core.Result {
	if c.init != InitStartService {
		return core.NoopUndo(c.toolName)
	}
	output := c.state.Stop(c.cfg.Service, parseDuration(c.cfg.Grace, defaultStopGrace))
	return core.Result{
		Signal: SignalServiceStopped, CommandName: c.toolName, Output: jsonOutput(output),
	}
}

func (c command) start() core.Result {
	output, err := c.state.Start(StartSpec{
		Name:      c.cfg.Service,
		Binary:    c.cfg.Binary,
		Profile:   c.cfg.Profile,
		Directory: c.cfg.Directory,
		Request:   c.cfg.Request,
		Address:   c.cfg.Address,
		Env:       c.cfg.Env,
	})
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{Signal: SignalServiceStarted, CommandName: c.toolName, Output: jsonOutput(output)}
}

func (c command) awaitHealthy() core.Result {
	output, healthy := c.state.AwaitHealthy(
		c.cfg.URL,
		parseDuration(c.cfg.Timeout, defaultHealthTimeout),
		parseDuration(c.cfg.Interval, defaultHealthInterval),
	)
	signal := SignalHealthy
	if !healthy {
		signal = SignalHealthTimeout
	}
	return core.Result{Signal: signal, CommandName: c.toolName, Output: jsonOutput(output)}
}

func (c command) stop() core.Result {
	output := c.state.Stop(c.cfg.Service, parseDuration(c.cfg.Grace, defaultStopGrace))
	return core.Result{Signal: SignalServiceStopped, CommandName: c.toolName, Output: jsonOutput(output)}
}

func (c command) runValidators(ctx context.Context) core.Result {
	outcomes := RunValidators(ctx, c.cfg.Binary, c.cfg.Validators, parseDuration(c.cfg.Timeout, defaultRunTimeout))
	payload := map[string]interface{}{
		"validators": outcomes,
		"passed":     AllPassed(outcomes),
	}
	signal := SignalValidatorsCompleted
	if failure, failed := FirstFailure(outcomes); failed {
		payload["first_failure"] = failure
		if failure.TimedOut || failure.Error != "" {
			signal = SignalValidatorsIncomplete
		}
	}
	return core.Result{Signal: signal, CommandName: c.toolName, Output: jsonOutput(payload)}
}

func (c command) listScenarios() core.Result {
	scenarios, err := ListScenarios(c.cfg.Roots)
	if err != nil {
		return commandError(c.toolName, err)
	}
	return core.Result{
		Signal: SignalScenariosListed, CommandName: c.toolName,
		Output: jsonOutput(map[string]interface{}{"count": len(scenarios), "scenarios": scenarios}),
	}
}

func commandError(name string, err error) core.Result {
	return core.Result{Signal: core.CommandError, CommandName: name, Output: err.Error(), Err: err}
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
