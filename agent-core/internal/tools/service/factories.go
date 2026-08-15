// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	InitStopService = "stop_service"

	// The assembler's session words. Each per-scenario step reads the current
	// scenario from the session, so the pipeline stays visible as machine
	// transitions while working on data discovered at runtime (srd018 R1).
	InitInitScenarioSession  = "init_scenario_session"
	InitNextScenario         = "next_scenario"
	InitStartScenarioMock    = "start_scenario_mock"
	InitStartSubject         = "start_scenario_subject"
	InitRunScenarioValidator = "run_scenario_validator"
	InitRecordValidators     = "record_scenario_validators"
	InitCollectVerdict       = "collect_scenario_verdict"
	InitListScenarioChildren = "list_scenario_children"
	InitStopAllServices      = "stop_all_services"
	InitReportSession        = "report_scenario_session"
)

// StandardInits lists every service builtin init name.
var StandardInits = []string{
	InitStopService,
	InitInitScenarioSession, InitNextScenario, InitStartScenarioMock, InitStartSubject,
	InitRunScenarioValidator, InitRecordValidators, InitCollectVerdict, InitListScenarioChildren,
	InitStopAllServices, InitReportSession,
}

// Result signals distinguish each child operation and thin session mutation.
const (
	SignalServiceStopped      core.Signal = "ServiceStopped"
	SignalValidatorCompleted  core.Signal = "ValidatorCompleted"
	SignalValidatorIncomplete core.Signal = "ValidatorIncomplete"
	SignalValidatorsRecorded  core.Signal = "ValidatorsRecorded"

	SignalSessionSeeded          core.Signal = "SessionSeeded"
	SignalNoScenarios            core.Signal = "NoScenarios"
	SignalScenarioReady          core.Signal = "ScenarioReady"
	SignalAllScenariosDone       core.Signal = "AllScenariosDone"
	SignalMockStarted            core.Signal = "MockStarted"
	SignalSubjectStarted         core.Signal = "SubjectStarted"
	SignalScenarioChildrenListed core.Signal = "ScenarioChildrenListed"
	SignalAllServicesStopped     core.Signal = "AllServicesStopped"
	SignalScenarioPassed         core.Signal = "ScenarioPassed"
	SignalScenarioFailed         core.Signal = "ScenarioFailed"
	SignalSessionPassed          core.Signal = "SessionPassed"
	SignalSessionFailed          core.Signal = "SessionFailed"
)

// ToolConfig is the declared config for every service word. Each word reads
// the fields it needs; unrelated fields stay empty.
type ToolConfig struct {
	Service   string   `yaml:"service,omitempty"`
	Binary    string   `yaml:"binary,omitempty"`
	Profile   string   `yaml:"profile,omitempty"`
	Directory string   `yaml:"directory,omitempty"`
	Env       []string `yaml:"env,omitempty"`

	Timeout      string `yaml:"timeout,omitempty"`
	Grace        string `yaml:"grace,omitempty"`
	OTLPEndpoint string `yaml:"otlp_endpoint,omitempty" json:"otlp_endpoint,omitempty"`

	Roots     []string `yaml:"roots,omitempty"`
	Fixture   string   `yaml:"fixture,omitempty"`
	Validator string   `yaml:"validator,omitempty"`
	Outcomes  string   `yaml:"outcomes,omitempty"`

	// Reason forces a scenario verdict to fail with this text, so a machine
	// can route a failed start or health step to a verdict that names the
	// cause rather than an empty one.
	Reason string `yaml:"reason,omitempty"`

	// AddressEnv names the environment variable that carries the address the
	// rig allocated to a child. A child binds a port the rig chose, so it has
	// to be told which one: its profile declares address: ${VAR:-...} and this
	// names that VAR. Defaults to MOCK_ADDRESS for mocks and SUBJECT_ADDRESS
	// for the subject.
	AddressEnv string `yaml:"address_env,omitempty"`
}

// FactoryDeps holds service factory dependencies.
type FactoryDeps struct {
	State    *State
	Session  *ScenarioSessionState
	CoreRoot string
}

// RegisterBuiltins registers every service builtin factory. The session and
// the service state are shared across the words, so every child started during
// a run is reachable for teardown.
func RegisterBuiltins(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	if deps.State == nil {
		deps.State = NewState()
	}
	if deps.Session == nil {
		deps.Session = NewScenarioSession(deps.State)
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
		return Builder{
			ToolName: def.Name, Init: init, Config: cfg,
			State: deps.State, Session: deps.Session, CoreRoot: deps.CoreRoot,
		}, nil
	}
}

