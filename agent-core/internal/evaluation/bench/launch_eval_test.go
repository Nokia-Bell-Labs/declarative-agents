// Copyright (c) 2026 Nokia. All rights reserved.

package bench

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/support/execute"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/catalog"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/undo"
)

func TestLaunchEvalFactoryRequiresChildAgentConfig(t *testing.T) {
	factory := LaunchEvalFactory(NewBenchState(ServerConfig{}))

	_, err := factory(catalog.ToolDef{Name: "launch_eval", Init: "launch_eval"}, nil)
	require.ErrorContains(t, err, "requires profile")
}

func TestLaunchEvalFactoryAcceptsProfileConfig(t *testing.T) {
	factory := LaunchEvalFactory(NewBenchState(ServerConfig{}))

	builder, err := factory(catalog.ToolDef{
		Name: "launch_eval",
		Init: "launch_eval",
		Config: map[string]interface{}{
			"profile": "agents/evaluator/profile.yaml",
		},
	}, nil)

	require.NoError(t, err)
	launchBuilder, ok := builder.(*LaunchEvalBuilder)
	require.True(t, ok)
	require.Equal(t, "agents/evaluator/profile.yaml", launchBuilder.Config.Profile)
}

func TestLaunchEvalUndoMementoCapturesChildEvalCompensation(t *testing.T) {
	t.Parallel()
	cmd := &launchEvalCmd{
		config:    execute.Config{Profile: "agents/evaluator/profile.yaml"},
		suitePath: "suites/basic.yaml",
		outputDir: "out/eval",
	}

	memento, err := cmd.UndoMemento()
	require.NoError(t, err)
	require.NoError(t, core.ValidateUndoMemento(memento))

	var payload undo.BoundaryCompensationPayload
	require.NoError(t, json.Unmarshal(memento.Payload, &payload))
	require.Equal(t, "child_eval_artifact_compensation", payload.BoundaryCompensation.Strategy)
	require.Equal(t, []string{"out/eval"}, payload.BoundaryCompensation.ArtifactPaths)
	require.Equal(t, "agents/evaluator/profile.yaml", payload.BoundaryCompensation.ChildProfile)
}
