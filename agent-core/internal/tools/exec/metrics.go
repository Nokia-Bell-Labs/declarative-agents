// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package exec

import (
	"context"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func (c *ExecCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	c.rec = rec
}

func (c *ExecCmd) recordExecMetrics(
	duration time.Duration, outputBytes, exitCode int,
) {
	if c.rec == nil {
		return
	}
	values := map[string]float64{
		"process_duration": float64(duration.Milliseconds()),
		"output_bytes":     float64(outputBytes),
		"exit_code":        float64(exitCode),
	}
	core.RecordDeclaredToolMetrics(
		context.Background(), c.rec, c.Name(), c.def.Metrics, values,
		map[string]string{"binary": c.def.Binary},
	)
}
