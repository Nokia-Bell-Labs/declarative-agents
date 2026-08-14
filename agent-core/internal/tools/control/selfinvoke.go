// Copyright (c) 2026 Nokia. All rights reserved.

package control

import (
	"context"
	"fmt"
	"os"

	"go.opentelemetry.io/otel/attribute"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	toolexec "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/exec"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/undo"
)

const selfInvokeTraceAttributeLimit = 500

const (
	defaultSelfInvokeName     = "self_invoke"
	selfInvokeUndoStrategy    = "child_agent_workspace_restore"
	childWorkspaceRequirement = "child_workspace_ref"
	childTraceRequirement     = "child_trace"
)

// SelfInvokeBuilder constructs self-invocation commands.
type SelfInvokeBuilder struct {
	ToolName      string
	Config        execute.Config
	RequestFrom   string
	OutputFrom    string
	WorkspacePath string
	ExtraArgs     []string
	Ctx           context.Context
	Tracer        tracing.Tracer
}

func (b *SelfInvokeBuilder) Build(res core.Result) core.Command {
	return &selfInvokeCmd{
		toolName: b.name(),
		config:   b.Config, requestFrom: b.RequestFrom, outputFrom: b.OutputFrom,
		workspacePath: b.workspacePath(), extraArgs: b.ExtraArgs,
		ctx: b.Ctx, tracer: b.Tracer, previous: res,
		runID: toolexec.ExtractStringParam(res.Output, "run_id"),
	}
}

// BuildReverser constructs a fresh command that can only consume the persisted
// child-boundary receipt. It deliberately carries no request/output selectors,
// child invocation input, binary, or execution context, so Undo cannot rerun
// the child agent.
func (b *SelfInvokeBuilder) BuildReverser() core.Command {
	return &selfInvokeCmd{
		toolName:      b.name(),
		config:        execute.Config{Profile: b.Config.Profile},
		workspacePath: b.workspacePath(),
	}
}

func (b *SelfInvokeBuilder) name() string {
	if b.ToolName != "" {
		return b.ToolName
	}
	return defaultSelfInvokeName
}

func (b *SelfInvokeBuilder) workspacePath() string {
	if b.WorkspacePath != "" {
		return b.WorkspacePath
	}
	return b.Config.Directory
}

type selfInvokeCmd struct {
	toolName      string
	config        execute.Config
	requestFrom   string
	outputFrom    string
	workspacePath string
	extraArgs     []string
	ctx           context.Context
	tracer        tracing.Tracer
	previous      core.Result
	view          core.CommandStateView
	runID         string
	tracePath     string
}

func (c *selfInvokeCmd) Name() string {
	if c.toolName != "" {
		return c.toolName
	}
	return defaultSelfInvokeName
}
func (c *selfInvokeCmd) SetCommandState(view core.CommandStateView) { c.view = view }
func (c *selfInvokeCmd) Undo(prior core.Result) core.Result {
	compensation, err := c.decodeUndoReceipt(prior)
	if err != nil {
		return c.fault(fmt.Errorf("undo: %w", err))
	}
	return undo.BoundaryCompensationResult(c.Name(), compensation)
}

var (
	_ core.CommandStateAware = (*selfInvokeCmd)(nil)
	_ core.ContextCommand    = (*selfInvokeCmd)(nil)
	_ core.Reverser          = (*SelfInvokeBuilder)(nil)
)

func (c *selfInvokeCmd) undoPayload() undo.BoundaryCompensationPayload {
	payload := undo.BoundaryCompensationPayload{BoundaryCompensation: undo.BoundaryCompensation{
		Strategy: selfInvokeUndoStrategy,
		Reason:   "restore the child workspace and inspect retained child trace artifacts",
		Requires: []string{childWorkspaceRequirement, childTraceRequirement},
		Data: map[string]interface{}{
			"command_name":         c.Name(),
			"child_profile":        c.config.Profile,
			"child_run_id":         c.runID,
			"child_workspace_path": c.workspacePath,
			"artifact_paths":       []string{},
		},
	}}
	if c.tracePath != "" {
		payload.BoundaryCompensation.Data["artifact_paths"] = []string{c.tracePath}
	}
	return payload
}

