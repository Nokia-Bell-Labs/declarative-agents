// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadToolDefsAllowsImportDiamond(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeDeclarationFixture(t, dir, "leaf.yaml", "unit: leaf\ntools:\n- {name: shared, binary: echo}\n")
	writeDeclarationFixture(t, dir, "left.yaml", "unit: left\nimports: [leaf.yaml]\ntools: []\n")
	writeDeclarationFixture(t, dir, "right.yaml", "unit: right\nimports: [leaf.yaml]\ntools: []\n")
	root := writeDeclarationFixture(
		t, dir, "root.yaml", "unit: root\nimports: [left.yaml, right.yaml]\ntools: []\n",
	)

	defs, err := LoadToolDefs(root)
	require.NoError(t, err)
	require.Len(t, defs, 1)
	require.Equal(t, "shared", defs[0].Name)
}

func TestLoadToolDefsReportsTrueImportChain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	a := writeDeclarationFixture(t, dir, "a.yaml", "unit: a\nimports: [b.yaml]\ntools: []\n")
	writeDeclarationFixture(t, dir, "b.yaml", "unit: b\nimports: [c.yaml]\ntools: []\n")
	writeDeclarationFixture(t, dir, "c.yaml", "unit: c\nimports: [a.yaml]\ntools: []\n")

	_, err := LoadToolDefs(a)
	require.ErrorContains(t, err, "tool import cycle")
	require.ErrorContains(t, err, "a.yaml")
	require.ErrorContains(t, err, "b.yaml")
	require.ErrorContains(t, err, "c.yaml")
	require.Contains(t, err.Error(), " -> ")
}

func TestLoadToolDeclarationClosureReadsEachSourceOnce(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	leaf := writeDeclarationFixture(t, dir, "leaf.yaml", "unit: leaf\ntools:\n- {name: shared, binary: echo}\n")
	writeDeclarationFixture(t, dir, "all.yaml", "unit: all\nimports: [leaf.yaml]\ntools: []\n")
	visits := map[string]int{}

	fromDirs, explicit, err := LoadToolDeclarationClosure(
		[]string{dir}, []string{leaf},
		func(path string, _ []byte) error {
			visits[canonicalProgramPath(path)]++
			return nil
		},
	)

	require.NoError(t, err)
	require.NotEmpty(t, fromDirs)
	require.NotEmpty(t, explicit)
	require.Equal(t, 1, visits[canonicalProgramPath(leaf)])
	for path, count := range visits {
		require.Equal(t, 1, count, path)
	}
}

func writeDeclarationFixture(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
