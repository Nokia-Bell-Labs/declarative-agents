// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildProgramRefTracksCompleteDeclarationClosure(t *testing.T) {
	paths, files := writeProgramRefFixture(t)

	first, err := BuildProgramRef(paths)
	require.NoError(t, err)
	second, err := BuildProgramRef(paths)
	require.NoError(t, err)
	require.Equal(t, first, second)
	require.Equal(t, canonicalProgramPath(paths.Profile), first.Profile)

	for _, name := range []string{"included", "tool_config", "rest_config"} {
		t.Run(name, func(t *testing.T) {
			original, err := os.ReadFile(files[name])
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, os.WriteFile(files[name], original, 0o600))
			})
			require.NoError(t, os.WriteFile(files[name], append(original, []byte("\n# digest drift\n")...), 0o600))

			changed, err := BuildProgramRef(paths)
			require.NoError(t, err)
			require.NotEqual(t, first.Digest, changed.Digest)
		})
	}
}

func TestProgramAssetFilesIncludesSortedClosurePaths(t *testing.T) {
	paths, files := writeProgramRefFixture(t)

	got, err := ProgramAssetFiles(paths)
	require.NoError(t, err)
	require.True(t, sort.StringsAreSorted(got))
	for _, name := range []string{"included", "tool_config", "rest_config"} {
		require.Contains(t, got, canonicalProgramPath(files[name]))
	}
}

func TestBuildProgramRefRejectsDeclarationIncludeCycle(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.yaml")
	second := filepath.Join(dir, "second.yaml")
	writeProgramRefFile(t, first, "includes: [second.yaml]\ntools: []\n")
	writeProgramRefFile(t, second, "includes: [first.yaml]\ntools: []\n")

	_, err := BuildProgramRef(ProgramPaths{
		Profile:          writeProgramRefFile(t, filepath.Join(dir, "profile.yaml"), "name: cycle\n"),
		Machine:          writeProgramRefFile(t, filepath.Join(dir, "machine.yaml"), "name: cycle\n"),
		ToolDeclarations: []string{first},
	})
	require.ErrorContains(t, err, "circular include detected")
}

func TestBuildProgramRefReportsMissingAsset(t *testing.T) {
	dir := t.TempDir()
	_, err := BuildProgramRef(ProgramPaths{
		Profile: writeProgramRefFile(t, filepath.Join(dir, "profile.yaml"), "name: missing\n"),
		Machine: filepath.Join(dir, "missing-machine.yaml"),
	})
	require.ErrorContains(t, err, "read program asset")
	require.ErrorContains(t, err, "missing-machine.yaml")
}

func writeProgramRefFixture(t *testing.T) (ProgramPaths, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tool-config")
	restDir := filepath.Join(dir, "rest-config")
	require.NoError(t, os.MkdirAll(toolDir, 0o755))
	require.NoError(t, os.MkdirAll(restDir, 0o755))
	included := writeProgramRefFile(t, filepath.Join(dir, "included.yaml"), `tools:
  - {name: included, type: exec, binary: "true"}
`)
	declaration := writeProgramRefFile(t, filepath.Join(dir, "declarations.yaml"), `includes: [included.yaml]
tools: []
`)
	toolConfig := writeProgramRefFile(t, filepath.Join(toolDir, "tool.yaml"), "tools: []\n")
	restConfig := writeProgramRefFile(t, filepath.Join(restDir, "rest.yaml"), "clients: {}\n")
	return ProgramPaths{
			Profile:          writeProgramRefFile(t, filepath.Join(dir, "profile.yaml"), "name: fixture\n"),
			Machine:          writeProgramRefFile(t, filepath.Join(dir, "machine.yaml"), "name: fixture\n"),
			ToolSelections:   []string{writeProgramRefFile(t, filepath.Join(dir, "tools.yaml"), "tools: [included]\n")},
			ToolDeclarations: []string{declaration},
			ToolConfigDirs:   []string{toolDir},
			RESTDefinitions:  []string{writeProgramRefFile(t, filepath.Join(dir, "rest.yaml"), "servers: {}\n")},
			RESTConfigDirs:   []string{restDir},
		}, map[string]string{
			"included": included, "tool_config": toolConfig, "rest_config": restConfig,
		}
}

func writeProgramRefFile(t *testing.T, path, content string) string {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
