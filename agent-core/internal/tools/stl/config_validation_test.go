// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateChildAgentConfigRequiresFields(t *testing.T) {
	require.ErrorContains(t, ValidateChildAgentConfig("execute_task", ChildAgentConfig{}), "requires profile or legacy machine")
	require.ErrorContains(t, ValidateChildAgentConfig("execute_task", ChildAgentConfig{Machine: "m.yaml"}), "requires tools")
	require.ErrorContains(t, ValidateChildAgentConfig("execute_task", ChildAgentConfig{
		Machine: "m.yaml",
		Tools:   "tools.yaml",
	}), "requires tools_declarations")
	require.NoError(t, ValidateChildAgentConfig("execute_task", ChildAgentConfig{
		Profile: "agents/generator/profile.yaml",
	}))
	require.NoError(t, ValidateChildAgentConfig("execute_task", ChildAgentConfig{
		Machine:          "m.yaml",
		Tools:            "tools.yaml",
		ToolDeclarations: []string{"builtin.yaml"},
	}))
}

func TestValidateRunPointConfigRequiresFields(t *testing.T) {
	require.ErrorContains(t, ValidateRunPointConfig("run_point", RunPointConfig{}), "requires point_machine")
	require.ErrorContains(t, ValidateRunPointConfig("run_point", RunPointConfig{
		PointMachine: "point.yaml",
	}), "requires point_tools")
	require.NoError(t, ValidateRunPointConfig("run_point", RunPointConfig{
		PointMachine: "point.yaml",
		PointTools:   "tools-point.yaml",
	}))
}
