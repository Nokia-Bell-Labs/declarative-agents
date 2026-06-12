// Copyright (c) 2026 Nokia. All rights reserved.

package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/telemetry"
)

func TestPropagateArgs_Build_Full(t *testing.T) {
	sc, err := telemetry.ParseTraceparent("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	require.NoError(t, err)

	p := PropagateArgs{
		SpanContext: sc,
		TraceFile:   "/tmp/trace.json",
		Model:       "qwen3.6:35b-mlx",
		Directory:   "/tmp/workspace",
		Prompt:      "task.yaml",
		OllamaURL:   "http://localhost:11434",
		MaxTime:     5 * time.Minute,
		LLMTimeout:  2 * time.Minute,
	}

	args := p.Build()

	assert.Contains(t, args, "--otel-parent-span")
	assert.Contains(t, args, "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	assert.Contains(t, args, "--otel-log-file")
	assert.Contains(t, args, "/tmp/trace.json")
	assert.Contains(t, args, "--model")
	assert.Contains(t, args, "qwen3.6:35b-mlx")
	assert.Contains(t, args, "--directory")
	assert.Contains(t, args, "/tmp/workspace")
	assert.Contains(t, args, "--prompt")
	assert.Contains(t, args, "task.yaml")
	assert.Contains(t, args, "--max-time")
	assert.Contains(t, args, "5m0s")
	assert.Contains(t, args, "--llm-timeout")
	assert.Contains(t, args, "2m0s")
}

func TestPropagateArgs_Build_Minimal(t *testing.T) {
	p := PropagateArgs{
		TraceFile: "/tmp/trace.json",
		Model:     "llama3",
		Directory: "/tmp/ws",
	}

	args := p.Build()

	assert.NotContains(t, args, "--otel-parent-span")
	assert.NotContains(t, args, "--max-time")
	assert.NotContains(t, args, "--llm-timeout")
	assert.NotContains(t, args, "--prompt")
	assert.Contains(t, args, "--otel-log-file")
	assert.Contains(t, args, "--model")
	assert.Contains(t, args, "--directory")
}

func TestPropagateArgs_Build_InvalidSpanContext(t *testing.T) {
	p := PropagateArgs{
		SpanContext: trace.SpanContext{},
		Model:       "test",
	}
	args := p.Build()
	assert.NotContains(t, args, "--otel-parent-span")
}

func TestRemainingBudget(t *testing.T) {
	tests := []struct {
		name    string
		total   time.Duration
		elapsed time.Duration
		want    time.Duration
	}{
		{"unlimited", 0, 5 * time.Minute, 0},
		{"plenty left", 10 * time.Minute, 3 * time.Minute, 7 * time.Minute},
		{"exhausted", 10 * time.Minute, 12 * time.Minute, 0},
		{"exact", 10 * time.Minute, 10 * time.Minute, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RemainingBudget(tt.total, tt.elapsed)
			assert.Equal(t, tt.want, got)
		})
	}
}
