// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package pipeline

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestExtractTaskFactoryAppliesDeclaredMaxWeight(t *testing.T) {
	t.Parallel()

	registry := toolregistry.NewBuiltinRegistry()
	RegisterFactories(registry, FactoryDeps{})
	factory, ok := registry.Resolve("extract_task")
	require.True(t, ok)
	builder, err := factory(catalog.ToolDef{
		Name: "extract_task", Config: map[string]interface{}{"max_weight": 3},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, 3, builder.(*ExtractTaskBuilder).PS.MaxWeight)
}

func TestExtractTaskDeclarationDefaultsToUnlimitedWeight(t *testing.T) {
	t.Parallel()

	defs, err := catalog.LoadToolDefs(filepath.Join("..", "..", "..", "tools", "builtin", "extract-task.yaml"))
	require.NoError(t, err)
	require.Len(t, defs, 1)
	registry := toolregistry.NewBuiltinRegistry()
	RegisterFactories(registry, FactoryDeps{})
	factory, ok := registry.Resolve("extract_task")
	require.True(t, ok)
	builder, err := factory(defs[0], nil)
	require.NoError(t, err)
	require.Zero(t, builder.(*ExtractTaskBuilder).PS.MaxWeight)
}

func TestPlannerProjectionFactoriesApplyProfilePolicy(t *testing.T) {
	t.Parallel()

	registry := toolregistry.NewBuiltinRegistry()
	RegisterFactories(registry, FactoryDeps{})
	taskFactory, ok := registry.Resolve("format_task_file")
	require.True(t, ok)
	taskBuilder, err := taskFactory(catalog.ToolDef{
		Name: "format_task_file", Config: map[string]interface{}{"path": "plan/next.yaml"},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, "plan/next.yaml", taskBuilder.(*FormatTaskFileBuilder).Path)

	issueFactory, ok := registry.Resolve("format_issue")
	require.True(t, ok)
	issueBuilder, err := issueFactory(catalog.ToolDef{
		Name: "format_issue",
		Config: map[string]interface{}{
			"body_path": ".planner/body.yaml", "deliverable_type": "documentation",
		},
	}, nil)
	require.NoError(t, err)
	require.Equal(t, ".planner/body.yaml", issueBuilder.(*FormatIssueBuilder).BodyPath)
	require.Equal(t, "documentation", issueBuilder.(*FormatIssueBuilder).DeliverableType)
}
