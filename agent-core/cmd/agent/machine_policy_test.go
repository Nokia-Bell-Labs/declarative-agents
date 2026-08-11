// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestLoopParamsUsesMachineCommandTimeout(t *testing.T) {
	t.Parallel()
	machine := core.MachineSpec{
		BudgetSpec: &core.BudgetSpec{CommandTimeout: "7s"},
	}
	params := loopParams(runtimeConfig{}, loopParamDeps{
		Machine: machine, State: &agentState{}, Registry: core.NewRegistry(),
	})
	require.Equal(t, 7*time.Second, params.CommandTimeout)

	machine.BudgetSpec = nil
	params = loopParams(runtimeConfig{}, loopParamDeps{
		Machine: machine, State: &agentState{}, Registry: core.NewRegistry(),
	})
	require.Zero(t, params.CommandTimeout)
}
