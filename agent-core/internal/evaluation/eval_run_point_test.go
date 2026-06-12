// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/tools/stl"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunPointFactoryRequiresNestedConfig(t *testing.T) {
	factory := RunPointFactory(&EvalSessionState{})

	_, err := factory(stl.ToolDef{Name: "run_point", Init: "run_point"}, nil)
	require.ErrorContains(t, err, "requires point_machine")

	_, err = factory(stl.ToolDef{
		Name: "run_point",
		Init: "run_point",
		Config: map[string]interface{}{
			"point_machine": "agents/evaluator/point.yaml",
		},
	}, nil)
	require.ErrorContains(t, err, "requires point_tools")
}
