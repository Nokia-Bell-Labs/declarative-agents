// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// Signals emitted by atomic evaluator workspace preparation commands.
const (
	SigPointDirCreated            core.Signal = "PointDirCreated"
	SigSampleWorkspaceCopied      core.Signal = "SampleWorkspaceCopied"
	SigSampleDocsCopied           core.Signal = "SampleDocsCopied"
	SigWorkspaceRepoInitialized   core.Signal = "WorkspaceRepoInitialized"
	SigWorkspaceBaselineStaged    core.Signal = "WorkspaceBaselineStaged"
	SigWorkspaceBaselineCommitted core.Signal = "WorkspaceBaselineCommitted"
)

// createPointDirCmd creates the per-point directory and records paths that
// later point tools consume.
type createPointDirCmd struct {
	pc          *PointContext
	snapshot    pointContextSnapshot
	hasSnapshot bool
}

func (c *createPointDirCmd) Name() string { return "create_point_dir" }
func (c *createPointDirCmd) Undo() core.Result {
	return undoPointContextSnapshot(c.Name(), c.pc, c.snapshot, c.hasSnapshot)
}
func (c *createPointDirCmd) UndoMemento() (core.UndoMemento, error) {
	return pointContextMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *createPointDirCmd) Execute() core.Result {
	c.snapshot = snapshotPointContext(c.pc)
	c.hasSnapshot = true
	pointDir := filepath.Join(c.pc.SessionDir, c.pc.PointID)
	if err := os.MkdirAll(pointDir, 0o755); err != nil {
		return pointToolError(c.Name(), fmt.Errorf("mkdir point dir: %w", err))
	}
	c.pc.PointDir = pointDir
	c.pc.TracePath = filepath.Join(pointDir, ArtifactTrace)
	return pointToolDone(c.Name(), SigPointDirCreated, fmt.Sprintf("point dir created at %s", pointDir))
}

// copySampleWorkspaceCmd copies only the sample workspace into the point dir.
type copySampleWorkspaceCmd struct {
	pc *PointContext
}

func (c *copySampleWorkspaceCmd) Name() string      { return "copy_sample_workspace" }
func (c *copySampleWorkspaceCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *copySampleWorkspaceCmd) Execute() core.Result {
	if err := requirePointDir(c.pc); err != nil {
		return pointToolError(c.Name(), err)
	}
	if err := copyDir(c.pc.Sample.WorkspaceDir, c.pc.PointDir); err != nil {
		return pointToolError(c.Name(), fmt.Errorf("copy workspace: %w", err))
	}
	return pointToolDone(c.Name(), SigSampleWorkspaceCopied, fmt.Sprintf("sample workspace copied to %s", c.pc.PointDir))
}

// copySampleDocsCmd copies optional sample docs into the point dir. Samples
// without docs still complete successfully so the machine sequence remains fixed.
type copySampleDocsCmd struct {
	pc *PointContext
}

func (c *copySampleDocsCmd) Name() string      { return "copy_sample_docs" }
func (c *copySampleDocsCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *copySampleDocsCmd) Execute() core.Result {
	if err := requirePointDir(c.pc); err != nil {
		return pointToolError(c.Name(), err)
	}
	if c.pc.Sample.DocDir == "" {
		return pointToolDone(c.Name(), SigSampleDocsCopied, "sample has no docs")
	}
	dst := filepath.Join(c.pc.PointDir, ArtifactDocDir)
	if err := copyDir(c.pc.Sample.DocDir, dst); err != nil {
		return pointToolError(c.Name(), fmt.Errorf("copy docs: %w", err))
	}
	return pointToolDone(c.Name(), SigSampleDocsCopied, fmt.Sprintf("sample docs copied to %s", dst))
}

type initWorkspaceRepoCmd struct {
	pc *PointContext
}

func (c *initWorkspaceRepoCmd) Name() string      { return "init_workspace_repo" }
func (c *initWorkspaceRepoCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *initWorkspaceRepoCmd) Execute() core.Result {
	if err := requirePointDir(c.pc); err != nil {
		return pointToolError(c.Name(), err)
	}
	if err := runPointGit(c.pc.PointDir, "init"); err != nil {
		return pointToolError(c.Name(), err)
	}
	return pointToolDone(c.Name(), SigWorkspaceRepoInitialized, "workspace git repo initialized")
}

