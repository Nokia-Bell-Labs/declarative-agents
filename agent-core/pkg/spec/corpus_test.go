// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadCorpus_Valid(t *testing.T) {
	c, err := LoadCorpus(filepath.Join("testdata", "valid"))
	require.NoError(t, err)

	assert.Len(t, c.SRDs, 3)
	assert.Contains(t, c.SRDs, "srd001-auth")
	assert.Contains(t, c.SRDs, "srd002-api")
	assert.Contains(t, c.SRDs, "srd003-storage")
	assert.Equal(t, []string{"srd001-auth", "srd002-api", "srd003-storage"}, c.SRDOrder)

	assert.Len(t, c.UseCases, 1)
	assert.Contains(t, c.UseCases, "rel00.0-uc001-login")

	assert.Len(t, c.TestSuites, 1)
	assert.Contains(t, c.TestSuites, "test-rel00.0")

	assert.Len(t, c.Roadmap.Releases, 2)
	assert.Len(t, c.SpecIndex.SRDIndex, 3)
}

func TestLoadCorpus_Machines(t *testing.T) {
	c, err := LoadCorpus(filepath.Join("testdata", "valid"))
	require.NoError(t, err)

	require.Contains(t, c.Machines, "test-agent")
	ms := c.Machines["test-agent"]
	assert.Equal(t, "test-agent", ms.Name)
	assert.Equal(t, "Idle", ms.InitialState)
	assert.Len(t, ms.States, 4)
	assert.Len(t, ms.Signals, 3)
	assert.Len(t, ms.Transitions, 3)

	require.Contains(t, c.ToolSelections, "test-agent")
	assert.Equal(t, []string{"do_work"}, c.ToolSelections["test-agent"])

	assert.Contains(t, c.MachineOrder, "test-agent")
}

func TestLoadCorpus_ToolDeclarations(t *testing.T) {
	c, err := LoadCorpus(filepath.Join("testdata", "valid"))
	require.NoError(t, err)

	require.Contains(t, c.ToolDeclarations, "do_work")
	td := c.ToolDeclarations["do_work"]
	assert.Equal(t, "word", td.Category)
	assert.Equal(t, []string{"ToolDone", "CommandError"}, td.Emits)
	assert.Equal(t, "noop", td.Undo.Strategy)
	assert.Equal(t, "reversible", td.Reversibility.Classification)
	assert.Len(t, td.SideEffects.Items, 1)
	assert.Equal(t, "state_mutation", td.SideEffects.Items[0].Kind)
	assert.Equal(t, "tools/builtin.yaml", td.SourceFile)
}

func TestLoadCorpus_NoAgentsDir(t *testing.T) {
	tmp := t.TempDir()
	setupTestCorpus(t, tmp)
	writeTestSRD(t, tmp, "srd001-ok.yaml", `id: srd-ok
title: OK
problem: test
requirements:
  R1:
    title: Stuff
    items:
      - R1.1: Do something.
`)

	c, err := LoadCorpus(tmp)
	require.NoError(t, err)
	assert.Empty(t, c.Machines, "no agents dir should produce empty machines map")
}

func TestLoadCorpus_NoDocsDir(t *testing.T) {
	_, err := LoadCorpus(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "docs directory not found")
}

func TestLoadCorpus_NoSRDFiles(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs", "specs", "software-requirements"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs", "road-map.yaml"), []byte("id: test\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs", "SPECIFICATIONS.yaml"), []byte("id: test\n"), 0o644))

	_, err := LoadCorpus(tmp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no SRD files found")
}

func TestLoadCorpus_InvalidDependsOn(t *testing.T) {
	tmp := t.TempDir()
	setupTestCorpus(t, tmp)
	writeTestSRD(t, tmp, "srd001-bad.yaml", `id: srd-bad
title: Bad
problem: test
requirements:
  R1:
    title: Stuff
    items:
      - R1.1: Do something.
depends_on:
  - srd_id: nonexistent-srd
    symbols_used: [Foo]
`)

	_, err := LoadCorpus(tmp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nonexistent-srd")
}

func TestLoadCorpus_NoUseCases(t *testing.T) {
	tmp := t.TempDir()
	setupTestCorpus(t, tmp)
	writeTestSRD(t, tmp, "srd001-ok.yaml", `id: srd-ok
title: OK
problem: test
requirements:
  R1:
    title: Stuff
    items:
      - R1.1: Do something.
`)

	c, err := LoadCorpus(tmp)
	require.NoError(t, err)
	assert.Empty(t, c.UseCases)
	assert.Empty(t, c.TestSuites)
}

func setupTestCorpus(t *testing.T, root string) {
	t.Helper()
	for _, d := range []string{
		filepath.Join("docs", "specs", "software-requirements"),
		filepath.Join("docs", "specs", "use-cases"),
		filepath.Join("docs", "specs", "test-suites"),
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "road-map.yaml"),
		[]byte("id: test\ntitle: Test\nreleases: []\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "SPECIFICATIONS.yaml"),
		[]byte("id: test\ntitle: Test\n"), 0o644))
}

func writeTestSRD(t *testing.T, root, filename, data string) {
	t.Helper()
	path := filepath.Join(root, "docs", "specs", "software-requirements", filename)
	require.NoError(t, os.WriteFile(path, []byte(data), 0o644))
}
