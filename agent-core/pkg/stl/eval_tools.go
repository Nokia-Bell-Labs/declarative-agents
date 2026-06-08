// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

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

