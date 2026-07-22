// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func findMapping(mappings []ParamMapping, name string) *ParamMapping {
	for i := range mappings {
		if mappings[i].Name == name {
			return &mappings[i]
		}
	}
	return nil
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)
	return data
}
