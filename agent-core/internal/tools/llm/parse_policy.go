// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"

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
