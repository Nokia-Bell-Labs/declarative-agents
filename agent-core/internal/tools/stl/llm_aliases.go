// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	toollm "gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/llm"
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
