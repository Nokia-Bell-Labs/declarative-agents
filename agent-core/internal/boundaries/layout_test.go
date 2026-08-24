// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package boundaries

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPackageLayoutInventoryMatchesGoList(t *testing.T) {
	root := moduleRoot(t)
	listed := inventoryFromPackageLayout(t, filepath.Join(root, "package-layout.md"))
	current := modulePackagePaths(t, root)
	if violations := inventoryMismatches(listed, current); len(violations) > 0 {
		t.Errorf("package-layout.md inventory does not match go list ./...:\n  %s",
			strings.Join(violations, "\n  "))
	}
}

func TestPackageLayoutInventoryDetectsDrift(t *testing.T) {
	t.Parallel()
	listed := []string{"cmd/agent"}
	current := []string{"cmd/agent", "internal/runtime/checkpoint"}
	joined := strings.Join(inventoryMismatches(listed, current), "\n")
	require.Contains(t, joined, "internal/runtime/checkpoint")
	require.Contains(t, joined, "package-layout.md")
}

func inventoryFromPackageLayout(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	const header = "## Current Go Package Inventory"
	text := string(data)
	i := strings.Index(text, header)
	require.GreaterOrEqual(t, i, 0, "missing %s in %s", header, path)
	rest := text[i+len(header):]
	if j := strings.Index(rest, "\n## "); j >= 0 {
		rest = rest[:j]
	}
	var out []string
	for _, line := range strings.Split(rest, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- `") || !strings.HasSuffix(line, "`") {
			continue
		}
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(line, "- `"), "`"))
	}
	require.NotEmpty(t, out, "no inventory entries in %s", path)
	return out
}

func modulePackagePaths(t *testing.T, root string) []string {
	t.Helper()
	cmd := exec.Command("go", "list", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err, "go list ./...")
	module := modulePath(t, root)
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		paths = append(paths, relImport(module, line))
	}
	sort.Strings(paths)
	return paths
}

func inventoryMismatches(listed, current []string) []string {
	want := map[string]bool{}
	got := map[string]bool{}
	for _, p := range listed {
		want[p] = true
	}
	for _, p := range current {
		got[p] = true
	}
	var violations []string
	for p := range got {
		if !want[p] {
			violations = append(violations, "package-layout.md missing "+p)
		}
	}
	for p := range want {
		if !got[p] {
			violations = append(violations, "package-layout.md stale entry "+p)
		}
	}
	sort.Strings(violations)
	return violations
}
