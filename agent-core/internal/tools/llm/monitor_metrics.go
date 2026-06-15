// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// SetMonitorRecorder connects invoke_llm to the embedded monitor recorder.
func (c *invokeLLMCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	c.recorder = rec
}

func (c *invokeLLMCmd) recordTokenMetrics(cost core.Cost) {
	values := map[string]float64{}
	if cost.TokensIn > 0 {
		values["prompt_tokens"] = float64(cost.TokensIn)
	}
	if cost.TokensOut > 0 {
		values["completion_tokens"] = float64(cost.TokensOut)
	}
	core.RecordDeclaredToolMetrics(c.ctx, c.recorder, c.Name(), c.metrics, values, map[string]string{
		"provider": c.providerName,
		"model":    c.model,
	})
}
