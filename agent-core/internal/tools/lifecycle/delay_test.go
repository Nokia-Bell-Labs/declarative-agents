// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestDelay_ElapsedAndDeadlineSignals(t *testing.T) {
	t.Parallel()

	builder, err := newDelayBuilder("retry_delay", DelayConfig{
		Duration:       "10ms",
		Deadline:       "1s",
		DeadlineSource: "$from(subject).started_at",
		ElapsedSignal:  "RetryReady",
		DeadlineSignal: "HealthTimeout",
	})
	require.NoError(t, err)

	cmd := builder.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(delayView(time.Now()))
	elapsed := cmd.Execute()
	require.Equal(t, core.Signal("RetryReady"), elapsed.Signal, elapsed.Output)

	cmd = builder.Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(delayView(time.Now().Add(-2 * time.Second)))
	expired := cmd.Execute()
	require.Equal(t, core.Signal("HealthTimeout"), expired.Signal, expired.Output)
	require.Contains(t, expired.Output, `"deadline_reached":true`)
}

func TestDelay_CancellationAndValidation(t *testing.T) {
	t.Parallel()

	builder, err := newDelayBuilder("delay", DelayConfig{Duration: "1s"})
	require.NoError(t, err)
	cmd := builder.Build(core.Result{}).(*delayCommand)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := cmd.ExecuteContext(ctx)
	require.Equal(t, core.CommandError, result.Signal)
	require.ErrorIs(t, result.Err, context.Canceled)

	_, err = newDelayBuilder("delay", DelayConfig{Duration: "0s"})
	require.ErrorContains(t, err, "must be positive")
	_, err = newDelayBuilder("delay", DelayConfig{
		Duration: "1ms", Deadline: "1s", DeadlineSource: "started_at",
	})
	require.ErrorContains(t, err, "deadline_source must be")
}

func delayView(start time.Time) core.CommandStateView {
	return core.NewCommandStateView(core.Execution{{
		CommandName: "subject",
		Result: core.ResultDigest{
			Output:           `{"started_at":"` + start.UTC().Format(time.RFC3339Nano) + `"}`,
			RedactionVersion: core.OutputRedactionVersion1,
			RedactionStatus:  core.OutputRedactionApplied,
		},
	}})
}
