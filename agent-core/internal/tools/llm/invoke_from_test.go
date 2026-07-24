// Copyright (c) 2026 Nokia. All rights reserved.

package llm

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func viewFrom(entries ...core.Entry) core.CommandStateView {
	for i := range entries {
		entries[i].Result.RedactionVersion = core.OutputRedactionVersion1
		entries[i].Result.RedactionStatus = core.OutputRedactionApplied
	}
	return core.NewCommandStateView(core.Execution(entries))
}

// A $tool-dispatched chat-LLM word receives the router's tool-call JSON as its
// dispatch Output; user_prompt_from redirects the user message to the composed
// prompt a non-adjacent compose word rendered.
func TestResolveUserMessageWholeOutput(t *testing.T) {
	c := &invokeLLMCmd{
		userMessage: `{"tool":"invoke_llm_deep","parameters":{}}`,
		promptFrom:  "$from(compose_prompt)",
		tracer:      tracing.NoopTracer{},
	}
	c.SetCommandState(viewFrom(
		core.Entry{CommandName: "compose_prompt", Result: core.ResultDigest{Output: "Answer using chunk-a and chunk-b."}},
	))
	c.resolveUserMessage()
	require.Equal(t, "Answer using chunk-a and chunk-b.", c.userMessage)
}

func TestResolveUserMessageDottedPath(t *testing.T) {
	c := &invokeLLMCmd{
		userMessage: "ignored",
		promptFrom:  "$from(embed).carried.input",
		tracer:      tracing.NoopTracer{},
	}
	c.SetCommandState(viewFrom(
		core.Entry{CommandName: "embed", Result: core.ResultDigest{Output: `{"carried":{"input":"what is x?"}}`}},
	))
	c.resolveUserMessage()
	require.Equal(t, "what is x?", c.userMessage)
}

func TestResolveUserMessageKeepsFallbackWhenUnset(t *testing.T) {
	c := &invokeLLMCmd{userMessage: "original", tracer: tracing.NoopTracer{}}
	c.resolveUserMessage()
	require.Equal(t, "original", c.userMessage)
}

func TestResolveUserMessageKeepsFallbackOnUnresolved(t *testing.T) {
	c := &invokeLLMCmd{
		userMessage: "fallback",
		promptFrom:  "$from(missing)",
		tracer:      tracing.NoopTracer{},
	}
	c.SetCommandState(viewFrom())
	c.resolveUserMessage()
	require.Equal(t, "fallback", c.userMessage)
}

func TestWholeOutputSelector(t *testing.T) {
	cases := []struct {
		selector string
		label    string
		ok       bool
	}{
		{"$from(compose_prompt)", "compose_prompt", true},
		{"$from(embed).carried.input", "", false},
		{"$from()", "", false},
		{"compose_prompt", "", false},
	}
	for _, tc := range cases {
		label, ok := wholeOutputSelector(tc.selector)
		require.Equal(t, tc.ok, ok, tc.selector)
		require.Equal(t, tc.label, label, tc.selector)
	}
}
