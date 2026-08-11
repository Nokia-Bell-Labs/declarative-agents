// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

// ParseSuiteConfigBuilder creates parseSuiteConfigCmd instances.
type ParseSuiteConfigBuilder struct {
	ES *EvalSessionState
}

func (b *ParseSuiteConfigBuilder) Build(_ core.Result) core.Command {
	return &evaluatorReceiptCmd{inner: &parseSuiteConfigCmd{es: b.ES}, session: b.ES}
}

func (b *ParseSuiteConfigBuilder) BuildReverser() core.Command {
	return &evaluatorReceiptCmd{inner: &parseSuiteConfigCmd{es: b.ES}, session: b.ES}
}

type parseSuiteConfigCmd struct {
	es *EvalSessionState
}

func (c *parseSuiteConfigCmd) Name() string { return "parse_suite_config" }
func (c *parseSuiteConfigCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, session: c.es}).Undo(prior)
}

func (c *parseSuiteConfigCmd) Execute() core.Result {
	if c.es.SuitePath == "" {
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("parse_suite_config: no suite path configured"),
			Output:      "no suite path configured",
			CommandName: c.Name(),
		}
	}

	data, err := os.ReadFile(c.es.SuitePath)
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("read suite: %w", err),
			Output:      fmt.Sprintf("read suite: %v", err),
			CommandName: c.Name(),
		}
	}

	suite, err := ParseSuiteConfig(data, filepath.Dir(c.es.SuitePath))
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("parse suite config: %v", err),
			CommandName: c.Name(),
		}
	}

	if c.es.ChildAgentBinary != "" {
		for i := range suite.Profiles {
			suite.Profiles[i].Binary = c.es.ChildAgentBinary
		}
	}

	c.es.Suite = suite
	return core.Result{
		Signal:      SigSuiteConfigParsed,
		Output:      fmt.Sprintf("parsed suite %q", suite.Name),
		CommandName: c.Name(),
	}
}

// DiscoverSuiteSamplesBuilder creates discoverSuiteSamplesCmd instances.
type DiscoverSuiteSamplesBuilder struct {
	ES *EvalSessionState
}

func (b *DiscoverSuiteSamplesBuilder) Build(_ core.Result) core.Command {
	return &evaluatorReceiptCmd{inner: &discoverSuiteSamplesCmd{es: b.ES}, session: b.ES}
}

func (b *DiscoverSuiteSamplesBuilder) BuildReverser() core.Command {
	return &evaluatorReceiptCmd{inner: &discoverSuiteSamplesCmd{es: b.ES}, session: b.ES}
}

type discoverSuiteSamplesCmd struct {
	es *EvalSessionState
}

func (c *discoverSuiteSamplesCmd) Name() string { return "discover_suite_samples" }
func (c *discoverSuiteSamplesCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, session: c.es}).Undo(prior)
}

func (c *discoverSuiteSamplesCmd) Execute() core.Result {
	samples, err := DiscoverSamples(c.es.Suite.SamplesDir, c.es.SampleLayout)
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("discover samples: %v", err),
			CommandName: c.Name(),
		}
	}
	c.es.Suite.Samples = samples
	return core.Result{
		Signal:      SigSuiteSamplesDiscovered,
		Output:      fmt.Sprintf("discovered %d samples", len(samples)),
		CommandName: c.Name(),
	}
}

// ExpandEvalGridBuilder creates expandEvalGridCmd instances.
type ExpandEvalGridBuilder struct {
	ES *EvalSessionState
}

func (b *ExpandEvalGridBuilder) Build(_ core.Result) core.Command {
	return &evaluatorReceiptCmd{inner: &expandEvalGridCmd{es: b.ES}, session: b.ES}
}

func (b *ExpandEvalGridBuilder) BuildReverser() core.Command {
	return &evaluatorReceiptCmd{inner: &expandEvalGridCmd{es: b.ES}, session: b.ES}
}

type expandEvalGridCmd struct {
	es *EvalSessionState
}

func (c *expandEvalGridCmd) Name() string { return "expand_eval_grid" }
func (c *expandEvalGridCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, session: c.es}).Undo(prior)
}

func (c *expandEvalGridCmd) Execute() core.Result {
	c.es.ExpandGrid()
	return core.Result{
		Signal:      SigEvalGridExpanded,
		Output:      fmt.Sprintf("expanded %d grid points", len(c.es.gridPoints)),
		CommandName: c.Name(),
	}
}

// InitEvalSessionBuilder creates initEvalSessionCmd instances.
type InitEvalSessionBuilder struct {
	ES *EvalSessionState
}

func (b *InitEvalSessionBuilder) Build(_ core.Result) core.Command {
	return &evaluatorReceiptCmd{
		inner: &initEvalSessionCmd{es: b.ES}, session: b.ES,
		removePaths: func() []string { return []string{b.ES.SessionDir} },
		removeRoot:  func() string { return filepath.Dir(b.ES.SessionDir) },
	}
}

