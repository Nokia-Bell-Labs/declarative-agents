// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestUIKitTarballName(t *testing.T) {
	t.Parallel()
	if got, want := uiKitTarballName("1.2.3"), "declarative-agents-ui-kit-1.2.3.tgz"; got != want {
		t.Fatalf("uiKitTarballName = %q, want %q", got, want)
	}
}

func TestUIKitVersion(t *testing.T) {
	t.Parallel()
	version, err := uiKitVersion(filepath.Join("..", uiKitDir))
	if err != nil {
		t.Fatal(err)
	}
	if version == "" {
		t.Fatal("repository kit has no version")
	}

	other := t.TempDir()
	writeUIFile(t, filepath.Join(other, "package.json"), `{"name":"other","version":"1.0.0"}`)
	if _, err := uiKitVersion(other); err == nil || !strings.Contains(err.Error(), uiKitPackageName) {
		t.Fatalf("uiKitVersion accepted a foreign package: %v", err)
	}
	unversioned := t.TempDir()
	writeUIFile(t, filepath.Join(unversioned, "package.json"), `{"name":"@declarative-agents/ui-kit"}`)
	if _, err := uiKitVersion(unversioned); err == nil {
		t.Fatal("uiKitVersion accepted a package without a version")
	}
}

func TestCheckUIKitTag(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, tagged, head string
		wantErr            bool
	}{
		{name: "untagged", tagged: "", head: "abc"},
		{name: "tag on HEAD", tagged: "abc", head: "abc"},
		{name: "tag on another commit", tagged: "def", head: "abc", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := checkUIKitTag("ui-kit/v1.0.0", tc.tagged, tc.head)
			if (err != nil) != tc.wantErr {
				t.Fatalf("checkUIKitTag error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestUIKitReleaseCommands(t *testing.T) {
	t.Parallel()
	tarball := filepath.Join("applications", "ui-kit", "out", "declarative-agents-ui-kit-1.0.0.tgz")
	got := uiKitReleaseCommands("ui-kit/v1.0.0", true, tarball)
	for _, want := range []string{
		"git tag ui-kit/v1.0.0",
		"git push origin ui-kit/v1.0.0",
		"gh release create ui-kit/v1.0.0 " + tarball,
		"releases/download/ui-kit%2Fv1.0.0/declarative-agents-ui-kit-1.0.0.tgz",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("release commands missing %q:\n%s", want, got)
		}
	}
	if tagged := uiKitReleaseCommands("ui-kit/v1.0.0", false, tarball); strings.Contains(tagged, "git tag") {
		t.Errorf("an existing tag must not be recreated:\n%s", tagged)
	}
}

func TestUIKitIsNotAShippedUI(t *testing.T) {
	t.Parallel()
	for _, root := range uiSearchRoots {
		if strings.HasPrefix(uiKitDir, root) {
			t.Fatalf("uiSearchRoots entry %s would treat the kit library as a shipped UI with a committed dist", root)
		}
	}
}

// kitRepo lays out a repository with the canonical tokens, the kit, and one UI.
func kitRepo(t *testing.T, appDeps string) (root, app string) {
	t.Helper()
	root = t.TempDir()
	writeUIFile(t, filepath.Join(root, filepath.FromSlash(canonicalUITokensPath)), ":root{}")
	kit := filepath.Join(root, filepath.FromSlash(uiKitDir))
	writeUIFile(t, filepath.Join(kit, "package.json"), `{"name":"@declarative-agents/ui-kit","version":"0.1.0"}`)
	writeUIFile(t, filepath.Join(kit, "src", "index.ts"), "export {};")
	writeUIFile(t, filepath.Join(kit, "node_modules", "dep", "index.js"), "")
	writeUIFile(t, filepath.Join(kit, "dist", "ui-kit.js"), "")
	writeUIFile(t, filepath.Join(kit, uiKitOutDir, "old.tgz"), "")
	app = filepath.Join(root, "applications", "demo", "agents", "demo", "ui")
	writeUIFile(t, filepath.Join(app, "package.json"),
		`{"scripts":{"build":"vite build"},"dependencies":{`+appDeps+`}}`)
	writeUIFile(t, filepath.Join(app, "package-lock.json"), "{}")
	writeUIFile(t, filepath.Join(app, "dist", "index.html"), "<html>")
	return root, app
}

func TestStageUIBuildStagesUIKitSource(t *testing.T) {
	t.Parallel()
	_, app := kitRepo(t, `"@declarative-agents/ui-kit":"file:../../../../ui-kit"`)
	tmp := t.TempDir()
	if _, err := stageUIBuild(app, tmp); err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(tmp, "repo", filepath.FromSlash(uiKitDir))
	files, err := treeFiles(staged)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"package.json":                     true,
		filepath.Join("src", "index.ts"):   true,
		filepath.Join("src", "tokens.css"): true,
	}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("staged kit files = %v, want %v (no node_modules, dist, or out)", files, want)
	}
}

func TestRebuildAndDiffUIBuildsKitBeforeDependentInstall(t *testing.T) {
	t.Parallel()
	_, app := kitRepo(t, `"@declarative-agents/ui-kit":"file:../../../../ui-kit"`)
	var calls []string
	run := func(dir, _ string, args ...string) error {
		where := "app"
		if filepath.Base(dir) == "ui-kit" {
			where = "kit"
		}
		calls = append(calls, where+": "+strings.Join(args, " "))
		if where == "app" && strings.Join(args, " ") == "run build" {
			writeUIFile(t, filepath.Join(dir, "dist", "index.html"), "<html>")
		}
		return nil
	}
	if err := rebuildAndDiffUIWithRunner(app, run); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"kit: ci --no-audit --prefer-offline",
		"kit: run build",
		"app: ci --no-audit --prefer-offline",
		"app: audit --audit-level=high",
		"app: audit --omit=dev --audit-level=high",
		"app: run build",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("runner calls = %v, want %v", calls, want)
	}
}

func TestRebuildAndDiffUISkipsKitForIndependentUI(t *testing.T) {
	t.Parallel()
	_, app := kitRepo(t, `"react":"^19.1.0"`)
	var kitCalls int
	run := func(dir, _ string, args ...string) error {
		if filepath.Base(dir) == "ui-kit" {
			kitCalls++
		}
		if strings.Join(args, " ") == "run build" {
			writeUIFile(t, filepath.Join(dir, "dist", "index.html"), "<html>")
		}
		return nil
	}
	if err := rebuildAndDiffUIWithRunner(app, run); err != nil {
		t.Fatal(err)
	}
	if kitCalls != 0 {
		t.Fatalf("kit built %d times for a UI that does not depend on it", kitCalls)
	}
}
