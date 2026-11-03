// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
)

// checkoutsGit runs git for the checkouts tests and fails the test on error.
func checkoutsGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// checkoutsProgram is a program for the checkouts word: a bare origin for
// the system repository g/program and each registered repository, served
// at git@example.com:<project>.git through url.insteadOf, and the system
// checkout cloned at its group path <root>/g/program, whose subsystem SRDs
// register the repositories. It returns the mirror root, the bare origins'
// folder, and the system checkout.
func checkoutsProgram(t *testing.T, registered ...string) (root, origins, system string) {
	t.Helper()
	base, _ := filepath.EvalSymlinks(t.TempDir())
	origins, root = filepath.Join(base, "origins"), filepath.Join(base, "src")
	cfg := filepath.Join(base, "gitconfig")
	os.WriteFile(cfg, []byte("[url \""+origins+"/\"]\n\tinsteadOf = git@example.com:\n"), 0o644)
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	bare := func(project string) {
		p := filepath.Join(origins, filepath.FromSlash(project)+".git")
		os.MkdirAll(filepath.Dir(p), 0o755)
		checkoutsGit(t, base, "init", "-q", "--bare", "-b", "main", p)
		seed := t.TempDir()
		checkoutsGit(t, base, "clone", "-q", p, seed)
		os.WriteFile(filepath.Join(seed, "README.md"), []byte("# "+project+"\n"), 0o644)
		checkoutsGit(t, seed, "add", "-A")
		checkoutsGit(t, seed, "commit", "-q", "-m", "first")
		checkoutsGit(t, seed, "push", "-q", "origin", "main")
	}
	bare("g/program")
	for _, p := range registered {
		bare(p)
	}
	system = filepath.Join(root, "g", "program")
	os.MkdirAll(filepath.Dir(system), 0o755)
	checkoutsGit(t, base, "clone", "-q", "git@example.com:g/program.git", system)
	srds := filepath.Join(system, "docs", "specs", "software-requirements")
	os.MkdirAll(srds, 0o755)
	for i, p := range registered {
		url := "git@example.com:" + filepath.ToSlash(p) + ".git"
		srd := fmt.Sprintf(`---
id: srd%03d-team%d
---
<!-- field: context -->
## Context

- owner: o%d
- scrum_repository: %s

<!-- field: implementation_targets -->
## Components

| name | repository | path |
| --- | --- | --- |
| c%d | %s | / |
`, i+2, i, i, url, i, url)
		os.WriteFile(filepath.Join(srds, fmt.Sprintf("srd%03d-team%d.md", i+2, i)), []byte(srd), 0o644)
	}
	checkoutsGit(t, system, "add", "-A")
	checkoutsGit(t, system, "commit", "-q", "-m", "registry")
	return root, origins, system
}

// byProject is the entry for one project in a checkouts result.
func byProject(t *testing.T, r CheckoutsResult, p string) CheckoutEntry {
	t.Helper()
	for _, e := range r.Repositories {
		if e.Project == p {
			return e
		}
	}
	t.Fatalf("%s is not in the result: %+v", p, r.Repositories)
	return CheckoutEntry{}
}

// Checkouts clones every registered repository at its group path under the
// mirrored root, and a second run updates them (GH-2812).
func TestCheckoutsClonesTheRegistryAtItsGroupPaths(t *testing.T) {
	root, _, system := checkoutsProgram(t, "g/pods/p1/tpm", "g/analytics/model-evaluation")
	r, err := Checkouts(system, false)
	if err != nil {
		t.Fatalf("Checkouts: %v", err)
	}
	if r.Root != root {
		t.Fatalf("root = %q, want %q", r.Root, root)
	}
	for _, p := range []string{"g/pods/p1/tpm", "g/analytics/model-evaluation"} {
		dir := filepath.Join(root, filepath.FromSlash(p))
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			t.Errorf("%s was not cloned at %s: %v", p, dir, err)
		}
		if e := byProject(t, r, p); e.Action != "cloned" {
			t.Errorf("%s = %s (%s), want cloned", p, e.Action, e.Detail)
		}
	}
	again, err := Checkouts(system, false)
	if err != nil {
		t.Fatalf("second run: %v", err)
	}
	for _, e := range again.Repositories {
		if e.Action != "updated" {
			t.Errorf("second run: %s = %s (%s), want updated", e.Project, e.Action, e.Detail)
		}
	}
	if h := again.Human(); !strings.Contains(h, "0 cloned, 2 updated") {
		t.Errorf("Human = %q", h)
	}
}

