// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// SigWorkspaceReady is emitted when workspace preparation completes.
const SigWorkspaceReady core.Signal = "WorkspaceReady"

// prepareWorkspaceCmd copies the sample workspace, doc dir, and
// initialises a git repo with a baseline commit.
type prepareWorkspaceCmd struct {
	pc *PointContext
}

func (c *prepareWorkspaceCmd) Name() string { return "prepare_workspace" }

func (c *prepareWorkspaceCmd) Execute() core.Result {
	pc := c.pc

	pointDir := filepath.Join(pc.SessionDir, pc.PointID)
	if err := os.MkdirAll(pointDir, 0o755); err != nil {
		return c.fail(fmt.Errorf("mkdir point dir: %w", err))
	}
	pc.PointDir = pointDir
	pc.TracePath = filepath.Join(pointDir, ArtifactTrace)

	if err := copyDir(pc.Sample.WorkspaceDir, pointDir); err != nil {
		return c.fail(fmt.Errorf("copy workspace: %w", err))
	}

	if pc.Sample.DocDir != "" {
		dst := filepath.Join(pointDir, ArtifactDocDir)
		if err := copyDir(pc.Sample.DocDir, dst); err != nil {
			return c.fail(fmt.Errorf("copy docs: %w", err))
		}
	}

	for _, cmd := range [][]string{
		{"git", "init"},
		{"git", "add", "-A"},
		{"git", "commit", "-m", "baseline", "--allow-empty"},
	} {
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Dir = pointDir
		if out, err := c.CombinedOutput(); err != nil {
			return (&prepareWorkspaceCmd{pc: pc}).fail(
				fmt.Errorf("%s: %s: %w", cmd[0], string(out), err))
		}
	}

	return core.Result{
		CommandName: "prepare_workspace",
		Signal:      SigWorkspaceReady,
		Output:      fmt.Sprintf("workspace ready at %s", pointDir),
	}
}

func (c *prepareWorkspaceCmd) fail(err error) core.Result {
	return core.Result{
		CommandName: "prepare_workspace",
		Signal:      core.CommandError,
		Err:         err,
		Output:      err.Error(),
	}
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

func (c *checkResultsCmd) Name() string { return "check_results" }

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

func (c *collectMetricsCmd) Name() string { return "collect_metrics" }

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

