// Copyright (c) 2026 Nokia. All rights reserved.

package pipeline

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestProjectPlannerContextEmitsPromptNeutralFields(t *testing.T) {
	ps := minimalState(t)
	selected := (&ExtractTaskBuilder{PS: ps}).Build(core.Result{}).Execute()
	require.Equal(t, SigTaskExtracted, selected.Signal)

	result := (&ProjectPlannerContextBuilder{PS: ps}).Build(core.Result{}).Execute()
	require.Equal(t, SigPlannerContextProjected, result.Signal)
	var context map[string]string
	require.NoError(t, json.Unmarshal([]byte(result.Output), &context))
	require.Equal(t, ps.CurrentTask.ID, context["task_id"])
	require.Equal(t, ps.CurrentTask.SRDID, context["srd_id"])
	require.NotEmpty(t, context["problem"])
	require.Contains(t, context["items"], "- ")
	require.NotContains(t, result.Output, "Implementation Planning")
	require.NotContains(t, result.Output, "Output Format")
}

func TestCapturePlannerFailurePublishesPriorOutput(t *testing.T) {
	result := (&CapturePlannerFailureBuilder{}).Build(core.Result{Output: "go test failed: parser"}).Execute()
	require.Equal(t, SigFailureCaptured, result.Signal)
	var context map[string]string
	require.NoError(t, json.Unmarshal([]byte(result.Output), &context))
	require.Equal(t, "go test failed: parser", context["output"])
}
