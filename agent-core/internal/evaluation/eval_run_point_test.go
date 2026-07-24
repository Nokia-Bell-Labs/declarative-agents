// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

func TestRunPointFactoryRequiresNestedConfig(t *testing.T) {
	factory := RunPointFactory(&EvalSessionState{})

	_, err := factory(catalog.ToolDef{Name: "run_point", Init: "run_point"}, nil)
	require.ErrorContains(t, err, "requires point_machine")

	_, err = factory(catalog.ToolDef{
		Name: "run_point",
		Init: "run_point",
		Config: map[string]interface{}{
			"point_machine": "agents/critic/point.yaml",
		},
	}, nil)
	require.ErrorContains(t, err, "requires point_tools")

	_, err = factory(catalog.ToolDef{
		Name: "run_point",
		Init: "run_point",
		Config: map[string]interface{}{
			"point_machine": "agents/critic/point.yaml",
			"point_tools":   "agents/critic/tools-point.yaml",
		},
	}, nil)
	require.ErrorContains(t, err, "requires point_tool_declarations")
}

func TestBuildPointRegistryUsesUnifiedBuiltinAndExecDeclarations(t *testing.T) {
	dir := t.TempDir()
	selection := filepath.Join(dir, "tools.yaml")
	declarations := filepath.Join(dir, "declarations.yaml")
	require.NoError(t, os.WriteFile(selection, []byte("tools:\n  - create_point_dir\n  - point_pwd\n"), 0o644))
	require.NoError(t, os.WriteFile(declarations, []byte(`tools:
  - name: create_point_dir
    type: builtin
    init: create_point_dir
    visibility: internal
  - name: point_pwd
    type: exec
    binary: pwd
    visibility: internal
`), 0o644))

	firstRoot := t.TempDir()
	es := &EvalState{PC: &PointContext{PointDir: firstRoot}}
	reg, err := buildPointRegistry(es, catalog.RunPointConfig{
		PointTools:            selection,
		PointToolDeclarations: []string{declarations},
	})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"create_point_dir", "point_pwd"}, reg.AllToolNames())

	builder, ok := reg.Resolve("point_pwd")
	require.True(t, ok)
	cmd := builder.Build(core.Result{})
	secondRoot := t.TempDir()
	es.PC.PointDir = secondRoot
	result := cmd.Execute()
	require.Equal(t, core.ToolDone, result.Signal, result.Output)
	require.Equal(t, secondRoot, strings.TrimSpace(result.Output))
}

func TestBuildPointRegistryRejectsUndeclaredSelection(t *testing.T) {
	dir := t.TempDir()
	selection := filepath.Join(dir, "tools.yaml")
	declarations := filepath.Join(dir, "declarations.yaml")
	require.NoError(t, os.WriteFile(selection, []byte("tools:\n  - absent\n"), 0o644))
	require.NoError(t, os.WriteFile(declarations, []byte("tools: []\n"), 0o644))

	_, err := buildPointRegistry(&EvalState{}, catalog.RunPointConfig{
		PointTools:            selection,
		PointToolDeclarations: []string{declarations},
	})
	require.ErrorContains(t, err, `tool "absent" is selected but not declared`)
}
