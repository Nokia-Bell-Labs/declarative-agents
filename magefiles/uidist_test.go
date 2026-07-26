// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
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

func TestAuditUIDependenciesChecksBuildAndProductionScopes(t *testing.T) {
	var calls []string
	run := func(dir, name string, args ...string) error {
		calls = append(calls, dir+" "+name+" "+strings.Join(args, " "))
		return nil
	}
	if err := auditUIDependencies("/ui", run); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/ui npm audit --audit-level=high",
		"/ui npm audit --omit=dev --audit-level=high",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("audit calls = %v, want %v", calls, want)
	}
}

func TestRebuildAndDiffUIStopsOnHighBuildAudit(t *testing.T) {
	app := t.TempDir()
	writeUIFile(t, filepath.Join(app, "package.json"), `{"scripts":{"build":"vite build"}}`)
	writeUIFile(t, filepath.Join(app, "package-lock.json"), "{}")
	writeUIFile(t, filepath.Join(app, "dist", "index.html"), "<html>")
	auditErr := errors.New("high vulnerability")
	buildCalled := false
	run := func(_ string, _ string, args ...string) error {
		command := strings.Join(args, " ")
		switch command {
		case "ci":
			return nil
		case "audit --audit-level=high":
			return auditErr
		case "run build":
			buildCalled = true
		}
		return nil
	}
	err := rebuildAndDiffUIWithRunner(app, run)
	if !errors.Is(err, auditErr) ||
		!strings.Contains(err.Error(), "zero high/critical") {
		t.Fatalf("error = %v, want audit policy failure", err)
	}
	if buildCalled {
		t.Fatal("UI build ran after vulnerable build dependency audit")
	}
}
