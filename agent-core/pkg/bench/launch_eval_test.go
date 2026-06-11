// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/stl"
)

func TestLaunchEvalFactoryRequiresChildAgentConfig(t *testing.T) {
	factory := LaunchEvalFactory(NewBenchState(ServerConfig{}))

	_, err := factory(stl.ToolDef{Name: "launch_eval", Init: "launch_eval"}, nil)
	require.ErrorContains(t, err, "requires machine")

	_, err = factory(stl.ToolDef{
		Name: "launch_eval",
		Init: "launch_eval",
		Config: map[string]interface{}{
			"machine": "configs/evaluator/machine.yaml",
			"tools":   "configs/evaluator/tools.yaml",
		},
	}, nil)
	require.ErrorContains(t, err, "requires tools_declarations")
}
