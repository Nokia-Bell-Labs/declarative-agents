// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package doltsql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDoltCommitLiteralsLiveOnlyHere(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	var others []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "testdata", "magefiles":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == filepath.Join("internal", "doltsql", "statements.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "CALL DOLT_COMMIT") {
			others = append(others, rel)
		}
		return nil
	}))
	require.Empty(t, others, "CALL DOLT_COMMIT must live only in internal/doltsql/statements.go")
}

func TestDoltDriverRegistersOnce(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	var registers []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "node_modules", "testdata", "magefiles":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), `sql.Register("dolt"`) {
			registers = append(registers, rel)
		}
		return nil
	}))
	require.Equal(t, []string{filepath.Join("internal", "doltsql", "driver.go")}, registers)
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found")
		dir = parent
	}
}
