// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadToolDefsAllowsIncludeDiamond(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeDeclarationFixture(t, dir, "leaf.yaml", "tools:\n- {name: shared, binary: echo}\n")
	writeDeclarationFixture(t, dir, "left.yaml", "includes: [leaf.yaml]\ntools: []\n")
	writeDeclarationFixture(t, dir, "right.yaml", "includes: [leaf.yaml]\ntools: []\n")
	root := writeDeclarationFixture(
		t, dir, "root.yaml", "includes: [left.yaml, right.yaml]\ntools: []\n",
	)

	defs, err := LoadToolDefs(root)
	require.NoError(t, err)
	require.Len(t, defs, 1)
	require.Equal(t, "shared", defs[0].Name)
}

func TestLoadToolDefsReportsTrueIncludeChain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := writeDeclarationFixture(t, dir, "a.yaml", "includes: [b.yaml]\ntools: []\n")
	writeDeclarationFixture(t, dir, "b.yaml", "includes: [c.yaml]\ntools: []\n")
	writeDeclarationFixture(t, dir, "c.yaml", "includes: [a.yaml]\ntools: []\n")

	_, err := LoadToolDefs(a)
	require.ErrorContains(t, err, "circular include detected")
	require.ErrorContains(t, err, "a.yaml")
	require.ErrorContains(t, err, "b.yaml")
	require.ErrorContains(t, err, "c.yaml")
	require.Contains(t, err.Error(), " -> ")
}

func writeDeclarationFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
