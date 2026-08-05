// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func reportParseErrorFactory(st *agentState) toolregistry.BuiltinFactory {
	return func(def catalog.ToolDef, _ map[string]string) (core.Builder, error) {
		var cfg catalog.ReportParseErrorConfig
		if err := catalog.DecodeToolConfig(def, &cfg); err != nil {
			return nil, err
		}
		contract, err := toollm.ParseErrorResponseContractValue(cfg.ResponseContract)
		if err != nil {
			return nil, fmt.Errorf("tool %q config: %w", def.Name, err)
		}
		return &toollm.ReportParseErrorBuilder{
			Tracer:           st.tracer,
			Retry:            st.parseRetries,
			ResponseContract: contract,
		}, nil
	}
}
