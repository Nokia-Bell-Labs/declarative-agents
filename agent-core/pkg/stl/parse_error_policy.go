// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// ParseErrorRetryTracker tracks consecutive parse failures for grammars that
// route ParseFailed through an explicit report_parse_error word.
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
	if p == nil {
		return
	}
	p.consecutive = consecutive
}

// RecordParseResult resets the parse failure count once parsing produces a
// non-ParseFailed signal.
func (p *ParseErrorRetryTracker) RecordParseResult(sig core.Signal) {
	if p == nil {
		return
	}
	if sig != core.ParseFailed {
		p.consecutive = 0
	}
}

// ReportParseError records one grammar-visible parse error report and returns
// the signal that report_parse_error should emit.
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

// ParseErrorPolicy returns an AfterDispatch hook that counts
// consecutive ParseFailed signals and returns BudgetExhausted when
// the limit is reached. Signals from report_parse_error and
// invoke_llm do not reset the counter (they are part of the
// parse→retry cycle). Prefer ParseErrorRetryTracker for machines that
// route ParseFailed through report_parse_error; this hook remains as a
// compatibility fallback for machines without that explicit word.
func ParseErrorPolicy(maxConsecutive int) func(core.Command, core.Result) core.Signal {
	var consecutive int
	return func(cmd core.Command, res core.Result) core.Signal {
		sig := res.Signal
		if sig == core.ParseFailed {
			consecutive++
		} else if sig != core.ToolDone || (cmd.Name() != "report_parse_error" && cmd.Name() != "invoke_llm") {
			consecutive = 0
		}
		if maxConsecutive > 0 && consecutive >= maxConsecutive {
			return core.BudgetExhausted
		}
		return ""
	}
}
