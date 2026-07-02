// Copyright (c) 2026 Nokia. All rights reserved.

package docsapi

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestServeDocumentationFactoryBuildsFromConfig(t *testing.T) {
	t.Parallel()

	docsDir := t.TempDir()
	def := catalog.ToolDef{
		Name: "serve_documentation",
		Config: map[string]any{
			"addr":     ":0",
			"docs_dir": docsDir,
		},
	}

	builder, err := ServeDocumentationFactory()(def, nil)

	require.NoError(t, err)
	require.IsType(t, ServeDocumentationBuilder{}, builder)
	require.Equal(t, ":0", builder.(ServeDocumentationBuilder).Config.Addr)
	require.Equal(t, filepath.Clean(docsDir), builder.(ServeDocumentationBuilder).Config.DocsDir)
}

func TestServeDocumentationFactoryRequiresDocsDir(t *testing.T) {
	t.Parallel()

	_, err := ServeDocumentationFactory()(catalog.ToolDef{Name: "serve_documentation"}, nil)

	require.ErrorContains(t, err, `tool "serve_documentation" config requires docs_dir`)
}

func TestRegisterFactoriesAddsServeDocumentation(t *testing.T) {
	t.Parallel()

	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br)

	_, ok := br.Resolve("serve_documentation")
	require.True(t, ok)
}
