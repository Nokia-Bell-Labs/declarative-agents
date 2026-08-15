// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/execute"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func childExecuteConfig(parsed catalog.ChildAgentConfig, coreRoot string) execute.Config {
	return execute.Config{
		Profile:  parsed.Profile,
		CoreRoot: coreRoot,
		Request:  parsed.Request,
		Output:   parsed.Output,
	}
}
