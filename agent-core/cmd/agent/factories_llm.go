// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/model/llm"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/control"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func registerLLMFactories(st *agentState) toolregistry.FactoryRegistrar {
	return func(br *toolregistry.BuiltinRegistry) {
		br.Register("invoke_llm", invokeLLMFactory(st))
		br.Register("parse_response", parseResponseFactory(st))
		br.Register("parse_structured", parseStructuredFactory())
		br.Register("report_parse_error", reportParseErrorFactory(st))
		br.Register("reset_history", resetHistoryFactory(st))
		br.Register("nudge_reread", nudgeRereadFactory(st))
		br.Register("done", func(catalog.ToolDef, map[string]string) (core.Builder, error) {
			return control.DoneBuilder{}, nil
		})
	}
}

func nudgeRereadFactory(st *agentState) toolregistry.BuiltinFactory {
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
		return &control.NudgeRereadBuilder{Tracer: st.tracer, Text: cfg.Text}, nil
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
		schema, err := toollm.CompileStructuredSchema(def.Name, cfg.Schema)
		if err != nil {
			return nil, err
		}
		return toollm.ParseStructuredBuilder{
			ToolName: def.Name, Source: cfg.Source, Schema: schema,
			Parsed: core.Signal(cfg.Parsed), Unparsed: core.Signal(cfg.Unparsed),
		}, nil
	}
}

func invokeLLMFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		history := st.conversation
		if st.isolateConversations {
			history = llm.NewConversation(nil, "", llm.ChatOptions{})
		}
		refProvider, refResolver := llmConversationReferencePorts(st)
		return toollm.NewInvokeLLMBuilder(def, toollm.InvokeLLMFactoryDeps{
			History: history, Registry: st.registry, Tracer: st.tracer,
			CaptureLevel: st.captureLevel, ConversationRefProvider: refProvider,
			ConversationRefResolver: refResolver, Ctx: st.ctx,
			OnResolved: onModelResolved(st),
		})
	}
}

func onModelResolved(st *agentState) func(toollm.InvokeLLMResolvedConfig) {
	return func(cfg toollm.InvokeLLMResolvedConfig) {
		st.parser = cfg.Parser
		st.model = cfg.Model
		st.providerName = cfg.ProviderName
		st.manifestState = cfg.ManifestState
		st.maxDuration = cfg.MaxTime
		st.maxTokens = cfg.MaxTokens
	}
}

func parseResponseFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(catalog.ToolDef, map[string]string) (core.Builder, error) {
		return &toollm.ParseResponseBuilder{
			Registry: st.registry, Parser: st.parser, Tracer: st.tracer,
			StateFunc:    func() core.State { return st.manifestState },
			CaptureLevel: st.captureLevel, Retry: st.parseRetries,
		}, nil
	}
}

func resetHistoryFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		refProvider, refResolver := llmConversationReferencePorts(st)
		return &toollm.ResetHistoryBuilder{
			ToolName: def.Name, History: st.conversation, Tracer: st.tracer,
			ConversationRefProvider: refProvider,
			ConversationRefResolver: refResolver,
		}, nil
	}
}
