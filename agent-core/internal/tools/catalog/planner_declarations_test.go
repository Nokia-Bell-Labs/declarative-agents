// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var plannerWordNames = map[string]bool{
	"extract_task": true, "select_all_ready": true, "seed_passthrough_plan": true,
	"mark_nodes_planning": true, "project_planner_context": true,
	"capture_planner_failure": true, "parse_plan": true,
	"mark_nodes_executing": true, "format_task_file": true,
	"mark_task_done": true, "mark_task_failed": true, "remaining_work": true,
}

func TestBuiltinBundleIncludesCanonicalPlannerContracts(t *testing.T) {
	t.Parallel()
	path := builtinBundlePath(t)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw ToolDefsFile
	require.NoError(t, yaml.Unmarshal(data, &raw))
	require.Equal(t, []string{"builtin/all.yaml"}, raw.Includes)
	require.Empty(t, raw.Tools)

	defs, err := LoadToolDeclarations([]string{path})
	require.NoError(t, err)
	for name := range plannerWordNames {
		def := toolDefByName(t, defs, name)
		if def.Undo.Strategy != "noop" && def.Undo.Strategy != "irreversible" {
			require.Equal(t, "pipeline_state_restore", def.Undo.Strategy, name)
		}
		for _, effect := range def.SideEffects.Items {
			require.NotEqual(t, "pipeline_graph", effect.Target, name)
		}
	}
}
