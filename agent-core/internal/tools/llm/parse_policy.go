// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"

// ParseErrorRetryTracker tracks consecutive parse failures.
type ParseErrorRetryTracker struct {
	MaxConsecutive int
	consecutive    int
}

// Snapshot returns the current consecutive parse failure count.
func (p *ParseErrorRetryTracker) Snapshot() int {
	if p == nil {
		return 0
	}
	return p.consecutive
}

// Restore resets the current consecutive parse failure count.
func (p *ParseErrorRetryTracker) Restore(consecutive int) {
	if p != nil {
		p.consecutive = consecutive
	}
}

// RecordParseResult resets the count once parsing succeeds or completes.
func (p *ParseErrorRetryTracker) RecordParseResult(sig core.Signal) {
	if p != nil && sig != core.ParseFailed {
		p.consecutive = 0
	}
}

// ReportParseError records one grammar-visible parse error report.
func (p *ParseErrorRetryTracker) ReportParseError() core.Signal {
	if p == nil {
		return core.ToolDone
	}
	p.consecutive++
	if p.MaxConsecutive > 0 && p.consecutive >= p.MaxConsecutive {
		return core.BudgetExhausted
	}
	return core.ToolDone
}

// ParseErrorPolicy returns an AfterDispatch hook for legacy parse retry loops.
func ParseErrorPolicy(maxConsecutive int) func(core.Command, core.Result) core.Signal {
	var consecutive int
	return func(cmd core.Command, res core.Result) core.Signal {
		if res.Signal == core.ParseFailed {
			consecutive++
		} else if res.Signal != core.ToolDone || (cmd.Name() != "report_parse_error" && cmd.Name() != "invoke_llm") {
			consecutive = 0
		}
		if maxConsecutive > 0 && consecutive >= maxConsecutive {
			return core.BudgetExhausted
		}
		return ""
	}
}