func (b *InitEvalSessionBuilder) BuildReverser() core.Command {
	return &evaluatorReceiptCmd{inner: &initEvalSessionCmd{es: b.ES}, session: b.ES}
}

type initEvalSessionCmd struct {
	es *EvalSessionState
}

func (c *initEvalSessionCmd) Name() string { return "init_eval_session" }
func (c *initEvalSessionCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, session: c.es}).Undo(prior)
}

func (c *initEvalSessionCmd) Execute() core.Result {
	outputDir := c.es.OutputDir
	if outputDir == "" {
		outputDir = c.es.DefaultOutputDir
	}
	reps := c.es.Reps
	if reps == 0 && c.es.Suite.Reps > 0 {
		reps = c.es.Suite.Reps
	}
	if reps == 0 {
		reps = c.es.DefaultReps
	}

	timeout := c.es.Timeout
	if timeout == 0 && c.es.Suite.Timeout > 0 {
		timeout = c.es.Suite.Timeout
	}
	if timeout == 0 {
		timeout = c.es.DefaultTimeout
	}

	ollamaURL := c.es.OllamaURL
	if ollamaURL == "" && c.es.Suite.OllamaURL != "" {
		ollamaURL = c.es.Suite.OllamaURL
	}

	if err := c.es.InitSession(outputDir, reps, timeout, ollamaURL, 0); err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("init session: %v", err),
			CommandName: c.Name(),
		}
	}

	return core.Result{
		Signal:      SigEvalSessionInitialized,
		Output:      fmt.Sprintf("initialized session %s", c.es.SessionDir),
		CommandName: c.Name(),
	}
}

// ReportSuiteSummaryBuilder creates reportSuiteSummaryCmd instances.
type ReportSuiteSummaryBuilder struct {
	ES *EvalSessionState
}

func (b *ReportSuiteSummaryBuilder) Build(_ core.Result) core.Command {
	return &reportSuiteSummaryCmd{es: b.ES}
}

type reportSuiteSummaryCmd struct {
	es *EvalSessionState
}

func (c *reportSuiteSummaryCmd) Name() string                   { return "report_suite_summary" }
func (c *reportSuiteSummaryCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func (c *reportSuiteSummaryCmd) Execute() core.Result {
	suite := c.es.Suite
	total := len(suite.Profiles) * len(c.es.gridPoints) * len(suite.Samples) * c.es.reps
	_, _ = fmt.Fprintf(c.es.Stderr, "Suite %q: %d profiles x %d samples x %d reps = %d points\n",
		suite.Name, len(suite.Profiles), len(suite.Samples), c.es.reps, total)

	return core.Result{
		Signal:      SigSuiteLoaded,
		Output:      fmt.Sprintf("loaded suite %q with %d points", suite.Name, total),
		CommandName: c.Name(),
	}
}

// Config keys: input, output_dir, reps, timeout, ollama_url.
func evaluatorSessionConfigFactory(es *EvalSessionState, build func(*EvalSessionState) core.Builder) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, vars map[string]string) (core.Builder, error) {
		if err := applyLoadSuiteConfig(es, def); err != nil {
			return nil, err
		}
		return build(es), nil
	}
}

func applyLoadSuiteConfig(es *EvalSessionState, def catalog.ToolDef) error {
	var cfg catalog.LoadSuiteConfig
	if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
		return err
	}
	if es.SuitePath == "" && cfg.Input != "" {
		es.SuitePath = cfg.Input
	}
	if es.OutputDir == "" && es.DefaultOutputDir == "" && cfg.OutputDir != "" {
		es.DefaultOutputDir = cfg.OutputDir
	}
	if es.Reps == 0 && es.DefaultReps == 0 && cfg.Reps > 0 {
		es.DefaultReps = cfg.Reps
	}
	if es.Timeout == 0 && es.DefaultTimeout == 0 && cfg.Timeout > 0 {
		es.DefaultTimeout = time.Duration(cfg.Timeout) * time.Second
	}
	if es.OllamaURL == "" && cfg.OllamaURL != "" {
		es.OllamaURL = cfg.OllamaURL
	}
	if cfg.WorkspaceDir != "" || cfg.DocDir != "" || cfg.PromptFile != "" ||
		cfg.AllowSharedPrompt || cfg.RequireSamples {
		if cfg.WorkspaceDir == "" || cfg.PromptFile == "" {
			return fmt.Errorf("tool %q config requires workspace_dir and prompt_file", def.Name)
		}
		es.SampleLayout = SampleLayout{
			WorkspaceDir: cfg.WorkspaceDir, DocDir: cfg.DocDir, PromptFile: cfg.PromptFile,
			AllowSharedPrompt: cfg.AllowSharedPrompt, RequireSamples: cfg.RequireSamples,
		}
	}
	return nil
}
