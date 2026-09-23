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

// The asset file set is the declared roots, every declaration the loader
// visited, and every file under a declared config directory, sorted.
func TestProgramAssetFilesFromVisitedIncludesRootsVisitsAndConfigDirectories(t *testing.T) {
	paths, files := writeProgramRefFixture(t)
	visited := []string{files["included"]}

	got, err := ProgramAssetFilesFromVisited(paths, visited)
	require.NoError(t, err)
	require.True(t, sort.StringsAreSorted(got))
	for _, path := range []string{
		paths.Profile, paths.Machine, paths.ToolSelections[0], paths.ToolDeclarations[0],
		paths.RESTDefinitions[0], files["included"], files["tool_config"], files["rest_config"],
	} {
		require.Contains(t, got, canonicalProgramPath(path))
	}
}

func TestProgramAssetFilesFromVisitedReportsMissingConfigDirectory(t *testing.T) {
	paths, _ := writeProgramRefFixture(t)
	paths.ToolConfigDirs = []string{filepath.Join(t.TempDir(), "absent")}
	_, err := ProgramAssetFilesFromVisited(paths, nil)
	require.Error(t, err)
}

// The reference hashes canonical paths with the captured bytes: the same
// program named relatively or absolutely has one identity, and any byte change
// in any asset changes the digest.
func TestBuildProgramRefFromAssetsIsCanonicalAndBoundToBytes(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "profile.yaml")
	assets := map[string][]byte{
		profile:                            []byte("name: fixture\n"),
		filepath.Join(dir, "machine.yaml"): []byte("name: fixture\n"),
	}
	first := BuildProgramRefFromAssets(profile, assets)
	require.Equal(t, first, BuildProgramRefFromAssets(profile, assets))
	require.Equal(t, canonicalProgramPath(profile), first.Profile)

	relative, err := filepath.Rel(mustGetwd(t), dir)
	require.NoError(t, err)
	renamed := map[string][]byte{
		filepath.Join(relative, "profile.yaml"): assets[profile],
		filepath.Join(relative, "machine.yaml"): assets[filepath.Join(dir, "machine.yaml")],
	}
	require.Equal(t, first, BuildProgramRefFromAssets(filepath.Join(relative, "profile.yaml"), renamed))

	assets[filepath.Join(dir, "machine.yaml")] = []byte("name: fixture\n# drift\n")
	require.NotEqual(t, first.Digest, BuildProgramRefFromAssets(profile, assets).Digest)
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	require.NoError(t, err)
	return wd
}

func writeProgramRefFixture(t *testing.T) (ProgramPaths, map[string]string) {
	t.Helper()
	dir := t.TempDir()
	toolDir := filepath.Join(dir, "tool-config")
	restDir := filepath.Join(dir, "rest-config")
	require.NoError(t, os.MkdirAll(toolDir, 0o755))
	require.NoError(t, os.MkdirAll(restDir, 0o755))
	included := writeProgramRefFile(t, filepath.Join(dir, "included.yaml"), `unit: included
tools:
  - {name: included, type: exec, binary: "true"}
`)
	declaration := writeProgramRefFile(t, filepath.Join(dir, "declarations.yaml"), `unit: declarations
imports: [included.yaml]
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
