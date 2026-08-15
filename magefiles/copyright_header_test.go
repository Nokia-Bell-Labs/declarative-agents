// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCopyrightHeaderGoYAMLMarkdown(t *testing.T) {
	cases := []struct {
		ext     string
		content string
	}{
		{".go", "// Copyright (c) 2026 Nokia\n// SPDX-License-Identifier: BSD-3-Clause\n\npackage demo\n"},
		{".yaml", "# Copyright (c) 2026 Nokia\n# SPDX-License-Identifier: BSD-3-Clause\nname: demo\n"},
		{".yml", "# Copyright (c) 2026 Nokia\n# SPDX-License-Identifier: BSD-3-Clause\nversion: \"2\"\n"},
		{".md", "<!-- Copyright (c) 2026 Nokia -->\n<!-- SPDX-License-Identifier: BSD-3-Clause -->\n\n# Demo\n"},
	}
	for _, tc := range cases {
		if err := checkCopyrightHeader([]byte(tc.content), tc.ext); err != nil {
			t.Errorf("%s: %v", tc.ext, err)
		}
	}
}

func TestCheckCopyrightHeaderMarkdownFrontmatter(t *testing.T) {
	content := "---\ntitle: Demo\n---\n<!-- Copyright (c) 2026 Nokia -->\n<!-- SPDX-License-Identifier: BSD-3-Clause -->\n\n# Body\n"
	if err := checkCopyrightHeader([]byte(content), ".md"); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCopyrightHeaderRejectsOldAndMITForms(t *testing.T) {
	rejects := []struct {
		ext     string
		content string
	}{
		{".go", "// Copyright (c) 2026 Nokia. All" + " rights reserved.\n\npackage demo\n"},
		{".go", "// Copyright (c) 2026 Petar Djukic. All" + " rights reserved.\n// SPDX-License-Identifier: " + "MIT\n\npackage demo\n"},
		{".yaml", "# Copyright (c) 2026 Nokia. All" + " rights reserved.\nname: demo\n"},
		{".md", "<!-- Copyright (c) 2026 Nokia. All" + " rights reserved. -->\n\n# Demo\n"},
		{".md", "# Demo\n"},
	}
	for _, tc := range rejects {
		if err := checkCopyrightHeader([]byte(tc.content), tc.ext); err == nil {
			t.Errorf("%s accepted non-canonical header:\n%s", tc.ext, tc.content)
		}
	}
}

func TestEveryTrackedSourceHasCanonicalCopyrightHeader(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if err := checkTrackedCopyrightHeaders(root); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCopyrightHeaderFrontmatterWithoutCloserFails(t *testing.T) {
	if err := checkCopyrightHeader([]byte("---\ntitle: Demo\n"), ".md"); err == nil {
		t.Fatal("expected error for unclosed frontmatter")
	}
}

func TestTrackedCopyrightPathsOnlyListsSupportedExtensions(t *testing.T) {
	root, err := findRepositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	paths, err := trackedCopyrightPaths(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("expected tracked go/yaml/md files")
	}
	for _, rel := range paths {
		ext := strings.ToLower(filepath.Ext(rel))
		if _, ok := copyrightHeaderByExt[ext]; !ok {
			t.Errorf("unexpected path %s", rel)
		}
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Errorf("missing %s: %v", rel, err)
		}
	}
}
