// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestLaunchDocumentationFactoryBuildsFromConfig(t *testing.T) {
	t.Parallel()

	docsDir := t.TempDir()
	def := catalog.ToolDef{
		Name: "launch_documentation",
		Config: map[string]any{
			"addr":     ":0",
			"docs_dir": docsDir,
		},
	}

	builder, err := LaunchDocumentationFactory(NewDocumentationHostLifecycle())(def, nil)

	require.NoError(t, err)
	require.IsType(t, launchDocumentationBuilder{}, builder)
	require.Equal(t, ":0", builder.(launchDocumentationBuilder).config.Addr)
	require.Equal(t, filepath.Clean(docsDir), builder.(launchDocumentationBuilder).config.DocsDir)
}

func TestLaunchDocumentationFactoryRequiresDocsDir(t *testing.T) {
	t.Parallel()

	_, err := LaunchDocumentationFactory(NewDocumentationHostLifecycle())(catalog.ToolDef{Name: "launch_documentation"}, nil)

	require.ErrorContains(t, err, `tool "launch_documentation" config requires docs_dir`)
}

func TestRegisterFactoriesAddsLaunchAndStopDocumentation(t *testing.T) {
	t.Parallel()

	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br)

	_, launchOK := br.Resolve("launch_documentation")
	require.True(t, launchOK)
	_, stopOK := br.Resolve("stop_documentation")
	require.True(t, stopOK)
}

func TestLaunchAndStopShareOneHost(t *testing.T) {
	t.Parallel()

	// stop_documentation must tear down exactly the listener launch_documentation
	// started, so both factories close over the same host lifecycle owner.
	host := NewDocumentationHostLifecycle()
	docsDir := t.TempDir()
	lb, err := LaunchDocumentationFactory(host)(catalog.ToolDef{Name: "launch_documentation", Config: map[string]any{"addr": ":0", "docs_dir": docsDir}}, nil)
	require.NoError(t, err)
	require.Same(t, host, lb.(launchDocumentationBuilder).host)

	sb, err := StopDocumentationFactory(host)(catalog.ToolDef{Name: "stop_documentation"}, nil)
	require.NoError(t, err)
	require.Same(t, host, sb.(stopDocumentationBuilder).host)
}