func validateToolConfig(name, init string, cfg ToolConfig) error {
	switch init {
	case InitStopService:
		if cfg.Service == "" {
			return fmt.Errorf("tool %q (%s) requires a service name or selector", name, init)
		}
		if strings.HasPrefix(cfg.Service, "$") {
			if _, _, ok := core.ParseFromSelector(cfg.Service); !ok {
				return fmt.Errorf("tool %q (%s) service must be a $from(label).path selector", name, init)
			}
		}
	case InitInitScenarioSession:
		if len(cfg.Roots) == 0 {
			return fmt.Errorf("tool %q (%s) requires at least one root", name, init)
		}
	case InitStartScenarioMock:
		if cfg.Profile == "" {
			return fmt.Errorf("tool %q (%s) requires the mock profile", name, init)
		}
		if _, _, ok := core.ParseFromSelector(cfg.Fixture); !ok {
			return fmt.Errorf("tool %q (%s) fixture must be a $from(label).path selector", name, init)
		}
	case InitRunScenarioValidator:
		if _, _, ok := core.ParseFromSelector(cfg.Validator); !ok {
			return fmt.Errorf("tool %q (%s) validator must be a $from(label).path selector", name, init)
		}
	case InitRecordValidators:
		if _, _, ok := core.ParseFromSelector(cfg.Outcomes); !ok {
			return fmt.Errorf("tool %q (%s) outcomes must be a $from(label).path selector", name, init)
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
	Session  *ScenarioSessionState
	CoreRoot string
}

// Build creates one service command.
func (b Builder) Build(_ core.Result) core.Command {
	session := b.Session
	if session == nil {
		session = NewScenarioSession(b.State)
	}
	return &command{
		toolName: b.ToolName, init: b.Init, cfg: b.Config,
		state: b.State, session: session, coreRoot: b.CoreRoot,
	}
}

// BuildReverser reconstructs a receipt-only command for child rollback.
func (b Builder) BuildReverser() core.Command {
	return b.Build(core.Result{})
}

type command struct {
	toolName     string
	init         string
	cfg          ToolConfig
	state        *State
	session      *ScenarioSessionState
	coreRoot     string
	commandState core.CommandStateView
}

func (c *command) Name() string { return c.toolName }

func (c *command) SetCommandState(view core.CommandStateView) { c.commandState = view }

func (c *command) Execute() core.Result { return c.ExecuteContext(context.Background()) }

func (c *command) ExecuteContext(ctx context.Context) core.Result {
	switch c.init {
	case InitStopService:
		return c.stop()
	case InitInitScenarioSession:
		return c.initSession()
	case InitNextScenario:
		return c.nextScenario()
	case InitStartScenarioMock:
		return c.startScenarioMock()
	case InitStartSubject:
		return c.startSubject()
	case InitRunScenarioValidator:
		return c.runScenarioValidator(ctx)
	case InitRecordValidators:
		return c.recordScenarioValidators()
	case InitCollectVerdict:
		return c.collectVerdict()
	case InitListScenarioChildren:
		return c.listScenarioChildren()
	case InitStopAllServices:
		return c.stopAllServices()
	case InitReportSession:
		return c.reportSession()
	default:
		return commandError(c.toolName, fmt.Errorf("unsupported service init %q", c.init))
	}
}

func (c *command) stopAllServices() core.Result {
	stopped := c.state.Reap()
	return core.Result{
		Signal: SignalAllServicesStopped, CommandName: c.toolName,
		Output: jsonOutput(map[string]any{"stopped": len(stopped), "children": stopped}),
	}
}

// Undo reverses scenario-specific child starts from their receipts; every
// other word is read-only or already terminal, so its undo is a noop (srd040
// R1.5, R3.3). The declarations must match this, or the corpus audit reports a
// tool-undo mismatch.
func (c *command) Undo(prior core.Result) core.Result {
	switch c.init {
	case InitStartScenarioMock:
		return c.undoStartedChild(prior)
	case InitStartSubject:
		return c.undoStartedChild(prior)
	default:
		return core.NoopUndo(c.toolName)
	}
}

func (c *command) undoStartedChild(prior core.Result) core.Result {
	var receipt struct {
		Service string `json:"service"`
	}
	if err := json.Unmarshal([]byte(prior.Receipt), &receipt); err != nil || receipt.Service == "" {
		return commandError(c.toolName, fmt.Errorf("%s: invalid child receipt", c.toolName))
	}
	output := c.state.Stop(receipt.Service, parseDuration(c.cfg.Grace, defaultStopGrace))
	return core.Result{
		Signal: SignalServiceStopped, CommandName: c.toolName, Output: jsonOutput(output),
	}
}

func (c command) stop() core.Result {
	service, err := c.serviceName()
	if err != nil {
		return commandError(c.toolName, err)
	}
	output := c.state.Stop(service, parseDuration(c.cfg.Grace, defaultStopGrace))
	return core.Result{
		Signal: SignalServiceStopped, CommandName: c.toolName,
		Output: jsonOutput(output),
	}
}

func (c command) serviceName() (string, error) {
	if _, _, selector := core.ParseFromSelector(c.cfg.Service); !selector {
		return c.cfg.Service, nil
	}
	value, err := core.ResolveFromSelector(c.commandState, c.cfg.Service)
	if err != nil {
		return "", err
	}
	service, ok := value.(string)
	if !ok || service == "" {
		return "", fmt.Errorf("%s: service selector did not resolve to a string", c.toolName)
	}
	return service, nil
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

var _ core.Reverser = Builder{}
var _ core.CommandStateAware = (*command)(nil)
