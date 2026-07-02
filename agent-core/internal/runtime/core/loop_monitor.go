// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"context"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
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
