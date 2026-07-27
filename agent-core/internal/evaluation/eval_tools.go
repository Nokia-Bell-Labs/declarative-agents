// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// Signals emitted by atomic evaluator workspace preparation commands.
const (
	SigPointDirCreated core.Signal = "PointDirCreated"
	SigDocsPresent     core.Signal = "SampleDocsPresent"
	SigDocsAbsent      core.Signal = "SampleDocsAbsent"
	SigFailureRecorded core.Signal = "PointFailureRecorded"
)

// createPointDirCmd creates the per-point directory and records paths that
// later point tools consume.
type createPointDirCmd struct {
	pc *PointContext
}

func (c *createPointDirCmd) Name() string { return "create_point_dir" }
func (c *createPointDirCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, point: c.pc}).Undo(prior)
}

func (c *createPointDirCmd) Execute() core.Result {
	pointDir := filepath.Join(c.pc.SessionDir, c.pc.PointID)
	if err := os.MkdirAll(pointDir, 0o755); err != nil {
		return pointToolError(c.Name(), fmt.Errorf("mkdir point dir: %w", err))
	}
	c.pc.PointDir = pointDir
	c.pc.TracePath = filepath.Join(pointDir, ArtifactTrace)
	c.pc.ResultPath = filepath.Join(pointDir, ArtifactResult)
	output, _ := json.Marshal(map[string]any{
		"parameters": map[string]string{
			"source":      c.pc.Sample.WorkspaceDir + string(os.PathSeparator) + ".",
			"destination": pointDir,
		},
	})
	return pointToolDone(c.Name(), SigPointDirCreated, string(output))
}

// sampleDocsCmd exposes the optional-docs branch and copy_dir parameters
// without performing filesystem work.
type sampleDocsCmd struct {
	pc *PointContext
}

func (c *sampleDocsCmd) Name() string                   { return "sample_docs" }
func (c *sampleDocsCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func (c *sampleDocsCmd) Execute() core.Result {
	if err := requirePointDir(c.pc); err != nil {
		return pointToolError(c.Name(), err)
	}
	if c.pc.Sample.DocDir == "" {
		return pointToolDone(c.Name(), SigDocsAbsent, `{"present":false}`)
	}
	output, err := json.Marshal(map[string]any{
		"present": true,
		"parameters": map[string]string{
			"source":      c.pc.Sample.DocDir + string(os.PathSeparator) + ".",
			"destination": filepath.Join(c.pc.PointDir, ArtifactDocDir),
		},
	})
	if err != nil {
		return pointToolError(c.Name(), fmt.Errorf("encode docs copy parameters: %w", err))
	}
	return pointToolDone(c.Name(), SigDocsPresent, string(output))
}

func pointToolDone(command string, signal core.Signal, output string) core.Result {
	return core.Result{
		CommandName: command,
		Signal:      signal,
		Output:      output,
	}
}

func pointToolError(command string, err error) core.Result {
	return core.Result{
		CommandName: command,
		Signal:      core.CommandError,
		Err:         err,
		Output:      err.Error(),
	}
}

func requirePointDir(pc *PointContext) error {
	if pc.PointDir == "" {
		return fmt.Errorf("point dir not initialized")
	}
	return nil
}

// recordOracleResultCmd maps the configured oracle exec result into point state.
type recordOracleResultCmd struct {
	pc    *PointContext
	prior core.Result
}

func (c *recordOracleResultCmd) Name() string { return "record_oracle_result" }
func (c *recordOracleResultCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, point: c.pc}).Undo(prior)
}

func (c *recordOracleResultCmd) Execute() core.Result {
	pc := c.pc
	pc.TestsPassed = c.prior.Signal == core.ToolDone
	pc.TestOutput = c.prior.Output

	signal := SigOracleCheckPassed
	if !pc.TestsPassed {
		signal = SigOracleCheckFailed
	}
	return core.Result{
		CommandName: c.Name(),
		Signal:      signal,
		Output:      pc.TestOutput,
	}
}

// collectTraceTokensCmd extracts token usage from the point trace file.
type collectTraceTokensCmd struct {
	pc *PointContext
}

func (c *collectTraceTokensCmd) Name() string { return "collect_trace_tokens" }
func (c *collectTraceTokensCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, point: c.pc}).Undo(prior)
}

func (c *collectTraceTokensCmd) Execute() core.Result {
	pc := c.pc
	if _, err := os.Stat(pc.TracePath); err != nil {
		if os.IsNotExist(err) {
			pc.Tokens = 0
			return core.Result{
				CommandName: c.Name(),
				Signal:      SigTraceTokensCollected,
				Output:      "trace file not found; tokens=0",
			}
		}
		return pointToolError(c.Name(), fmt.Errorf("stat trace: %w", err))
	}

	spans, err := ReadTraceFile(pc.TracePath)
	if err != nil {
		return pointToolError(c.Name(), err)
	}

	total := 0
	for _, s := range spans {
		total += IntAttr(s, "gen_ai.usage.input_tokens")
		total += IntAttr(s, "gen_ai.usage.output_tokens")
	}
	pc.Tokens = total

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigTraceTokensCollected,
		Output:      fmt.Sprintf("collected %d trace tokens", pc.Tokens),
		Cost:        core.Cost{TokensIn: pc.Tokens},
	}
}