type stageWorkspaceBaselineCmd struct {
	pc *PointContext
}

func (c *stageWorkspaceBaselineCmd) Name() string      { return "stage_workspace_baseline" }
func (c *stageWorkspaceBaselineCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *stageWorkspaceBaselineCmd) Execute() core.Result {
	if err := requirePointDir(c.pc); err != nil {
		return pointToolError(c.Name(), err)
	}
	if err := runPointGit(c.pc.PointDir, "add", "-A"); err != nil {
		return pointToolError(c.Name(), err)
	}
	return pointToolDone(c.Name(), SigWorkspaceBaselineStaged, "workspace baseline staged")
}

type commitWorkspaceBaselineCmd struct {
	pc *PointContext
}

func (c *commitWorkspaceBaselineCmd) Name() string      { return "commit_workspace_baseline" }
func (c *commitWorkspaceBaselineCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *commitWorkspaceBaselineCmd) Execute() core.Result {
	if err := requirePointDir(c.pc); err != nil {
		return pointToolError(c.Name(), err)
	}
	if err := runPointGit(c.pc.PointDir, "-c", "user.name=agent-core", "-c", "user.email=agent-core@example.invalid", "commit", "-m", "baseline", "--allow-empty"); err != nil {
		return pointToolError(c.Name(), err)
	}
	return pointToolDone(c.Name(), SigWorkspaceBaselineCommitted, "workspace baseline committed")
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

func runPointGit(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %v: %s: %w", args, string(out), err)
	}
	return nil
}

func copyDir(src, dst string) error {
	cmd := exec.Command("cp", "-a", src+"/.", dst)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("cp -a %s/. %s: %s: %w", src, dst, string(out), err)
	}
	return nil
}

// runOracleCheckCmd runs the sample's oracle tests and records pass/fail output.
type runOracleCheckCmd struct {
	pc          *PointContext
	snapshot    pointContextSnapshot
	hasSnapshot bool
}

func (c *runOracleCheckCmd) Name() string { return "run_oracle_check" }
func (c *runOracleCheckCmd) Undo() core.Result {
	return undoPointContextSnapshot(c.Name(), c.pc, c.snapshot, c.hasSnapshot)
}
func (c *runOracleCheckCmd) UndoMemento() (core.UndoMemento, error) {
	return pointContextMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *runOracleCheckCmd) Execute() core.Result {
	pc := c.pc
	c.snapshot = snapshotPointContext(pc)
	c.hasSnapshot = true
	pc.TestsPassed, pc.TestOutput = runOracleCheck(pc.PointDir)

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
	pc          *PointContext
	snapshot    pointContextSnapshot
	hasSnapshot bool
}

func (c *collectTraceTokensCmd) Name() string { return "collect_trace_tokens" }
func (c *collectTraceTokensCmd) Undo() core.Result {
	return undoPointContextSnapshot(c.Name(), c.pc, c.snapshot, c.hasSnapshot)
}
func (c *collectTraceTokensCmd) UndoMemento() (core.UndoMemento, error) {
	return pointContextMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *collectTraceTokensCmd) Execute() core.Result {
	pc := c.pc
	c.snapshot = snapshotPointContext(pc)
	c.hasSnapshot = true
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
	pc          *PointContext
	snapshot    pointContextSnapshot
	hasSnapshot bool
}

func (c *checkAgentVersionCmd) Name() string { return "check_agent_version" }
func (c *checkAgentVersionCmd) Undo() core.Result {
	return undoPointContextSnapshot(c.Name(), c.pc, c.snapshot, c.hasSnapshot)
}
func (c *checkAgentVersionCmd) UndoMemento() (core.UndoMemento, error) {
	return pointContextMemento(c.Name(), c.snapshot, c.hasSnapshot)
}

func (c *checkAgentVersionCmd) Execute() core.Result {
	pc := c.pc
	c.snapshot = snapshotPointContext(pc)
	c.hasSnapshot = true
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
			fmt.Fprintf(pc.Stderr, "  WARN: %s\n", msg)
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

func (c *summarizePointResultsCmd) Name() string      { return "summarize_point_results" }
func (c *summarizePointResultsCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

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

// collectMetricsCmd writes the meta.json file for the evaluation point.
type collectMetricsCmd struct {
	pc *PointContext
}

func (c *collectMetricsCmd) Name() string      { return "collect_metrics" }
func (c *collectMetricsCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

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
