// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// SetMonitorRecorder connects invoke_llm to the embedded monitor recorder.
func (c *invokeLLMCmd) SetMonitorRecorder(rec monitor.ToolMetricsRecorder) {
	c.recorder = rec
}

func (c *invokeLLMCmd) recordTokenMetrics(cost core.Cost) {
	if cost.TokensIn > 0 {
		c.recordMetric("llm.prompt_tokens", float64(cost.TokensIn), "Prompt tokens returned by the LLM provider.")
	}
	if cost.TokensOut > 0 {
		c.recordMetric("llm.completion_tokens", float64(cost.TokensOut), "Completion tokens returned by the LLM provider.")
	}
}

func (c *invokeLLMCmd) recordMetric(name string, value float64, description string) {
	if c.recorder == nil {
		return
	}
	sample := monitor.MetricSample{
		Name: name, Kind: monitor.InstrumentHistogram, Unit: "1",
		Description: description, Value: value, ToolName: c.Name(),
		Attributes: map[string]string{
			"provider": c.providerName,
			"model":    c.model,
		},
		Timestamp: time.Now(),
	}
	// Monitoring is observational; recorder failures must not change tool output.
	_ = c.recorder.RecordMetric(c.ctx, sample)
}