func (c *selfInvokeCmd) Execute() core.Result {
	return c.ExecuteContext(c.ctx)
}

func (c *selfInvokeCmd) ExecuteContext(ctx context.Context) core.Result {
	cfg := c.config
	if cfg.Binary == "" {
		cfg.Binary = os.Args[0]
	}
	if err := c.resolveInvocation(&cfg); err != nil {
		wrapped := fmt.Errorf("%s: %w", c.Name(), err)
		return core.Result{
			Output: wrapped.Error(), Signal: core.CommandError,
			CommandName: c.Name(), Err: wrapped,
		}
	}
	if c.workspacePath == "" {
		c.workspacePath, _ = os.Getwd()
	}
	cfg = c.configWithTrace(cfg)
	if ctx == nil {
		ctx = context.Background()
	}
	result := execute.RunAgent(ctx, cfg, c.extraArgs...)
	c.traceResult(cfg, result)
	receipt := ""
	if childDefinitelyStarted(result) {
		receipt = undo.EncodeBoundaryReceipt(c.undoPayload())
	}
	return core.Result{
		Output: result.Stdout, Signal: selfInvokeSignal(result),
		Cost: core.Cost{Duration: result.Duration}, CommandName: c.Name(),
		Receipt: receipt,
	}
}

func childDefinitelyStarted(result *execute.Result) bool {
	return result != nil && result.Started
}

func (c *selfInvokeCmd) decodeUndoReceipt(prior core.Result) (undo.BoundaryCompensation, error) {
	if prior.CommandName != "" && prior.CommandName != c.Name() {
		return undo.BoundaryCompensation{}, fmt.Errorf(
			"receipt command %q does not match reverser %q", prior.CommandName, c.Name(),
		)
	}
	compensation, ok, err := undo.DecodeBoundaryReceipt(prior.Receipt)
	if err != nil {
		return undo.BoundaryCompensation{}, fmt.Errorf("decode child boundary receipt: %w", err)
	}
	if !ok {
		return undo.BoundaryCompensation{}, fmt.Errorf("child boundary receipt is missing compensation data")
	}
	if compensation.Strategy != selfInvokeUndoStrategy {
		return undo.BoundaryCompensation{}, fmt.Errorf(
			"child boundary receipt strategy %q does not match %q",
			compensation.Strategy, selfInvokeUndoStrategy,
		)
	}
	if err := validateSelfInvokeRequirements(compensation.Requires); err != nil {
		return undo.BoundaryCompensation{}, err
	}
	name, err := requiredReceiptString(compensation.Data, "command_name")
	if err != nil {
		return undo.BoundaryCompensation{}, err
	}
	if name != c.Name() {
		return undo.BoundaryCompensation{}, fmt.Errorf(
			"child boundary receipt command %q does not match reverser %q", name, c.Name(),
		)
	}
	profile, err := requiredReceiptString(compensation.Data, "child_profile")
	if err != nil {
		return undo.BoundaryCompensation{}, err
	}
	if c.config.Profile != "" && profile != c.config.Profile {
		return undo.BoundaryCompensation{}, fmt.Errorf(
			"child boundary receipt profile %q does not match configured profile %q",
			profile, c.config.Profile,
		)
	}
	if _, err := requiredReceiptString(compensation.Data, "child_run_id"); err != nil {
		return undo.BoundaryCompensation{}, err
	}
	workspacePath, err := requiredReceiptString(compensation.Data, "child_workspace_path")
	if err != nil {
		return undo.BoundaryCompensation{}, err
	}
	if c.workspacePath != "" && workspacePath != c.workspacePath {
		return undo.BoundaryCompensation{}, fmt.Errorf(
			"child boundary receipt workspace path %q does not match configured workspace path %q",
			workspacePath, c.workspacePath,
		)
	}
	if err := validateArtifactPaths(compensation.Data["artifact_paths"]); err != nil {
		return undo.BoundaryCompensation{}, err
	}
	return compensation, nil
}

