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
	writeUIFile(t, filepath.Join(app, "package.json"), `{"scripts":{"dev":"vite","build":"vite build","preview":"vite preview"}}`)
	writeUIFile(t, filepath.Join(app, "dist", "index.html"), "<html>")
	// A lockfile inside node_modules must be ignored.
	writeUIFile(t, filepath.Join(app, "node_modules", "dep", "package-lock.json"), "{}")

	uis, err := discoverShippedUIs([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(uis) != 1 || uis[0] != app {
		t.Fatalf("discoverShippedUIs = %v, want [%s]", uis, app)
	}
}

func TestDiscoverShippedUIsRejectsScriptOrDistDrift(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		scripts string
		dist    bool
		want    string
	}{
		{name: "missing preview", scripts: `{"dev":"vite","build":"vite build"}`, dist: true, want: `"preview" script`},
		{name: "missing dist", scripts: `{"dev":"vite","build":"vite build","preview":"vite preview"}`, want: "no tracked dist tree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			app := filepath.Join(root, "ui")
			writeUIFile(t, filepath.Join(app, "package-lock.json"), "{}")
			writeUIFile(t, filepath.Join(app, "package.json"), `{"scripts":`+tc.scripts+`}`)
			if tc.dist {
				writeUIFile(t, filepath.Join(app, "dist", "index.html"), "<html>")
			}
			_, err := discoverShippedUIs([]string{root})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("discover error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestRepositoryShippedUIDiscovery(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	var roots []string
	for _, root := range uiSearchRoots {
		roots = append(roots, filepath.Join(repoRoot, filepath.FromSlash(root)))
	}
	got, err := discoverShippedUIs(roots)
	if err != nil {
		t.Fatal(err)
	}
	wantRel := []string{
		"applications/catalog/agents/bench/ui",
		"applications/catalog/agents/collector/ui",
		"applications/catalog/agents/knowledge-manager/documentation-curator/ui/docs",
		"applications/catalog/agents/knowledge-manager/documentation-curator/ui/monitor",
		"applications/chatbot-mesh/agents/chatbot/ui/app",
		"applications/chatbot-mesh/agents/observer/ui",
	}
	var want []string
	for _, path := range wantRel {
		want = append(want, filepath.Join(repoRoot, filepath.FromSlash(path)))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("discoverShippedUIs = %v, want %v", got, want)
	}
}

func TestShippedUIsImportCanonicalDesignTokens(t *testing.T) {
	repoRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Clean(filepath.Join(repoRoot, filepath.FromSlash(canonicalUITokensPath)))
	for _, rel := range []string{
		"applications/catalog/agents/bench/ui",
		"applications/catalog/agents/collector/ui",
		"applications/catalog/agents/knowledge-manager/documentation-curator/ui/docs",
		"applications/catalog/agents/knowledge-manager/documentation-curator/ui/monitor",
		"applications/chatbot-mesh/agents/chatbot/ui/app",
		"applications/chatbot-mesh/agents/observer/ui",
	} {
		app := filepath.Join(repoRoot, filepath.FromSlash(rel))
		data, err := os.ReadFile(filepath.Join(app, "src", "App.css"))
		if err != nil {
			t.Fatal(err)
		}
		first := strings.SplitN(string(data), "\n", 2)[0]
		const prefix = `@import "`
		if !strings.HasPrefix(first, prefix) || !strings.HasSuffix(first, `";`) {
			t.Errorf("%s: first CSS line does not import canonical tokens: %q", rel, first)
			continue
		}
		imported := strings.TrimSuffix(strings.TrimPrefix(first, prefix), `";`)
		resolved := filepath.Clean(filepath.Join(app, "src", filepath.FromSlash(imported)))
		if resolved != canonical {
			t.Errorf("%s: token import resolves to %s, want %s", rel, resolved, canonical)
		}
		if strings.Contains(string(data), "--bg-primary:") {
			t.Errorf("%s: App.css contains a copied canonical token declaration", rel)
		}
	}
}

func TestUISearchRootsCoverCatalogAndRunnableApplications(t *testing.T) {
	want := []string{
		"applications/catalog",
		"applications/coding-agent",
		"applications/chatbot-mesh",
	}
	if !reflect.DeepEqual(uiSearchRoots, want) {
		t.Fatalf("uiSearchRoots = %#v, want %#v", uiSearchRoots, want)
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

func TestUIDistPrerequisitePolicies(t *testing.T) {
	found := func(string) (string, error) { return "/usr/bin/npm", nil }
	missing := func(string) (string, error) { return "", errors.New("not found") }

	t.Run("present npm proceeds regardless of mode", func(t *testing.T) {
		for _, release := range []bool{false, true} {
			proceed, err := uiDistPrerequisite(found, release)
			if err != nil || !proceed {
				t.Fatalf("release=%v: proceed=%v err=%v, want true/nil", release, proceed, err)
			}
		}
	})

	t.Run("missing npm outside release skips cleanly", func(t *testing.T) {
		proceed, err := uiDistPrerequisite(missing, false)
		if err != nil {
			t.Fatalf("err = %v, want clean skip", err)
		}
		if proceed {
			t.Fatal("proceed = true, want skip when npm is absent outside a release")
		}
	})

	t.Run("missing npm in release is fatal", func(t *testing.T) {
		proceed, err := uiDistPrerequisite(missing, true)
		if err == nil {
			t.Fatal("err = nil, want a fatal release-gate failure when npm is absent")
		}
		if proceed {
			t.Fatal("proceed = true after a fatal prerequisite error")
		}
		if !strings.Contains(err.Error(), uiDistReleaseEnv) {
			t.Fatalf("error = %v, want it to reference %s", err, uiDistReleaseEnv)
		}
	})
}

func TestReleaseModeEnabledReadsEnv(t *testing.T) {
	t.Setenv(uiDistReleaseEnv, "")
	if releaseModeEnabled() {
		t.Fatal("release mode enabled with empty env")
	}
	t.Setenv(uiDistReleaseEnv, "1")
	if !releaseModeEnabled() {
		t.Fatalf("release mode not enabled with %s=1", uiDistReleaseEnv)
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

func TestRebuildAndDiffUIRunsDeclaredTestsBeforeBuild(t *testing.T) {
	app := t.TempDir()
	writeUIFile(t, filepath.Join(app, "package.json"),
		`{"scripts":{"dev":"vite","build":"vite build","preview":"vite preview","test":"node --test"}}`)
	writeUIFile(t, filepath.Join(app, "package-lock.json"), "{}")
	writeUIFile(t, filepath.Join(app, "dist", "index.html"), "<html>")
	var calls []string
	run := func(dir, _ string, args ...string) error {
		calls = append(calls, strings.Join(args, " "))
		if strings.Join(args, " ") == "run build" {
			writeUIFile(t, filepath.Join(dir, "dist", "index.html"), "<html>")
		}
		return nil
	}
	if err := rebuildAndDiffUIWithRunner(app, run); err != nil {
		t.Fatal(err)
	}
	want := []string{"ci", "audit --audit-level=high", "audit --omit=dev --audit-level=high", "test", "run build"}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("runner calls = %v, want %v", calls, want)
	}
}
