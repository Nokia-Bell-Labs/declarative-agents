// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
)

func recordMonitorEvent(ctx context.Context, rec monitor.RuntimeRecorder, event RunEvent) {
	if rec == nil {
		return
	}
	_ = rec.RecordEvent(ctx, monitor.RunEvent{
		Iteration:   event.Iteration,
		Timestamp:   event.Timestamp,
		CommandName: event.CommandName,
		Signal:      string(event.Signal),
		FromState:   string(event.FromState),
		ToState:     string(event.ToState),
		Duration:    event.Cost.Duration,
		TokensIn:    event.Cost.TokensIn,
		TokensOut:   event.Cost.TokensOut,
	})
}

func recordMonitorRun(ctx context.Context, rec monitor.RuntimeRecorder, run monitor.RunSnapshot) {
	if rec == nil {
		return
	}
	_ = rec.RecordRun(ctx, run)
}

// observeCommandState refreshes the live command-state source with the current
// execution log after each dispatch, so a background monitor server reads
// completed steps rather than a launch-time snapshot (srd033 R7.1).
func (r *loopRunner) observeCommandState() {
	if r.params.CommandStateObserver == nil {
		return
	}
	r.params.CommandStateObserver.ObserveCommandState(r.execution)
}