func validateSelfInvokeRequirements(requires []string) error {
	present := make(map[string]bool, len(requires))
	for _, requirement := range requires {
		if requirement == "" {
			return fmt.Errorf("child boundary receipt has an empty compensation requirement")
		}
		present[requirement] = true
	}
	for _, required := range []string{childWorkspaceRequirement, childTraceRequirement} {
		if !present[required] {
			return fmt.Errorf("child boundary receipt is missing compensation requirement %q", required)
		}
	}
	return nil
}

func requiredReceiptString(data map[string]interface{}, field string) (string, error) {
	if data == nil {
		return "", fmt.Errorf("child boundary receipt is missing data")
	}
	value, ok := data[field]
	if !ok {
		return "", fmt.Errorf("child boundary receipt is missing %s", field)
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("child boundary receipt %s has type %T, want string", field, value)
	}
	if text == "" {
		return "", fmt.Errorf("child boundary receipt has empty %s", field)
	}
	return text, nil
}

func validateArtifactPaths(value interface{}) error {
	paths, ok := value.([]interface{})
	if !ok {
		return fmt.Errorf("child boundary receipt artifact_paths has type %T, want array", value)
	}
	for _, path := range paths {
		text, ok := path.(string)
		if !ok || text == "" {
			return fmt.Errorf("child boundary receipt artifact_paths contains invalid path %v", path)
		}
	}
	return nil
}

func (c *selfInvokeCmd) fault(err error) core.Result {
	wrapped := fmt.Errorf("%s: %w", c.Name(), err)
	return core.Result{
		Output: wrapped.Error(), Signal: core.CommandError,
		CommandName: c.Name(), Err: wrapped,
	}
}

func (c *selfInvokeCmd) resolveInvocation(cfg *execute.Config) error {
	var err error
	if c.requestFrom != "" {
		cfg.Request, err = c.resolveString(c.requestFrom)
		if err != nil {
			return fmt.Errorf("request_from: %w", err)
		}
	}
	if c.outputFrom != "" {
		cfg.Output, err = c.resolveString(c.outputFrom)
		if err != nil {
			return fmt.Errorf("output_from: %w", err)
		}
	}
	return nil
}

func (c *selfInvokeCmd) resolveString(selector string) (string, error) {
	value, err := core.ResolveFromSelector(c.view, selector)
	if err != nil {
		return "", err
	}
	text, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("selector %q resolved to %T, want string", selector, value)
	}
	if text == "" {
		return "", fmt.Errorf("selector %q resolved to an empty string", selector)
	}
	return text, nil
}

func (c *selfInvokeCmd) configWithTrace(cfg execute.Config) execute.Config {
	if cfg.OTelDir == "" {
		return cfg
	}
	c.tracePath = fmt.Sprintf("%s/child-%s.otel.json", cfg.OTelDir, c.runID)
	cfg.OTelLogFile = c.tracePath
	return cfg
}

func (c *selfInvokeCmd) traceResult(cfg execute.Config, result *execute.Result) {
	if c.tracer == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("self_invoke.binary", modelllm.Truncate(cfg.Binary, selfInvokeTraceAttributeLimit)),
		attribute.String("self_invoke.profile", modelllm.Truncate(cfg.Profile, selfInvokeTraceAttributeLimit)),
		attribute.String("self_invoke.run_id", modelllm.Truncate(c.runID, selfInvokeTraceAttributeLimit)),
		attribute.Int("self_invoke.exit_code", result.ExitCode),
		attribute.String("self_invoke.output", modelllm.Truncate(result.Stdout, selfInvokeTraceAttributeLimit)),
		attribute.String("self_invoke.stdout", modelllm.Truncate(result.Stdout, selfInvokeTraceAttributeLimit)),
		attribute.String("self_invoke.stderr", modelllm.Truncate(result.Stderr, selfInvokeTraceAttributeLimit)),
	}
	c.tracer.SetAttributes(attrs...)
	c.tracer.Event("self_invoke.result", attrs...)
}

func selfInvokeSignal(result *execute.Result) core.Signal {
	if result.Success() {
		return core.ToolDone
	}
	return core.ToolFailed
}
