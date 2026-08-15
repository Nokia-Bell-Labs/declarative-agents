// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pipeline

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/planning/plan"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

// DoParsePlan calls plan.ParsePlan and returns the parsed plan plus a
// core.Result with SigPlanReady on success or core.ParseFailed on error.
// Used by both pipeline and apply state machines.
func DoParsePlan(cmdName, rawResp string) (plan.ImplementationPlan, core.Result) {
	p, err := plan.ParsePlan(rawResp)
	if err != nil {
		return plan.ImplementationPlan{}, core.Result{
			CommandName: cmdName,
			Signal:      core.ParseFailed,
			Output:      err.Error(),
		}
	}
	return p, core.Result{
		CommandName: cmdName,
		Signal:      SigPlanReady,
		Output:      fmt.Sprintf("parsed plan: %s (%d files, %d requirements)", p.Title, len(p.Files), len(p.Requirements)),
	}
}
