// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// prepareWorkspaceCmd creates the point directory, copies the sample workspace,
// and initializes git.
type prepareWorkspaceCmd struct {
	pc *PointContext
}

func (c *prepareWorkspaceCmd) Name() string { return "prepare_workspace" }

func (c *prepareWorkspaceCmd) Execute() core.Result {
	pc := c.pc
	pc.PointDir = filepath.Join(pc.SessionDir, pc.PointID)

	if err := os.MkdirAll(pc.PointDir, 0o755); err != nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         fmt.Errorf("create point dir: %w", err),
		}
	}

	if err := copyDirTo(pc.Sample.WorkspaceDir, pc.PointDir); err != nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         fmt.Errorf("copy workspace: %w", err),
		}
	}

	if pc.Sample.DocDir != "" {
		docDst := filepath.Join(pc.PointDir, "doc")
		if err := copyDirTo(pc.Sample.DocDir, docDst); err != nil {
			return core.Result{
				CommandName: c.Name(),
				Signal:      core.CommandError,
				Err:         fmt.Errorf("copy doc dir: %w", err),
			}
		}
	}

	if err := gitInit(context.TODO(), pc.PointDir); err != nil {
		fmt.Fprintf(pc.Stderr, "  WARN: git init failed: %v\n", err)
	}

	pc.TracePath = filepath.Join(pc.PointDir, "trace.json")
	pc.ResultPath = filepath.Join(pc.PointDir, "result.json")

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigWorkspaceReady,
		Output:      pc.PointDir,
	}
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

// summarizeCmd is a no-op placeholder; session-level summary is handled
// outside the per-point state machine.
type summarizeCmd struct {
	pc *PointContext
}

func (c *summarizeCmd) Name() string { return "summarize" }

func (c *summarizeCmd) Execute() core.Result {
	return core.Result{
		CommandName: c.Name(),
		Signal:      SigSummarized,
	}
}
