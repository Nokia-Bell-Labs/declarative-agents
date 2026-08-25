// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package llm

import (
	"context"
	"fmt"
	"time"

	modelllm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/control"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

const (
	InitInvokeLLM        = "invoke_llm"
	InitParseResponse    = "parse_response"
	InitParseStructured  = "parse_structured"
	InitReportParseError = "report_parse_error"
	InitResetHistory     = "reset_history"
	InitNudgeReread      = "nudge_reread"
	InitDone             = "done"
)

// ResolvedModel carries what invoke_llm's build learns to the factories and
// the composition root that need it.
type ResolvedModel struct {
	Parser        modelllm.ResponseParser
	Model         string
	ProviderName  string
	ManifestState core.State
	MaxDuration   time.Duration
	MaxTokens     int
}

// ReferencePorts are checkpoint-backed conversation ports already resolved by
// the composition root (checkpointForOps stays in cmd/agent).
type ReferencePorts struct {
	Provider ConversationReferenceProvider
	Resolver ConversationReferenceResolver
}

// FactoryDeps names the process-local ports the LLM family captures.
type FactoryDeps struct {
	Ctx                  context.Context
	Tracer               tracing.Tracer
	Registry             *core.Registry
	Conversation         *modelllm.Conversation
	IsolateConversations bool
	CaptureLevel         CaptureLevel
	ParseRetries         *ParseErrorRetryTracker
	ConversationRefs     ReferencePorts
	Resolved             *ResolvedModel
}

// RegisterFactories registers LLM builtin factories. done and nudge_reread
// builders live in control, but they stay in this family so SelectedBy gating
// still pulls the LLM catalog when a profile selects only those inits.
func RegisterFactories(br *toolregistry.BuiltinRegistry, deps FactoryDeps) {
	br.Register(InitInvokeLLM, invokeLLMFactory(deps))
	br.Register(InitParseResponse, parseResponseFactory(deps))
	br.Register(InitParseStructured, parseStructuredFactory())
	br.Register(InitReportParseError, reportParseErrorFactory(deps))
	br.Register(InitResetHistory, resetHistoryFactory(deps))
	br.Register(InitNudgeReread, nudgeRereadFactory(deps))
	br.Register(InitDone, func(catalog.ToolDef, map[string]string) (core.Builder, error) {
		return control.DoneBuilder{}, nil
	})
}

func nudgeRereadFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg struct {
			Text string `json:"nudge_text"`
		}
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if cfg.Text == "" {
			return nil, fmt.Errorf("tool %q config nudge_text is required", def.Name)
		}
		return &control.NudgeRereadBuilder{Tracer: deps.Tracer, Text: cfg.Text}, nil
	}
}

func parseStructuredFactory() toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.ParseStructuredConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if err := catalog.ValidateParseStructuredConfig(def.Name, cfg); err != nil {
			return nil, err
		}
		schema, err := CompileStructuredSchema(def.Name, cfg.Schema)
		if err != nil {
			return nil, err
		}
		return ParseStructuredBuilder{
			ToolName: def.Name, Source: cfg.Source, Schema: schema,
			Parsed: core.Signal(cfg.Parsed), Unparsed: core.Signal(cfg.Unparsed),
		}, nil
	}
}

func invokeLLMFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		history := deps.Conversation
		if deps.IsolateConversations {
			history = modelllm.NewConversation(nil, "", modelllm.ChatOptions{})
		}
		return NewInvokeLLMBuilder(def, InvokeLLMFactoryDeps{
			History: history, Registry: deps.Registry, Tracer: deps.Tracer,
			CaptureLevel: deps.CaptureLevel, ConversationRefProvider: deps.ConversationRefs.Provider,
			ConversationRefResolver: deps.ConversationRefs.Resolver, Ctx: deps.Ctx,
			OnResolved: applyResolved(deps.Resolved),
		})
	}
}

func applyResolved(resolved *ResolvedModel) func(InvokeLLMResolvedConfig) {
	return func(cfg InvokeLLMResolvedConfig) {
		if resolved == nil {
			return
		}
		resolved.Parser = cfg.Parser
		resolved.Model = cfg.Model
		resolved.ProviderName = cfg.ProviderName
		resolved.ManifestState = cfg.ManifestState
		resolved.MaxDuration = cfg.MaxTime
		resolved.MaxTokens = cfg.MaxTokens
	}
}

func parseResponseFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var parser modelllm.ResponseParser
		if deps.Resolved != nil {
			parser = deps.Resolved.Parser
		}
		return &ParseResponseBuilder{
			ToolName: def.Name, Registry: deps.Registry, Parser: parser, Tracer: deps.Tracer,
			StateFunc: func() core.State {
				if deps.Resolved == nil {
					return ""
				}
				return deps.Resolved.ManifestState
			},
			CaptureLevel: deps.CaptureLevel, Retry: deps.ParseRetries,
		}, nil
	}
}

func resetHistoryFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		return &ResetHistoryBuilder{
			ToolName: def.Name, History: deps.Conversation, Tracer: deps.Tracer,
			ConversationRefProvider: deps.ConversationRefs.Provider,
			ConversationRefResolver: deps.ConversationRefs.Resolver,
		}, nil
	}
}

func reportParseErrorFactory(deps FactoryDeps) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.ReportParseErrorConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		if cfg.FeedbackTemplate == "" {
			return nil, fmt.Errorf("tool %q config feedback_template is required", def.Name)
		}
		return &ReportParseErrorBuilder{
			ToolName: def.Name, Tracer: deps.Tracer, Retry: deps.ParseRetries,
			FeedbackTemplate: cfg.FeedbackTemplate,
		}, nil
	}
}
