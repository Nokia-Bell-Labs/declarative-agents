// Copyright (c) 2026 Nokia. All rights reserved.

package cli

import (
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/telemetry"
)

// Standard flag names used across all agent CLIs.
const (
	FlagOTelLogFile   = "otel-log-file"
	FlagOTelParent    = "otel-parent-span"
	FlagModel         = "model"
	FlagDirectory     = "directory"
	FlagMaxTime       = "max-time"
	FlagLLMTimeout    = "llm-timeout"
	FlagPrompt        = "prompt"
	FlagOllamaURL     = "ollama-url"
)

// PropagateArgs builds CLI flags for invoking a child agent process.
// It serialises the parent's current span as a traceparent and computes
// the remaining time budget so the child can manage its own shutdown.
type PropagateArgs struct {
	SpanContext  trace.SpanContext // current span to propagate
	TraceFile    string           // --otel-log-file for child
	Model        string
	Directory    string
	Prompt       string
	OllamaURL    string
	MaxTime      time.Duration // remaining wall-clock budget; 0 = omit
	LLMTimeout   time.Duration // per-LLM-call timeout; 0 = omit
}

// Build returns the flag slice ready for exec.Command.
func (p PropagateArgs) Build() []string {
	var args []string

	if tp := telemetry.FormatTraceparent(p.SpanContext); tp != "" {
		args = append(args, "--"+FlagOTelParent, tp)
	}
	if p.TraceFile != "" {
		args = append(args, "--"+FlagOTelLogFile, p.TraceFile)
	}
	if p.Model != "" {
		args = append(args, "--"+FlagModel, p.Model)
	}
	if p.Directory != "" {
		args = append(args, "--"+FlagDirectory, p.Directory)
	}
	if p.Prompt != "" {
		args = append(args, "--"+FlagPrompt, p.Prompt)
	}
	if p.OllamaURL != "" {
		args = append(args, "--"+FlagOllamaURL, p.OllamaURL)
	}
	if p.MaxTime > 0 {
		args = append(args, "--"+FlagMaxTime, p.MaxTime.String())
	}
	if p.LLMTimeout > 0 {
		args = append(args, "--"+FlagLLMTimeout, p.LLMTimeout.String())
	}

	return args
}

// RemainingBudget computes the time left from a total budget and elapsed
// duration. Returns 0 if the budget is zero (unlimited) or already
// exhausted.
func RemainingBudget(total, elapsed time.Duration) time.Duration {
	if total <= 0 {
		return 0
	}
	remaining := total - elapsed
	if remaining <= 0 {
		return 0
	}
	return remaining
}

// FormatFlag returns "--name value" as two strings for appending to
// an args slice. Convenience for ad-hoc flags not in PropagateArgs.
func FormatFlag(name, value string) []string {
	return []string{fmt.Sprintf("--%s", name), value}
}
