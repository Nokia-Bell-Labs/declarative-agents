// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
)

type (
	InvokeLLMBuilder        = toollm.InvokeLLMBuilder
	InvokeLLMFactoryDeps    = toollm.InvokeLLMFactoryDeps
	InvokeLLMResolvedConfig = toollm.InvokeLLMResolvedConfig
	ParseResponseBuilder    = toollm.ParseResponseBuilder
	ReportParseErrorBuilder = toollm.ReportParseErrorBuilder
	ResetHistoryBuilder     = toollm.ResetHistoryBuilder
	ParseErrorRetryTracker  = toollm.ParseErrorRetryTracker
)

var (
	NewInvokeLLMBuilder   = toollm.NewInvokeLLMBuilder
	DecodeInvokeLLMConfig = toollm.DecodeInvokeLLMConfig
	ParseErrorPolicy      = toollm.ParseErrorPolicy
)
