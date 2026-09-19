// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package declstyle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// providerEmbeddingPaths are the provider endpoints the shipped provider
// libraries own. An application operation calling one hand-writes what a
// library fragment already declares (srd058 R4.2).
var providerEmbeddingPaths = map[string]bool{
	"/api/embeddings": true, "/api/embed": true, "/v2/embed": true, "/v2/rerank": true,
}

// handWrittenProviderOperations lists REST operations outside a provider
// library, and outside test fixtures, that call a provider embedding or rerank
// endpoint directly.
func handWrittenProviderOperations(paths []string, repo string) ([]string, error) {
	var found []string
	for _, path := range paths {
		rel, err := filepath.Rel(repo, path)
		if err != nil {
			return nil, err
		}
		rel = filepath.ToSlash(rel)
		if strings.Contains(rel, "/tools/providers/") || strings.Contains(rel, "testdata/") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var file struct {
			Rest struct {
				Clients map[string]struct {
					Operations map[string]struct {
						Path string `yaml:"path"`
					} `yaml:"operations"`
				} `yaml:"clients"`
			} `yaml:"rest"`
		}
		if yaml.Unmarshal(placeholderSafe(data), &file) != nil {
			continue
		}
		for client, declared := range file.Rest.Clients {
			for name, operation := range declared.Operations {
				if providerEmbeddingPaths[operation.Path] {
					found = append(found, rel+": "+client+"."+name+" "+operation.Path)
				}
			}
		}
	}
	return found, nil
}

// handWrittenProviderBaseline is the one operation still awaiting adoption:
// the chatbot's query embedding, which the chart also renders (GH-2245). An
// entry is deleted when its operation is gone; the list does not grow.
var handWrittenProviderBaseline = []string{
	"applications/chatbot-mesh/agents/chatbot/rest.yaml: embedding.embed_query /api/embeddings",
}

func TestNoHandWrittenProviderEmbedding(t *testing.T) {
	paths, err := discoverLegacyDeclarationFiles(declarationRoots(t))
	require.NoError(t, err)

	found, err := handWrittenProviderOperations(paths, filepath.Dir(moduleRoot(t)))

	require.NoError(t, err)
	require.ElementsMatch(t, handWrittenProviderBaseline, found,
		"embedding and rerank operations come from the bound provider library (srd058 R4.2); "+
			"a new entry is hand-written provider REST, a missing one is a stale baseline line")
}

func TestHandWrittenProviderEmbeddingIsFound(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) string {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
		return path
	}
	operation := "rest:\n  clients:\n    ollama:\n      operations:\n        embed: {method: POST, path: /api/embeddings}\n"
	paths := []string{
		write("applications/app/agents/a/rest.yaml", operation),
		write("agent-core/tools/providers/ollama/embed-query-fragment.yaml", operation),
		write("applications/app/testdata/rest.yaml", operation),
	}

	found, err := handWrittenProviderOperations(paths, root)

	require.NoError(t, err)
	require.Equal(t, []string{"applications/app/agents/a/rest.yaml: ollama.embed /api/embeddings"}, found)
}