// A checkout on another branch, or holding changes, is kept with the
// reason, and the rest still run (GH-2812).
func TestCheckoutsKeepsWhatAPersonIsWorkingIn(t *testing.T) {
	_, _, system := checkoutsProgram(t, "g/pods/p1/tpm", "g/pods/p2/tpm")
	r, err := Checkouts(system, false)
	if err != nil {
		t.Fatalf("first run: %v", err)
	}
	dirty := byProject(t, r, "g/pods/p1/tpm").Path
	os.WriteFile(filepath.Join(dirty, "notes.txt"), []byte("wip\n"), 0o644)
	branched := byProject(t, r, "g/pods/p2/tpm").Path
	checkoutsGit(t, branched, "switch", "-q", "-c", "7-a-fix")

	r, err = Checkouts(system, false)
	if err != nil {
		t.Fatalf("Checkouts: %v", err)
	}
	if e := byProject(t, r, "g/pods/p1/tpm"); e.Action != "kept" || !strings.Contains(e.Detail, "holds changes") {
		t.Errorf("dirty checkout = %s (%s), want kept", e.Action, e.Detail)
	}
	if e := byProject(t, r, "g/pods/p2/tpm"); e.Action != "kept" || !strings.Contains(e.Detail, "on branch 7-a-fix") {
		t.Errorf("branched checkout = %s (%s), want kept", e.Action, e.Detail)
	}
}

// A dry run reports the plan and clones nothing (GH-2812).
func TestCheckoutsDryRunTouchesNothing(t *testing.T) {
	root, _, system := checkoutsProgram(t, "g/pods/p1/tpm")
	r, err := Checkouts(system, true)
	if err != nil {
		t.Fatalf("Checkouts: %v", err)
	}
	e := byProject(t, r, "g/pods/p1/tpm")
	if !r.DryRun || e.Action != "cloned" || !strings.Contains(e.Detail, "would clone") {
		t.Fatalf("dry run = %+v", r)
	}
	if _, err := os.Stat(filepath.Join(root, "g", "pods")); !os.IsNotExist(err) {
		t.Errorf("a dry run cloned something: %v", err)
	}
}

// A repository that cannot be reached fails alone; the run goes on
// (GH-2812).
func TestCheckoutsFailsOneRepositoryAlone(t *testing.T) {
	root, origins, system := checkoutsProgram(t, "g/pods/p1/tpm", "g/pods/p2/tpm")
	os.RemoveAll(filepath.Join(origins, "g", "pods", "p2"))
	r, err := Checkouts(system, false)
	if err != nil {
		t.Fatalf("Checkouts: %v", err)
	}
	if e := byProject(t, r, "g/pods/p2/tpm"); e.Action != "failed" || e.Detail == "" {
		t.Errorf("unreachable repository = %s (%s), want failed with a reason", e.Action, e.Detail)
	}
	if e := byProject(t, r, "g/pods/p1/tpm"); e.Action != "cloned" {
		t.Errorf("reachable repository = %s (%s), want cloned", e.Action, e.Detail)
	}
	if _, err := os.Stat(filepath.Join(root, "g", "pods", "p1", "tpm", ".git")); err != nil {
		t.Errorf("the reachable repository was not cloned: %v", err)
	}
}

// A system checked out side by side, not below its group, clones beside
// itself, trimming the group segments its own path does not carry
// (GH-2812).
func TestCheckoutsClonesBesideASideBySideSystem(t *testing.T) {
	_, _, system := checkoutsProgram(t, "g/pods/p1/tpm")
	base := filepath.Dir(filepath.Dir(filepath.Dir(system)))
	side := filepath.Join(base, "w", "program")
	os.MkdirAll(filepath.Dir(side), 0o755)
	checkoutsGit(t, base, "clone", "-q", "git@example.com:g/program.git", side)
	if out, err := exec.Command("cp", "-R", filepath.Join(system, "docs"), filepath.Join(side, "docs")).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v: %s", err, out)
	}
	r, err := Checkouts(side, false)
	if err != nil {
		t.Fatalf("Checkouts: %v", err)
	}
	want := filepath.Join(base, "w", "pods", "p1", "tpm")
	if e := byProject(t, r, "g/pods/p1/tpm"); e.Action != "cloned" || e.Path != want {
		t.Fatalf("side-by-side clone = %+v, want cloned at %s", e, want)
	}
}

