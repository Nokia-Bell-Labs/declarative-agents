// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeUIFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiffTrees(t *testing.T) {
	t.Parallel()
	a, b := t.TempDir(), t.TempDir()
	writeUIFile(t, filepath.Join(a, "index.html"), "<html>")
	writeUIFile(t, filepath.Join(a, "assets/app.js"), "console.log(1)")
	writeUIFile(t, filepath.Join(b, "index.html"), "<html>")
	writeUIFile(t, filepath.Join(b, "assets/app.js"), "console.log(1)")
	if d := diffTrees(a, b); d != "" {
		t.Fatalf("identical trees should not differ, got:\n%s", d)
	}

	// A changed byte, a missing file, and an extra file are all reported.
	writeUIFile(t, filepath.Join(b, "assets/app.js"), "console.log(2)")
	writeUIFile(t, filepath.Join(a, "only-tracked.txt"), "x")
	writeUIFile(t, filepath.Join(b, "only-rebuilt.txt"), "y")
	d := diffTrees(a, b)
	for _, want := range []string{"content differs: assets/app.js", "only in tracked dist: only-tracked.txt", "only in rebuilt dist: only-rebuilt.txt"} {
		if !strings.Contains(d, want) {
			t.Errorf("diff missing %q; got:\n%s", want, d)
		}
	}
}

func TestDiscoverShippedUIs(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// A shipped UI: lockfile + build script + tracked dist.
	app := filepath.Join(root, "agents", "chatbot", "ui", "app")
	writeUIFile(t, filepath.Join(app, "package-lock.json"), "{}")
	writeUIFile(t, filepath.Join(app, "package.json"), `{"scripts":{"build":"vite build"}}`)
	writeUIFile(t, filepath.Join(app, "dist", "index.html"), "<html>")
	// A lockfile inside node_modules must be ignored.
	writeUIFile(t, filepath.Join(app, "node_modules", "dep", "package-lock.json"), "{}")
	// A package with a lockfile but no build script is not a shipped UI.
	nob := filepath.Join(root, "agents", "tool")
	writeUIFile(t, filepath.Join(nob, "package-lock.json"), "{}")
	writeUIFile(t, filepath.Join(nob, "package.json"), `{"scripts":{"test":"jest"}}`)

	uis, err := discoverShippedUIs([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(uis) != 1 || uis[0] != app {
		t.Fatalf("discoverShippedUIs = %v, want [%s]", uis, app)
	}
}
