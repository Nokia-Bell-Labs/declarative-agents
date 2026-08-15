// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package conformance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// manifestFixtureRow captures a `name/` value from the integration manifest's
// Fixture column so the classification can be compared against the directory
// tree.
var manifestFixtureRow = regexp.MustCompile("^\\|\\s*`([^`]+?)/`\\s*\\|")

// TestIntegrationManifestClassifiesEveryFixture is the authoritative-manifest
// guard: the README in testdata/integration claims every fixture has exactly
// one classified entry, so every immediate fixture directory must appear as a
// row and every row must name a real directory. This catches a fixture added
// without a manifest entry (an unclassified consumer) and a manifest row left
// behind after its fixture was deleted (a stale claim). See GH-1351.
func TestIntegrationManifestClassifiesEveryFixture(t *testing.T) {
	t.Parallel()
	integrationRoot := ProfilePath(filepath.Join("testdata", "integration"))

	manifest, err := os.ReadFile(filepath.Join(integrationRoot, "README.md"))
	if err != nil {
		t.Fatalf("read integration manifest: %v", err)
	}

	classified := map[string]bool{}
	for _, line := range strings.Split(string(manifest), "\n") {
		if m := manifestFixtureRow.FindStringSubmatch(line); m != nil {
			classified[m[1]] = true
		}
	}
	if len(classified) == 0 {
		t.Fatal("parsed no fixture rows from the manifest table; the row format may have changed")
	}

	entries, err := os.ReadDir(integrationRoot)
	if err != nil {
		t.Fatalf("read integration fixtures: %v", err)
	}
	present := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			present[entry.Name()] = true
		}
	}

	for dir := range present {
		if !classified[dir] {
			t.Errorf("fixture directory %q is not classified in testdata/integration/README.md; "+
				"add a manifest row naming its single consumer or delete the fixture", dir)
		}
	}
	for row := range classified {
		if !present[row] {
			t.Errorf("manifest row %q names no fixture directory under testdata/integration; "+
				"the table must not carry a stale entry", row)
		}
	}
}
