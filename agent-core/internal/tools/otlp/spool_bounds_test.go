// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
)

func TestSpoolFactoriesRejectByteCapWithoutRetainedGeneration(t *testing.T) {
	t.Parallel()

	factories := map[string]toolregistry.BuiltinFactory{
		InitSpoolSpans:   spoolFactory(),
		InitSpoolMetrics: spoolMetricsFactory(),
	}
	for name, factory := range factories {
		for _, maxFiles := range []int{0, 1} {
			_, err := factory(catalog.ToolDef{
				Name: name,
				Config: map[string]interface{}{
					"path": "evidence.ndjson", "max_bytes": 1, "max_files": maxFiles,
				},
			}, nil)
			require.ErrorContains(t, err, "max_files must be at least 2 when max_bytes is set")
		}
	}
}

func TestRotateRejectsSingleGenerationWithoutDeletingEvidence(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "evidence.ndjson")
	original := []byte("{\"accepted\":true}\n")
	require.NoError(t, os.WriteFile(path, original, 0o644))

	err := rotateBeforeAppend(SpoolConfig{Path: path, MaxBytes: 1, MaxFiles: 1}, 10)

	require.ErrorContains(t, err, "max_files must be at least 2 when max_bytes is set")
	retained, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, original, retained)
}
