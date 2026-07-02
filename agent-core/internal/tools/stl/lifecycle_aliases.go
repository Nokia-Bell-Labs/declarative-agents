// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	toollifecycle "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/lifecycle"
)

type (
	SuspendConfig             = toollifecycle.SuspendConfig
	LifecycleFactoryDeps      = toollifecycle.FactoryDeps
	SuspendBuilder            = toollifecycle.SuspendBuilder
	CheckpointHistoryBuilder  = toollifecycle.CheckpointHistoryBuilder
	CheckpointRollbackBuilder = toollifecycle.CheckpointRollbackBuilder
)

var RegisterLifecycleFactories = toollifecycle.RegisterFactories
