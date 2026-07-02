// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"context"
	"testing"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/prompt"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

func TestInvokeLLMMetricsCarryDispatchEnvelope(t *testing.T) {
	t.Parallel()
	cmd := &invokeLLMCmd{
		client: metricClient{}, history: modelllm.NewConversation(nil, "", modelllm.ChatOptions{}),
		registry: core.NewRegistry(), assembler: metricAssembler{}, model: "qwen", providerName: "ollama",
		userMessage: "request", tracer: tracing.NoopTracer{}, ctx: context.Background(), metrics: llmMetrics(),
	}

	samples := runLLMMetricLoop(t, cmd, core.LLMResponded)

	requireMetric(t, samples, "llm.prompt_tokens", 11)
	requireLLMEnvelope(t, samples, "llm.prompt_tokens", cmd.Name())
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

func runLLMMetricLoop(t *testing.T, cmd core.Command, signal core.Signal) []monitor.MetricSample {
	t.Helper()
	// Keep this fixture package-local so LLM assertions name model-boundary commands and signals.
	store := monitor.NewStore(monitor.Limits{Samples: 10})
	params := llmMetricLoopParams(cmd, signal, monitor.NewRecorder(store, nil))
	_, err := core.Loop(params, context.Background())
	if err != nil {
		t.Fatalf("loop failed: %v", err)
	}
	return store.Snapshot().RecentSamples
}

func llmMetricLoopParams(cmd core.Command, signal core.Signal, rec monitor.RuntimeRecorder) core.LoopParams {
	spec := &core.MachineSpec{
		Name:           "llm-metrics",
		InitialState:   "Start",
		MetricLabels:   core.MetricLabels{"use_case": "rel04.0-monitor"},
		States:         core.StateSpecsFromNames("Start", "Working", "Done"),
		TerminalStates: []string{"Done"},
		Signals:        core.SignalSpecsFromNames(string(core.Seed), string(signal)),
		Transitions: []core.TransitionSpec{
			{State: "Start", Signal: string(core.Seed), Next: "Working", Action: cmd.Name(), MetricLabels: core.MetricLabels{"phase": "dispatch"}},
			{State: "Working", Signal: string(signal), Next: "Done"},
		},
	}
	return core.LoopParams{
		MachineSpec:     spec,
		AgentName:       "llm-run",
		Trace:           tracing.NoopTracer{},
		Budget:          core.Budget{MaxIterations: 3},
		MonitorRecorder: rec,
		InitFunc: func(reg *core.Registry) error {
			reg.Register(core.ToolSpec{Name: cmd.Name(), Visibility: core.Internal}, llmMetricBuilder{cmd: cmd})
			return nil
		},
		Hooks: core.LoopHooks{TerminalStatus: func(core.State) core.RunStatus { return core.StatusSucceeded }},
	}
}

type llmMetricBuilder struct {
	cmd core.Command
}

func (b llmMetricBuilder) Build(core.Result) core.Command {
	return b.cmd
}

func requireLLMEnvelope(t *testing.T, samples []monitor.MetricSample, name string, toolName string) {
	t.Helper()
	for _, sample := range samples {
		if sample.Name != name {
			continue
		}
		if sample.ToolName != toolName || sample.RunID != "llm-run" ||
			sample.State != "Working" || sample.Signal != string(core.LLMResponded) ||
			sample.Status != "success" || sample.Attributes["use_case"] != "rel04.0-monitor" ||
			sample.Attributes["phase"] != "dispatch" {
			t.Fatalf("bad metric envelope: %#v", sample)
		}
		return
	}
	t.Fatalf("missing metric %s in %#v", name, samples)
}
