// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefinitionVisitorIncludesNestedOpenAPIRefs(t *testing.T) {
	root := t.TempDir()
	restPath := writeDefinitionFixture(t, root, "rest.yaml", `rest:
  version: v1
  openapi:
    api:
      path: openapi.yaml
      expose: [listItems]
`)
	openAPIPath := writeDefinitionFixture(t, root, "openapi.yaml", `openapi: 3.0.3
info: {title: nested, version: "1"}
paths:
  /items:
    get:
      operationId: listItems
      responses:
        "200":
          description: ok
          content:
            application/json:
              schema:
                $ref: components.yaml#/components/schemas/Items
`)
	componentsPath := writeDefinitionFixture(t, root, "components.yaml", `components:
  schemas:
    Items:
      type: array
      items: {type: string}
`)
	visited := map[string]int{}

	_, err := LoadDefinitionWithVisitor(restPath, func(path string, _ []byte) error {
		visited[filepath.Clean(path)]++
		return nil
	})

	require.NoError(t, err)
	for _, path := range []string{restPath, openAPIPath, componentsPath} {
		require.Equal(t, 1, visited[filepath.Clean(path)], path)
	}
}

func writeDefinitionFixture(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
