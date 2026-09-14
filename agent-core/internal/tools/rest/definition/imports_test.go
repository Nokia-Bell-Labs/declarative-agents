// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadDefinitionClosureResolvesTwoLevelImportsAndAllMapFamilies(t *testing.T) {
	root := t.TempDir()
	t.Setenv("SHARED_TOKEN_REF", "IMPORTED_TOKEN")
	leaf := writeImportFixture(t, root, "shared/leaf.yaml", `unit: shared-leaf
rest:
  version: v1
  auth: {shared-auth: {token_ref: "${SHARED_TOKEN_REF}"}}
  limits: {shared-limits: {}}
  retry_policies: {shared-retry: {}}
  response_mappings: {shared-response: {}}
  document_resources: {shared-documents: {}}
`)
	middle := writeImportFixture(t, root, "middle/middle.yaml", `unit: middle
imports: [../shared/leaf.yaml]
rest:
  clients: {shared-client: {}}
`)
	top := writeImportFixture(t, root, "top.yaml", `unit: top
imports: [middle/middle.yaml]
rest:
  servers: {shared-server: {}}
`)
	visits := map[string]int{}

	def, err := LoadDefinitionClosure([]string{top}, func(path string, _ []byte) error {
		visits[filepath.Clean(path)]++
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, "v1", def.Version)
	require.Contains(t, def.Clients, "shared-client")
	require.Contains(t, def.Servers, "shared-server")
	require.Contains(t, def.Auth, "shared-auth")
	require.Equal(t, "IMPORTED_TOKEN", def.Auth["shared-auth"].TokenRef)
	require.Contains(t, def.Limits, "shared-limits")
	require.Contains(t, def.RetryPolicies, "shared-retry")
	require.Contains(t, def.ResponseMappings, "shared-response")
	require.Contains(t, def.DocumentResources, "shared-documents")
	for _, path := range []string{top, middle, leaf} {
		require.Equal(t, 1, visits[path], path)
	}
}

func TestLoadDefinitionClosureReusesDiamondSourceOnce(t *testing.T) {
	root := t.TempDir()
	leaf := writeImportFixture(t, root, "leaf.yaml", "unit: leaf\nrest: {version: v1}\n")
	writeImportFixture(t, root, "left.yaml", "unit: left\nimports: [leaf.yaml]\nrest: {}\n")
	writeImportFixture(t, root, "right.yaml", "unit: right\nimports: [leaf.yaml]\nrest: {}\n")
	top := writeImportFixture(t, root, "top.yaml", "unit: top\nimports: [left.yaml, right.yaml]\nrest: {}\n")
	visits := map[string]int{}

	_, err := LoadDefinitionClosure([]string{top}, func(path string, _ []byte) error {
		visits[filepath.Clean(path)]++
		return nil
	})

	require.NoError(t, err)
	require.Equal(t, 1, visits[leaf])
}

func TestLoadDefinitionClosureCompilesOpenAPIRelativeToDeclaringUnit(t *testing.T) {
	root := t.TempDir()
	writeImportFixture(t, root, "shared/api.yaml", `openapi: 3.0.3
info: {title: shared, version: "1"}
paths:
  /items:
    get:
      operationId: listItems
      responses:
        "200": {description: ok}
`)
	writeImportFixture(t, root, "shared/rest.yaml", `unit: shared-api
rest:
  version: v1
  openapi:
    shared:
      path: api.yaml
      base_url: https://example.invalid
      expose: [listItems]
`)
	top := writeImportFixture(t, root, "top.yaml", "unit: top\nimports: [shared/rest.yaml]\nrest: {}\n")

	def, err := LoadDefinitionClosure([]string{top}, nil)

	require.NoError(t, err)
	require.Nil(t, def.OpenAPI)
	require.Equal(t, "GET", def.Clients["shared"].Operations["listItems"].Method)
	require.Equal(t, "/items", def.Clients["shared"].Operations["listItems"].Path)
}

func TestLoadDefinitionClosureRejectsEveryMapFamilyCollision(t *testing.T) {
	families := []string{
		"clients", "servers", "openapi", "auth", "limits", "retry_policies",
		"response_mappings", "document_resources",
	}
	for _, family := range families {
		t.Run(family, func(t *testing.T) {
			root := t.TempDir()
			value := "{}"
			if family == "openapi" {
				value = "{path: api.yaml}"
			}
			writeImportFixture(t, root, "first.yaml", fmt.Sprintf(
				"unit: first\nrest:\n  %s: {duplicate: %s}\n", family, value,
			))
			writeImportFixture(t, root, "second.yaml", fmt.Sprintf(
				"unit: second\nrest:\n  %s: {duplicate: %s}\n", family, value,
			))
			top := writeImportFixture(
				t, root, "top.yaml",
				"unit: top\nimports: [first.yaml, second.yaml]\nrest: {}\n",
			)

			_, err := LoadDefinitionClosure([]string{top}, nil)

			require.ErrorContains(t, err, "duplicate REST "+family)
			require.ErrorContains(t, err, `unit "first"`)
			require.ErrorContains(t, err, filepath.Join(root, "first.yaml"))
			require.ErrorContains(t, err, `unit "second"`)
			require.ErrorContains(t, err, filepath.Join(root, "second.yaml"))
		})
	}
}

