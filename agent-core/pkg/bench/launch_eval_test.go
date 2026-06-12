// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
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
			"machine": "agents/evaluator/machine.yaml",
			"tools":   "agents/evaluator/tools.yaml",
		},
	}, nil)
	require.ErrorContains(t, err, "requires tools_declarations")
}

func TestLaunchEvalUndoMementoCapturesChildEvalCompensation(t *testing.T) {
	t.Parallel()
	cmd := &launchEvalCmd{
		config:    execute.Config{Machine: "machines/eval.yaml", Tools: "tools/eval.yaml"},
		suitePath: "suites/basic.yaml",
		outputDir: "out/eval",
	}

	memento, err := cmd.UndoMemento()
	require.NoError(t, err)
	require.NoError(t, core.ValidateUndoMemento(memento))

	var payload stl.BoundaryCompensationPayload
	require.NoError(t, json.Unmarshal(memento.Payload, &payload))
	require.Equal(t, "child_eval_artifact_compensation", payload.BoundaryCompensation.Strategy)
	require.Equal(t, []string{"out/eval"}, payload.BoundaryCompensation.ArtifactPaths)
	require.Equal(t, "machines/eval.yaml", payload.BoundaryCompensation.ChildMachine)
}
