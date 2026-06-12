// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"context"
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/materialize"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/planning/plan"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
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

// DoMaterialize creates a bd issue from an implementation plan.
// Returns the issue ID and a core.Result with SigMaterialized on
// success or core.CommandError on failure. Used by both pipeline
// and apply state machines.
func DoMaterialize(ctx context.Context, tracer tracing.Tracer, p plan.ImplementationPlan, dir string, deps map[string]string, cmdName string) (string, core.Result) {
	m := materialize.NewMaterializeTask()
	issueID, err := m.Execute(ctx, tracer, p, dir, deps)
	if err != nil {
		return "", core.Result{
			CommandName: cmdName,
			Signal:      core.CommandError,
			Err:         err,
			Output:      err.Error(),
		}
	}
	return issueID, core.Result{
		CommandName: cmdName,
		Signal:      SigMaterialized,
		Output:      fmt.Sprintf("created issue %s for plan %q", issueID, p.Title),
	}
}
