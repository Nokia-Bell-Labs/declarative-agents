// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
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
	pc *PointContext
}

func (c *createPointDirCmd) Name() string      { return "create_point_dir" }
func (c *createPointDirCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *createPointDirCmd) Execute() core.Result {
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

// checkResultsCmd runs the oracle test and parses token usage from the trace.
type checkResultsCmd struct {
	pc *PointContext
}

func (c *checkResultsCmd) Name() string      { return "check_results" }
func (c *checkResultsCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *checkResultsCmd) Execute() core.Result {
	pc := c.pc

	pc.TestsPassed, pc.TestOutput = runOracleCheck(pc.PointDir)

	if _, err := os.Stat(pc.TracePath); err == nil {
		if spans, parseErr := ReadTraceFile(pc.TracePath); parseErr == nil {
			for _, s := range spans {
				pc.Tokens += IntAttr(s, "gen_ai.usage.input_tokens")
				pc.Tokens += IntAttr(s, "gen_ai.usage.output_tokens")
			}
			if pc.Harness.Version != "" {
				if traceVer := AgentVersion(spans); traceVer != "" && traceVer != pc.Harness.Version {
					fmt.Fprintf(pc.Stderr, "  WARN: version mismatch: config=%s trace=%s\n",
						pc.Harness.Version, traceVer)
				}
			}
		}
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigResultsCollected,
		Output:      pc.TestOutput,
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
