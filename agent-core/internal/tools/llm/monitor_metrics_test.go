// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"testing"

	modelllm "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/llm"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/model/prompt"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/monitor"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

type metricRecorder struct {
	samples []monitor.MetricSample
}

func (r *metricRecorder) RecordMetric(_ context.Context, sample monitor.MetricSample) error {
	r.samples = append(r.samples, sample)
	return nil
}

type metricClient struct{}

func (metricClient) Chat(context.Context, []modelllm.Message, modelllm.ChatOptions) (modelllm.ChatResponse, error) {
	return modelllm.ChatResponse{Content: "ok", TokensIn: 11, TokensOut: 7}, nil
}

func (metricClient) ListModels(context.Context) ([]modelllm.ModelInfo, error) { return nil, nil }

type metricAssembler struct{}

func (metricAssembler) AssembleMessages(conv *modelllm.Conversation, _ *core.Registry, _ core.State) []modelllm.Message {
	return conv.Messages()
}

func TestInvokeLLMRecordsTokenMetrics(t *testing.T) {
	t.Parallel()
	rec := &metricRecorder{}
	cmd := &invokeLLMCmd{
		client: metricClient{}, history: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		registry: core.NewRegistry(), assembler: metricAssembler{}, model: "qwen", providerName: "ollama",
		userMessage: "request", tracer: tracing.NoopTracer{}, ctx: context.Background(), metrics: llmMetrics(),
	}
	cmd.SetMonitorRecorder(rec)

	res := cmd.Execute()

	if res.Signal != core.LLMResponded {
		t.Fatalf("signal = %s", res.Signal)
	}
	requireMetric(t, rec.samples, "llm.prompt_tokens", 11)
	requireMetric(t, rec.samples, "llm.completion_tokens", 7)
	for _, sample := range rec.samples {
		if sample.Attributes["provider"] != "ollama" || sample.Attributes["model"] != "qwen" {
			t.Fatalf("unsafe or missing attrs: %#v", sample.Attributes)
		}
	}
}

func TestInvokeLLMSkipsUndeclaredTokenMetric(t *testing.T) {
	t.Parallel()
	rec := &metricRecorder{}
	cmd := &invokeLLMCmd{
		client: metricClient{}, history: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		registry: core.NewRegistry(), assembler: metricAssembler{}, model: "qwen", providerName: "ollama",
		userMessage: "request", tracer: tracing.NoopTracer{}, ctx: context.Background(),
		metrics: core.MetricConfig{Instruments: []core.MetricInstrument{{
			Name: "llm.prompt_tokens", Kind: "histogram", Unit: "1",
			Description: "Prompt tokens.", ValueSource: "prompt_tokens",
		}}},
	}
	cmd.SetMonitorRecorder(rec)

	res := cmd.Execute()

	if res.Signal != core.LLMResponded {
		t.Fatalf("signal = %s", res.Signal)
	}
	requireMetric(t, rec.samples, "llm.prompt_tokens", 11)
	requireMissingMetric(t, rec.samples, "llm.completion_tokens")
}

func (metricAssembler) EnvelopeConfig() (*prompt.Envelope, bool) { return nil, false }

func llmMetrics() core.MetricConfig {
	return core.MetricConfig{
		Instruments: []core.MetricInstrument{
			{Name: "llm.prompt_tokens", Kind: "histogram", Unit: "1", Description: "Prompt tokens.", ValueSource: "prompt_tokens", Attributes: []string{"provider", "model"}},
			{Name: "llm.completion_tokens", Kind: "histogram", Unit: "1", Description: "Completion tokens.", ValueSource: "completion_tokens", Attributes: []string{"provider", "model"}},
		},
		Attributes: []core.MetricAttribute{
			{Name: "provider", Source: "config_literal", Cardinality: "low", Redaction: "none"},
			{Name: "model", Source: "config_literal", Cardinality: "low", Redaction: "none"},
		},
	}
}

func requireMetric(t *testing.T, samples []monitor.MetricSample, name string, value float64) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name && sample.Value == value {
			return
		}
	}
	t.Fatalf("missing metric %s=%v in %#v", name, value, samples)
}

func requireMissingMetric(t *testing.T, samples []monitor.MetricSample, name string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name == name {
			t.Fatalf("unexpected metric %s in %#v", name, samples)
		}
	}
}
