// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTrackedYAMLRejectsUnimplementedRESTDrainPolicy keeps shipped REST YAML and
// fixtures on values validateShutdownConfig accepts (srd029 AC4, GH-1774). The
// ollamaMonitor templates used drain, which the runtime rejects, so the monitor
// never bound and integration timed out on readiness instead of naming the
// unimplemented policy.
func TestTrackedYAMLRejectsUnimplementedRESTDrainPolicy(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	forbidden := "drain_policy: " + shutdownPolicyDrain
	var hits []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "generated-files":
				return filepath.SkipDir
			}
			return nil
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".tmpl") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		for i, line := range strings.Split(string(data), "\n") {
			if strings.TrimSpace(line) == forbidden {
				hits = append(hits, fmt.Sprintf("%s:%d", rel, i+1))
			}
		}
		return nil
	})
	require.NoError(t, err)
	if len(hits) > 0 {
		t.Fatalf("unimplemented REST drain_policy %q in:\n  %s\nuse %q",
			shutdownPolicyDrain, strings.Join(hits, "\n  "), shutdownPolicyDrainThenStop)
	}
}

func TestOllamaMonitorRESTFixturesValidate(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	for _, rel := range []string{
		"agent-core/magefiles/fixtures/uc008/monitor-rest.yaml.tmpl",
		"agent-core/testdata/integration/profiles/ollama-monitor/monitor-rest.yaml.tmpl",
	} {
		t.Run(rel, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
			require.NoError(t, err)
			rendered := strings.ReplaceAll(string(data), "{{ADDRESS}}", "127.0.0.1:0")
			def, err := ParseDefinition([]byte(rendered))
			require.NoError(t, err, rendered)
			require.NoError(t, ValidateDefinition(def))
		})
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", ".."))
}
