// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateChildAgentConfigRequiresFields(t *testing.T) {
	require.ErrorContains(t, ValidateChildAgentConfig("execute_task", ChildAgentConfig{}), "requires profile")
	require.NoError(t, ValidateChildAgentConfig("execute_task", ChildAgentConfig{
		Profile: "agents/executor/profile.yaml",
	}))
	require.NoError(t, ValidateChildAgentConfig("self_invoke", ChildAgentConfig{
		Profile:     "agents/critic/profile.yaml",
		RequestFrom: "$from(action).payload.body.config.suite",
		OutputFrom:  "$from(action).payload.body.config.output_dir",
	}))
	require.ErrorContains(t, ValidateChildAgentConfig("self_invoke", ChildAgentConfig{
		Profile: "agents/critic/profile.yaml", RequestFrom: "$.suite",
	}), "request_from")
}

func TestValidateRunPointConfigRequiresFields(t *testing.T) {
	require.ErrorContains(t, ValidateRunPointConfig("run_point", RunPointConfig{}), "requires point_machine")
	require.ErrorContains(t, ValidateRunPointConfig("run_point", RunPointConfig{
		PointMachine: "point.yaml",
	}), "requires point_tools")
	require.ErrorContains(t, ValidateRunPointConfig("run_point", RunPointConfig{
		PointMachine: "point.yaml",
		PointTools:   "tools-point.yaml",
	}), "requires point_tool_declarations")
	cfg := RunPointConfig{
		PointMachine:          "point.yaml",
		PointTools:            "tools-point.yaml",
		PointToolDeclarations: []string{"point-builtins.yaml", "exec.yaml"},
	}
	require.ErrorContains(t, ValidateRunPointConfig("run_point", cfg), "requires agent_name")
	cfg.AgentName = "critic-point"
	require.ErrorContains(t, ValidateRunPointConfig("run_point", cfg), "requires positive max_iterations")
	cfg.MaxIterations = 20
	require.ErrorContains(t, ValidateRunPointConfig("run_point", cfg), "requires success_state")
	cfg.SuccessState = "Done"
	require.NoError(t, ValidateRunPointConfig("run_point", cfg))
}
