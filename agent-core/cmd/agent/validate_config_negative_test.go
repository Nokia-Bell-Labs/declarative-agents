// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The OTLP replay fixture is typed, so the selector checks run end to end in
// the core suite: a copy corrupted one way fails --validate-config naming the
// fault, and the uncorrupted copy passes, so each failure is the corruption's
// (GH-2108).
func TestValidateConfigRejectsSelectorFaultsInTypedCoreFixture(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })

	for _, test := range []struct {
		name, file, from, to string
		want                 []string
	}{
		{name: "unchanged"},
		{
			name: "misspelled field", file: "declarations.yaml",
			from: "$from(loaded_batch).batch", to: "$from(loaded_batch).batch_nope",
			want: []string{"selector path mismatches", `closest declared field is "batch"`},
		},
		{
			name: "renamed label", file: "machine.yaml",
			from: "label: loaded_batch", to: "label: loaded_bundle",
			want: []string{"unresolved selector labels", "loaded_batch"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := copyOTLPReplayFixture(t)
			if test.file != "" {
				replaceInFixture(t, filepath.Join(filepath.Dir(profile), test.file), test.from, test.to)
			}
			clearAgentFlags()
			flagProfile = profile
			flagValidateConfig = true

			_, err := captureStderr(t, func() error { return run(rootCmd, nil) })
			if len(test.want) == 0 {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			for _, want := range test.want {
				require.Contains(t, err.Error(), want)
			}
		})
	}
}

// copyOTLPReplayFixture copies the fixture beside the original, inside
// testdata/integration/profiles, so its relative unit import still resolves.
func copyOTLPReplayFixture(t *testing.T) string {
	t.Helper()
	source := filepath.Dir(profilePathFromTest(t, "otlp-replay/profile.yaml"))
	target, err := os.MkdirTemp(filepath.Dir(source), "otlp-replay-negative-")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, os.RemoveAll(target)) })
	entries, err := os.ReadDir(source)
	require.NoError(t, err)
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(source, entry.Name()))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(target, entry.Name()), data, 0o644))
	}
	return filepath.Join(target, "profile.yaml")
}

func replaceInFixture(t *testing.T, path, from, to string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), from, "fixture text moved")
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), from, to, 1)), 0o644))
}