func TestLoadDefinitionClosureRejectsUnitCollisionAndCycle(t *testing.T) {
	t.Run("unit collision", func(t *testing.T) {
		root := t.TempDir()
		first := writeImportFixture(t, root, "first.yaml", "unit: duplicate\nrest: {}\n")
		second := writeImportFixture(t, root, "second.yaml", "unit: duplicate\nrest: {}\n")
		top := writeImportFixture(t, root, "top.yaml", "unit: top\nimports: [first.yaml, second.yaml]\nrest: {}\n")

		_, err := LoadDefinitionClosure([]string{top}, nil)

		require.ErrorContains(t, err, `duplicate REST unit "duplicate"`)
		require.ErrorContains(t, err, first)
		require.ErrorContains(t, err, second)
	})

	t.Run("cycle", func(t *testing.T) {
		root := t.TempDir()
		first := writeImportFixture(t, root, "first.yaml", "unit: first\nimports: [second.yaml]\nrest: {}\n")
		second := writeImportFixture(t, root, "second.yaml", "unit: second\nimports: [first.yaml]\nrest: {}\n")

		_, err := LoadDefinitionClosure([]string{first}, nil)

		require.ErrorContains(t, err, "REST import cycle")
		require.ErrorContains(t, err, first+" -> "+second+" -> "+first)
	})
}

func TestLoadDefinitionClosureRejectsInvalidImportContracts(t *testing.T) {
	tests := []struct {
		name       string
		importBody string
		want       string
	}{
		{name: "target without unit", importBody: "rest: {}\n", want: "must declare unit"},
		{name: "invalid unit", importBody: "unit: Invalid_Name\nrest: {}\n", want: `invalid unit "Invalid_Name"`},
		{name: "missing rest content", importBody: "unit: empty\nimports: []\n", want: "top-level rest field is required"},
		{name: "mixed tool content", importBody: "unit: mixed\ntools: []\nrest: {}\n", want: `field tools not found`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeImportFixture(t, root, "imported.yaml", test.importBody)
			top := writeImportFixture(t, root, "top.yaml", "unit: top\nimports: [imported.yaml]\nrest: {}\n")

			_, err := LoadDefinitionClosure([]string{top}, nil)

			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestLoadDefinitionClosureRequiresUnitWhenImportsFieldIsAuthored(t *testing.T) {
	root := t.TempDir()
	path := writeImportFixture(t, root, "top.yaml", "imports: []\nrest: {}\n")

	_, err := LoadDefinitionClosure([]string{path}, nil)

	require.ErrorContains(t, err, "must declare unit")
}

func TestLoadDefinitionClosureRejectsAbsoluteImport(t *testing.T) {
	root := t.TempDir()
	imported := writeImportFixture(t, root, "imported.yaml", "unit: imported\nrest: {}\n")
	top := writeImportFixture(
		t, root, "top.yaml",
		fmt.Sprintf("unit: top\nimports: [%q]\nrest: {}\n", imported),
	)

	_, err := LoadDefinitionClosure([]string{top}, nil)

	require.ErrorContains(t, err, "imports absolute path")
	require.ErrorContains(t, err, imported)
}

func TestLoadDefinitionClosureRejectsConflictingVersionsWithSources(t *testing.T) {
	root := t.TempDir()
	first := writeImportFixture(t, root, "first.yaml", "unit: first\nrest: {version: v1}\n")
	second := writeImportFixture(t, root, "second.yaml", "unit: second\nrest: {version: v2}\n")
	top := writeImportFixture(t, root, "top.yaml", "unit: top\nimports: [first.yaml, second.yaml]\nrest: {}\n")

	_, err := LoadDefinitionClosure([]string{top}, nil)

	require.ErrorContains(t, err, "conflicting REST versions")
	require.ErrorContains(t, err, first)
	require.ErrorContains(t, err, second)
}

func writeImportFixture(t *testing.T, root, name, content string) string {
	t.Helper()
	path := filepath.Clean(filepath.Join(root, name))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