// TestFindSystem: a clone of any of a program's repositories finds its system
// repository: beside it, named, or through the forge, cloning it when it is absent.
func TestFindSystem(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	run := func(d string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", d}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	bare := func(name string, files map[string]string) string {
		t.Helper()
		work := filepath.Join(root, "seed-"+name)
		for f, body := range files {
			os.MkdirAll(filepath.Dir(filepath.Join(work, f)), 0o755)
			if err := os.WriteFile(filepath.Join(work, f), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		run(work, "init", "-q", "-b", "main")
		run(work, "config", "user.email", "t@example.com")
		run(work, "config", "user.name", "t")
		run(work, "add", "-A")
		run(work, "commit", "-q", "-m", "start")
		b := filepath.Join(root, name+".git")
		if out, err := exec.Command("git", "clone", "-q", "--bare", work, b).CombinedOutput(); err != nil {
			t.Fatalf("%s", out)
		}
		return b
	}
	sys, team, plain := bare("program", map[string]string{checkout.TrackersFile: "x"}), bare("team", map[string]string{"README.md": "x"}), bare("plain", map[string]string{"README.md": "x"})
	cfg := filepath.Join(root, "gitconfig")
	conf := ""
	for n, b := range map[string]string{"program": sys, "team": team, "plain": plain} {
		conf += "[url \"" + b + "\"]\n\tinsteadOf = https://gitlab.example.com/g/" + n + ".git\n"
	}
	if err := os.WriteFile(cfg, []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	parent := filepath.Join(root, "work")
	os.MkdirAll(parent, 0o755)
	clone := filepath.Join(parent, "team")
	if out, err := exec.Command("git", "clone", "-q", "https://gitlab.example.com/g/team.git", clone).CombinedOutput(); err != nil {
		t.Fatalf("%s", out)
	}
	found := func(names ...string) func(string) ([]string, error) {
		return func(string) ([]string, error) { return names, nil }
	}

	if _, _, err := FindSystem(clone, "", found(), false); err == nil || !strings.Contains(err.Error(), "--system") {
		t.Fatalf("none found = %v", err)
	}
	if _, _, err := FindSystem(clone, "", found("g/a", "g/b"), false); err == nil || !strings.Contains(err.Error(), "g/a, g/b") {
		t.Fatalf("two found = %v", err)
	}
	if d, e, err := FindSystem(clone, "", found("g/program"), true); err != nil || d != "" || e == nil || !strings.Contains(e.Detail, "would clone") {
		t.Fatalf("dry run = %q, %+v, %v", d, e, err)
	}
	if _, err := os.Stat(filepath.Join(parent, "program")); err == nil {
		t.Fatal("a dry run cloned")
	}
	if _, _, err := FindSystem(clone, "g/plain", found(), false); err == nil || !strings.Contains(err.Error(), "no system repository") {
		t.Fatalf("a named repository that is no system = %v", err)
	}
	os.RemoveAll(filepath.Join(parent, "plain"))
	d, e, err := FindSystem(clone, "", found("g/program"), false)
	if err != nil || d != filepath.Join(parent, "program") || e == nil || e.Action != "cloned" {
		t.Fatalf("clone = %q, %+v, %v", d, e, err)
	}
	if d, e, err = FindSystem(clone, "", found(), false); err != nil || e != nil || filepath.Base(d) != "program" {
		t.Fatalf("beside it, a second run = %q, %+v, %v", d, e, err)
	}
	if d2, _, err := FindSystem(d, "", found(), false); err != nil || d2 != d {
		t.Fatalf("from the system repository itself = %q, %v", d2, err)
	}
}
