// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	internalload "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/load"
)

var updateDumpGoldens = flag.Bool("update", false, "update canonical config dump goldens")

func TestDumpGolden(t *testing.T) {
	t.Setenv("OTLP_REPLAY_FILE", "traces/replay.otlp.json")
	t.Setenv("OTLP_REPLAY_ENDPOINT", "127.0.0.1:4317")
	root := repoRootFromTest(t)
	profileRoot := filepath.Join(root, "testdata", "integration", "profiles")
	entries, err := os.ReadDir(profileRoot)
	require.NoError(t, err)
	var profiles []string
	for _, entry := range entries {
		dir := filepath.Join(profileRoot, entry.Name())
		if !entry.IsDir() || !regularTestFile(filepath.Join(dir, "profile.yaml")) {
			continue
		}
		templates, globErr := filepath.Glob(filepath.Join(dir, "*.tmpl"))
		require.NoError(t, globErr)
		if len(templates) == 0 {
			profiles = append(profiles, entry.Name())
		}
	}
	sort.Strings(profiles)
	require.NotEmpty(t, profiles)

	for _, name := range profiles {
		t.Run(name, func(t *testing.T) {
			profile := filepath.Join(profileRoot, name, "profile.yaml")
			closure, loadErr := internalload.LoadClosure(profile, internalload.Options{})
			require.NoError(t, loadErr)
			var first, second bytes.Buffer
			require.NoError(t, dumpConfig(closure, &first))
			require.NoError(t, dumpConfig(closure, &second))
			require.Equal(t, first.Bytes(), second.Bytes())

			actual := normalizeDumpGolden(first.Bytes(), root)
			golden := filepath.Join("testdata", "dump", name+".golden")
			if *updateDumpGoldens {
				require.NoError(t, os.MkdirAll(filepath.Dir(golden), 0o755))
				require.NoError(t, os.WriteFile(golden, actual, 0o644))
			}
			expected, readErr := os.ReadFile(golden)
			require.NoError(t, readErr,
				"regenerate with: go test ./cmd/agent -run TestDumpGolden -update")
			require.Equal(t, string(expected), string(actual))
		})
	}
}

func TestDumpConfigRunPath(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })
	clearAgentFlags()
	flagProfile = profilePathFromTest(t, "control/profile.yaml")
	flagDumpConfig = true
	cmd := &cobra.Command{}
	var output bytes.Buffer
	cmd.SetOut(&output)

	require.NoError(t, run(cmd, nil))
	require.Contains(t, output.String(), "dump_version: 1")
	require.Contains(t, output.String(), "profile:")
	require.Contains(t, output.String(), repoRootFromTest(t))

	flagProfile = filepath.Join(t.TempDir(), "missing.yaml")
	require.ErrorContains(t, run(cmd, nil), "load profile")

	flagProfile = profilePathFromTest(t, "control/profile.yaml")
	flagValidateConfig = true
	require.ErrorContains(t, run(cmd, nil), "cannot be used together")
	usage := rootCmd.PersistentFlags().Lookup("dump-config").Usage
	require.Contains(t, usage, "environment-expanded")
	require.Contains(t, usage, "secrets")
}

func regularTestFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func normalizeDumpGolden(data []byte, root string) []byte {
	text := strings.ReplaceAll(string(data), filepath.Clean(root), "<agent-core>")
	lines := strings.Split(text, "\n")
	for index, line := range lines {
		if strings.Contains(line, "<agent-core>") {
			lines[index] = strings.ReplaceAll(line, `\`, "/")
		}
	}
	return []byte(strings.Join(lines, "\n"))
}
