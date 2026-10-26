// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package release

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// The profiles archive holds the profiles, the agent-core tools tree, and
// the release, and a rebuild writes the same bytes (GH-34).
func TestProfilesArchiveIsTheTreesAndTheRelease(t *testing.T) {
	root := t.TempDir()
	core := coreFixture(root)
	a, b := filepath.Join(t.TempDir(), "a.tar.gz"), filepath.Join(t.TempDir(), "b.tar.gz")
	for _, p := range []string{a, b} {
		if err := WriteProfilesArchive(p, filepath.Join(root, "plugin", "agents"), filepath.Join(core, "tools"), "v1"); err != nil {
			t.Fatal(err)
		}
	}
	da, _ := os.ReadFile(a)
	db, _ := os.ReadFile(b)
	if !bytes.Equal(da, db) {
		t.Fatal("two builds of the archive differ")
	}
	gz, err := gzip.NewReader(bytes.NewReader(da))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	var names []string
	var release string
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, h.Name)
		if h.Name == "release" {
			body, _ := io.ReadAll(tr)
			release = string(body)
		}
	}
	if !slices.Equal(names, []string{"agent-core/tools/builtin/done.yaml", "agents/pop/profile.yaml", "release"}) || release != "v1\n" {
		t.Fatalf("entries %v, release %q", names, release)
	}
}
