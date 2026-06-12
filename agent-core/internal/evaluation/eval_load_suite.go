// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/stl"
	"os"
	"path/filepath"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// ParseSuiteConfigBuilder creates parseSuiteConfigCmd instances.
type ParseSuiteConfigBuilder struct {
	ES *EvalSessionState
}

func (b *ParseSuiteConfigBuilder) Build(_ core.Result) core.Command {
	return &parseSuiteConfigCmd{es: b.ES}
}

type parseSuiteConfigCmd struct {
	es          *EvalSessionState
	snapshot    evalSessionSnapshot
	hasSnapshot bool
}

func (c *parseSuiteConfigCmd) Name() string { return "parse_suite_config" }
func (c *parseSuiteConfigCmd) Undo() core.Result {
	return undoEvalSessionSnapshot(c.Name(), c.es, c.snapshot, c.hasSnapshot)
}
func (c *parseSuiteConfigCmd) UndoMemento() (core.UndoMemento, error) {
	return evalSessionMemento(c.Name(), c.snapshot, c.hasSnapshot)
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

	c.snapshot = snapshotEvalSession(c.es)
	c.hasSnapshot = true
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
	return &discoverSuiteSamplesCmd{es: b.ES}
}

type discoverSuiteSamplesCmd struct {
	es          *EvalSessionState
	snapshot    evalSessionSnapshot
	hasSnapshot bool
}

func (c *discoverSuiteSamplesCmd) Name() string { return "discover_suite_samples" }
func (c *discoverSuiteSamplesCmd) Undo() core.Result {
	return undoEvalSessionSnapshot(c.Name(), c.es, c.snapshot, c.hasSnapshot)
}
func (c *discoverSuiteSamplesCmd) UndoMemento() (core.UndoMemento, error) {
	return evalSessionMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *discoverSuiteSamplesCmd) Execute() core.Result {
	samples, err := DiscoverSamples(c.es.Suite.SamplesDir)
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("discover samples: %v", err),
			CommandName: c.Name(),
		}
	}
	c.snapshot = snapshotEvalSession(c.es)
	c.hasSnapshot = true
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
	return &expandEvalGridCmd{es: b.ES}
}

type expandEvalGridCmd struct {
	es          *EvalSessionState
	snapshot    evalSessionSnapshot
	hasSnapshot bool
}

func (c *expandEvalGridCmd) Name() string { return "expand_eval_grid" }
func (c *expandEvalGridCmd) Undo() core.Result {
	return undoEvalSessionSnapshot(c.Name(), c.es, c.snapshot, c.hasSnapshot)
}
func (c *expandEvalGridCmd) UndoMemento() (core.UndoMemento, error) {
	return evalSessionMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *expandEvalGridCmd) Execute() core.Result {
	c.snapshot = snapshotEvalSession(c.es)
	c.hasSnapshot = true
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
	return &initEvalSessionCmd{es: b.ES}
}

type initEvalSessionCmd struct {
	es          *EvalSessionState
	snapshot    evalSessionSnapshot
	hasSnapshot bool
}

func (c *initEvalSessionCmd) Name() string { return "init_eval_session" }
func (c *initEvalSessionCmd) Undo() core.Result {
	return undoEvalSessionSnapshot(c.Name(), c.es, c.snapshot, c.hasSnapshot)
}
func (c *initEvalSessionCmd) UndoMemento() (core.UndoMemento, error) {
	return evalSessionMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *initEvalSessionCmd) Execute() core.Result {
	reps := c.es.Reps
	if reps == 0 && c.es.Suite.Reps > 0 {
		reps = c.es.Suite.Reps
	}

	timeout := c.es.Timeout
	if timeout == 0 && c.es.Suite.Timeout > 0 {
		timeout = c.es.Suite.Timeout
	}

	ollamaURL := c.es.OllamaURL
	if ollamaURL == "" && c.es.Suite.OllamaURL != "" {
		ollamaURL = c.es.Suite.OllamaURL
	}

	c.snapshot = snapshotEvalSession(c.es)
	c.hasSnapshot = true
	if err := c.es.InitSession(c.es.OutputDir, reps, timeout, ollamaURL, 0); err != nil {
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

func (c *reportSuiteSummaryCmd) Name() string      { return "report_suite_summary" }
func (c *reportSuiteSummaryCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *reportSuiteSummaryCmd) Execute() core.Result {
	suite := c.es.Suite
	var total int
	if len(suite.Profiles) > 0 {
		total = len(suite.Profiles) * len(c.es.gridPoints) * len(suite.Samples) * c.es.reps
		fmt.Fprintf(c.es.Stderr, "Suite %q: %d profiles x %d samples x %d reps = %d points\n",
			suite.Name, len(suite.Profiles), len(suite.Samples), c.es.reps, total)
	} else {
		total = len(suite.Harnesses) * len(suite.Models) * len(c.es.gridPoints) * len(suite.Samples) * c.es.reps
		fmt.Fprintf(c.es.Stderr, "Suite %q: %d harnesses x %d models x %d samples x %d reps = %d points\n",
			suite.Name, len(suite.Harnesses), len(suite.Models), len(suite.Samples), c.es.reps, total)
	}

	return core.Result{
		Signal:      SigSuiteLoaded,
		Output:      fmt.Sprintf("loaded suite %q with %d points", suite.Name, total),
		CommandName: c.Name(),
	}
}

// Config keys: input, output_dir, reps, timeout, ollama_url.
func evaluatorSessionConfigFactory(es *EvalSessionState, build func(*EvalSessionState) core.Builder) stl.BuiltinFactory {
	return func(def stl.ToolDef, vars map[string]string) (core.Builder, error) {
		if err := applyLoadSuiteConfig(es, def); err != nil {
			return nil, err
		}
		return build(es), nil
	}
}

func applyLoadSuiteConfig(es *EvalSessionState, def stl.ToolDef) error {
	var cfg stl.LoadSuiteConfig
	if err := stl.DecodeToolConfig(def, &cfg); err != nil {
		return err
	}
	if es.SuitePath == "" && cfg.Input != "" {
		es.SuitePath = cfg.Input
	}
	if es.OutputDir == "" && cfg.OutputDir != "" {
		es.OutputDir = cfg.OutputDir
	}
	if es.OutputDir == "" {
		es.OutputDir = "eval-results"
	}
	if es.Reps == 0 && cfg.Reps > 0 {
		es.Reps = cfg.Reps
	}
	if es.Timeout == 0 && cfg.Timeout > 0 {
		es.Timeout = time.Duration(cfg.Timeout) * time.Second
	}
	if es.OllamaURL == "" && cfg.OllamaURL != "" {
		es.OllamaURL = cfg.OllamaURL
	}
	return nil
}