// checkAgentVersionCmd compares configured and traced agent versions.
type checkAgentVersionCmd struct {
	pc *PointContext
}

func (c *checkAgentVersionCmd) Name() string { return "check_agent_version" }
func (c *checkAgentVersionCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, point: c.pc}).Undo(prior)
}

func (c *checkAgentVersionCmd) Execute() core.Result {
	pc := c.pc
	if pc.Harness.Version == "" {
		return core.Result{
			CommandName: c.Name(),
			Signal:      SigAgentVersionChecked,
			Output:      "no harness version configured",
		}
	}
	if _, err := os.Stat(pc.TracePath); err != nil {
		if os.IsNotExist(err) {
			return core.Result{
				CommandName: c.Name(),
				Signal:      SigAgentVersionChecked,
				Output:      "trace file not found; version check skipped",
			}
		}
		return pointToolError(c.Name(), fmt.Errorf("stat trace: %w", err))
	}

	spans, err := ReadTraceFile(pc.TracePath)
	if err != nil {
		return pointToolError(c.Name(), err)
	}
	pc.TraceVersion = AgentVersion(spans)
	if pc.TraceVersion != "" && pc.TraceVersion != pc.Harness.Version {
		pc.VersionMismatch = true
		msg := fmt.Sprintf("version mismatch: config=%s trace=%s", pc.Harness.Version, pc.TraceVersion)
		if pc.Stderr != nil {
			_, _ = fmt.Fprintf(pc.Stderr, "  WARN: %s\n", msg)
		}
		return core.Result{
			CommandName: c.Name(),
			Signal:      SigAgentVersionMismatch,
			Output:      msg,
		}
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigAgentVersionChecked,
		Output:      fmt.Sprintf("agent version checked: config=%s trace=%s", pc.Harness.Version, pc.TraceVersion),
	}
}

// summarizePointResultsCmd emits the aggregate point result after prior words
// have populated oracle, trace, and version state.
type summarizePointResultsCmd struct {
	pc *PointContext
}

func (c *summarizePointResultsCmd) Name() string                   { return "summarize_point_results" }
func (c *summarizePointResultsCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

func (c *summarizePointResultsCmd) Execute() core.Result {
	pc := c.pc
	output := fmt.Sprintf("tests_passed=%t tokens=%d", pc.TestsPassed, pc.Tokens)
	if pc.VersionMismatch {
		output += fmt.Sprintf(" version_mismatch=config:%s trace:%s", pc.Harness.Version, pc.TraceVersion)
	}
	if pc.TestOutput != "" {
		output += "\n" + pc.TestOutput
	}
	return core.Result{
		CommandName: c.Name(),
		Signal:      SigResultsCollected,
		Output:      output,
		Cost:        core.Cost{TokensIn: pc.Tokens},
	}
}

// recordPointFailureCmd projects the failed command result into point metadata.
// The following collect_metrics word remains the sole meta.json writer.
type recordPointFailureCmd struct {
	pc    *PointContext
	prior core.Result
}

func (c *recordPointFailureCmd) Name() string { return "record_point_failure" }
func (c *recordPointFailureCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, point: c.pc}).Undo(prior)
}

func (c *recordPointFailureCmd) Execute() core.Result {
	c.pc.FailureStage = c.prior.CommandName
	if c.pc.FailureStage == "" {
		c.pc.FailureStage = "unknown"
	}
	if c.prior.Err != nil {
		c.pc.FailureCause = c.prior.Err.Error()
	} else {
		c.pc.FailureCause = c.prior.Output
	}
	if c.pc.FailureCause == "" {
		c.pc.FailureCause = string(c.prior.Signal)
	}
	return pointToolDone(c.Name(), SigFailureRecorded, c.pc.FailureStage+": "+c.pc.FailureCause)
}

// collectMetricsCmd writes the meta.json file for the evaluation point.
type collectMetricsCmd struct {
	pc *PointContext
}

func (c *collectMetricsCmd) Name() string { return "collect_metrics" }
func (c *collectMetricsCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, point: c.pc}).Undo(prior)
}

func (c *collectMetricsCmd) Execute() core.Result {
	pc := c.pc

	metaJSON, err := writeMetaJSON(pc)
	if err != nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         err,
		}
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigMetricsCollected,
		Output:      string(metaJSON),
	}
}
