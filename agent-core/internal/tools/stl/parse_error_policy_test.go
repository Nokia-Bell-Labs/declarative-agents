// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

type stubCmd struct{ name string }

func (s stubCmd) Name() string         { return s.name }
func (s stubCmd) Execute() core.Result { return core.Result{} }
func (s stubCmd) Undo() core.Result    { return core.NoopUndo(s.name) }

func TestParseErrorPolicy_TriggersAfterLimit(t *testing.T) {
	policy := ParseErrorPolicy(3)

	for i := 0; i < 2; i++ {
		sig := policy(stubCmd{"parse_response"}, core.Result{Signal: core.ParseFailed})
		assert.Empty(t, sig, "should not trigger before limit")
	}

	sig := policy(stubCmd{"parse_response"}, core.Result{Signal: core.ParseFailed})
	assert.Equal(t, core.BudgetExhausted, sig, "should trigger at limit")
}

func TestParseErrorPolicy_ResetsOnNonParseSignal(t *testing.T) {
	policy := ParseErrorPolicy(3)

	policy(stubCmd{"parse_response"}, core.Result{Signal: core.ParseFailed})
	policy(stubCmd{"parse_response"}, core.Result{Signal: core.ParseFailed})

	policy(stubCmd{"read"}, core.Result{Signal: core.ToolDone})

	sig := policy(stubCmd{"parse_response"}, core.Result{Signal: core.ParseFailed})
	assert.Empty(t, sig, "counter should have reset")
}

func TestParseErrorPolicy_KeepsDuringRetryLoop(t *testing.T) {
	policy := ParseErrorPolicy(3)

	policy(stubCmd{"parse_response"}, core.Result{Signal: core.ParseFailed})

	sig := policy(stubCmd{"report_parse_error"}, core.Result{Signal: core.ToolDone})
	assert.Empty(t, sig, "report_parse_error should not reset counter")

	sig = policy(stubCmd{"invoke_llm"}, core.Result{Signal: core.ToolDone})
	assert.Empty(t, sig, "invoke_llm should not reset counter")

	policy(stubCmd{"parse_response"}, core.Result{Signal: core.ParseFailed})
	sig = policy(stubCmd{"parse_response"}, core.Result{Signal: core.ParseFailed})
	assert.Equal(t, core.BudgetExhausted, sig, "counter should accumulate across retry cycle")
}

func TestParseErrorPolicy_ZeroLimitNeverTriggers(t *testing.T) {
	policy := ParseErrorPolicy(0)

	for i := 0; i < 100; i++ {
		sig := policy(stubCmd{"parse_response"}, core.Result{Signal: core.ParseFailed})
		assert.Empty(t, sig)
	}
}
