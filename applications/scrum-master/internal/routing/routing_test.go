// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package routing

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

const example = `---
id: trackers
title: Tracking systems
---

<!-- field: version -->
## Version

1

<!-- field: default -->
## Default

<!-- field: system -->
### System

bd

<!-- field: tracker_repository -->
### Tracker repository

https://github.com/acme/program-tracker.git

<!-- field: overrides | form: "tree" -->
## Overrides

- acme/delivery/fleet: gh
- acme/scoring: glab
`

// TestParseReadsTheDocument reads the default and overrides, and the
// document scrum-master writes reads back the same (#158).
func TestParseReadsTheDocument(t *testing.T) {
	c, err := Parse([]byte(example))
	if err != nil {
		t.Fatal(err)
	}
	if c.Default.Name != Beads || c.Default.TrackerRepository != "https://github.com/acme/program-tracker.git" || len(c.Overrides) != 2 ||
		c.Overrides["acme/delivery/fleet"] != GitHub || c.Overrides["acme/scoring"] != GitLab {
		t.Fatalf("config = %+v", c)
	}
	again, err := Parse(Document(c))
	if err != nil || again.Default.Name != c.Default.Name || again.Default.TrackerRepository != c.Default.TrackerRepository || len(again.Overrides) != 2 || again.Overrides["acme/scoring"] != GitLab {
		t.Fatalf("round trip = %+v, %v", again, err)
	}
}

// TestParseReadsTheTrackerBranch reads a program of one repository's tracker
// branch, and the document scrum-master writes keeps it.
func TestParseReadsTheTrackerBranch(t *testing.T) {
	doc := strings.Replace(example, "<!-- field: overrides", "<!-- field: tracker_branch -->\n### Tracker branch\n\nscrum-master/tracker\n\n<!-- field: overrides", 1)
	c, err := Parse([]byte(doc))
	if err != nil || c.Default.TrackerBranch != "scrum-master/tracker" {
		t.Fatalf("config = %+v, %v", c.Default, err)
	}
	if again, err := Parse(Document(c)); err != nil || again.Default.TrackerBranch != c.Default.TrackerBranch {
		t.Fatalf("round trip = %+v, %v", again.Default, err)
	}
	if c, err := Parse([]byte(example)); err != nil || c.Default.TrackerBranch != "" || strings.Contains(string(Document(c)), "tracker_branch") {
		t.Fatalf("a document naming no branch: %+v, %v", c.Default, err)
	}
}

// TestParseRefusesAmbiguity refuses a project mapped to two systems, an
// unknown system, and a bd override, which has no tracker repository of
// its own.
func TestParseRefusesAmbiguity(t *testing.T) {
	for line, want := range map[string]string{
		"- acme/scoring: glab\n- acme/scoring: gh\n": "acme/scoring is mapped to both glab and gh",
		"- acme/scoring: jira\n":                     `"jira" is not bd, gh, or glab`,
		"- acme/scoring: bd\n":                       "bd serves through the default",
	} {
		doc := example[:strings.Index(example, "- acme/delivery")] + line
		if _, err := Parse([]byte(doc)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("overrides %q: %v, want %q", line, err, want)
		}
	}
}

func TestOverrideReadsTPMTracker(t *testing.T) {
	for env, want := range map[string]System{
		"":                      {},
		"gh":                    {Name: GitHub},
		"glab":                  {Name: GitLab},
		"/srv/tracker.git":      {Name: Beads, TrackerRepository: "/srv/tracker.git"},
		"https://x/tracker.git": {Name: Beads, TrackerRepository: "https://x/tracker.git"},
	} {
		got, ok := Override(env)
		if got.Name != want.Name || got.TrackerRepository != want.TrackerRepository || ok != (env != "") {
			t.Errorf("Override(%q) = %+v, %v", env, got, ok)
		}
	}
}

// TestFindLooksHereThenAtSiblings finds the checkout's own document, else
// the one sibling's, and refuses two siblings.
func TestFindLooksHereThenAtSiblings(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	repo := func(name string, withConfig bool) string {
		dir := filepath.Join(root, name)
		if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		if withConfig {
			if err := os.MkdirAll(filepath.Join(dir, "control"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, File), []byte(example), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	team := repo("fleet", false)
	if p, err := Find(team); err != nil || p != "" {
		t.Fatalf("no document anywhere: %q, %v", p, err)
	}
	system := repo("system", true)
	if p, err := Find(team); err != nil || p != filepath.Join(system, File) {
		t.Fatalf("a team checkout beside the system: %q, %v", p, err)
	}
	if p, err := Find(system); err != nil || p != filepath.Join(system, File) {
		t.Fatalf("the system checkout itself: %q, %v", p, err)
	}
	// A worktree of the system, as pop opens beside its checkout, is the
	// same repository: the team still finds the main checkout's file (#188).
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git(system, "add", "-A")
	git(system, "commit", "-q", "-m", "control files")
	git(system, "worktree", "add", "-q", filepath.Join(root, "system-3-plan-release"), "-b", "3-plan-release")
	if p, err := Find(team); err != nil || p != filepath.Join(system, File) {
		t.Fatalf("a team checkout beside the system and its worktree: %q, %v", p, err)
	}
	repo("other-system", true)
	if _, err := Find(team); err == nil || !strings.Contains(err.Error(), "several sibling checkouts") {
		t.Fatalf("two systems beside the team: %v", err)
	}
	if p, err := Find(system); err != nil || p != filepath.Join(system, File) {
		t.Fatalf("its own document wins over siblings: %q, %v", p, err)
	}
}

// TestOpenRoutesOverridesAndHonoursTheEnvironment opens the default and
// each override behind a router, lets SCRUM_MASTER_TRACKER replace the default but
// not an override, and with no document opens what SCRUM_MASTER_TRACKER names.
func TestOpenRoutesOverridesAndHonoursTheEnvironment(t *testing.T) {
	opened := map[string]*fake.Fake{}
	for _, name := range []string{Beads, GitHub, GitLab} {
		prev := openers[name]
		f := fake.New()
		opened[name] = f
		openers[name] = func(System) (tracker.Tracker, error) { return f, nil }
		t.Cleanup(func() { openers[name] = prev })
	}
	root := t.TempDir()
	system := filepath.Join(root, "system")
	if err := os.MkdirAll(filepath.Join(system, "control"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(system, File), []byte(example), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCRUM_MASTER_TRACKER", "")
	got, err := Open(system)
	if err != nil {
		t.Fatal(err)
	}
	r, ok := got.(*tracker.Router)
	if !ok || r.Default != opened[Beads] || r.For("acme/delivery/fleet") != opened[GitHub] || r.For("acme/scoring/api") != opened[GitLab] {
		t.Fatalf("router = %+v", got)
	}
	t.Setenv("SCRUM_MASTER_TRACKER", "glab")
	got, _ = Open(system)
	if r := got.(*tracker.Router); r.Default != opened[GitLab] || r.For("acme/delivery/fleet") != opened[GitHub] {
		t.Fatal("SCRUM_MASTER_TRACKER did not replace only the default")
	}

	bare := t.TempDir()
	t.Setenv("SCRUM_MASTER_TRACKER", "")
	if _, err := Open(bare); err == nil || !strings.Contains(err.Error(), "no tracker") {
		t.Fatalf("no document, no SCRUM_MASTER_TRACKER: %v", err)
	}
	t.Setenv("SCRUM_MASTER_TRACKER", "/srv/tracker.git")
	if got, err := Open(bare); err != nil || got != opened[Beads] {
		t.Fatalf("no document, SCRUM_MASTER_TRACKER set: %v, %v", got, err)
	}
}

// TestOpenWithoutAnOpenerSaysSo names a system this scrum-master cannot open yet.
func TestOpenWithoutAnOpenerSaysSo(t *testing.T) {
	prev := openers[GitHub]
	delete(openers, GitHub)
	t.Cleanup(func() {
		if prev != nil {
			openers[GitHub] = prev
		}
	})
	t.Setenv("SCRUM_MASTER_TRACKER", "gh")
	if _, err := Open(t.TempDir()); err == nil || !strings.Contains(err.Error(), "no gh tracking system") {
		t.Fatalf("open gh = %v", err)
	}
}

// The trackers document records the program group, which the document
// scrum-master writes keeps and RecordedGroup reads from a checkout
// (GH-103).
func TestTheProgramGroupIsRecorded(t *testing.T) {
	c := Config{Group: "aeop", Default: System{Name: Beads, TrackerRepository: "https://example.com/tracker.git"}}
	again, err := Parse(Document(c))
	if err != nil || again.Group != "aeop" {
		t.Fatalf("round trip %+v, %v", again, err)
	}
	if c, err := Parse([]byte(example)); err != nil || c.Group != "" {
		t.Fatalf("a document recording no group: %+v, %v", c, err)
	}
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "control"), 0o755)
	os.WriteFile(filepath.Join(dir, File), Document(c), 0o644)
	if g := RecordedGroup(dir); g != "aeop" {
		t.Fatalf("recorded group %q", g)
	}
	// A checkout with no document of its own and none beside it records
	// none.
	alone := filepath.Join(t.TempDir(), "alone", "repo")
	os.MkdirAll(alone, 0o755)
	if g := RecordedGroup(alone); g != "" {
		t.Fatalf("a checkout with no document records %q", g)
	}
}

// hostFake is a tracker whose host serves the system's trackers document.
type hostFake struct {
	*fake.Fake
	served map[string][]byte
}

func (h hostFake) HostTree(string, string) ([]string, error) { return nil, nil }
func (h hostFake) HostFile(project, path string) ([]byte, string, error) {
	if b, ok := h.served[project+":"+path]; ok {
		return b, "c0ffee", nil
	}
	return nil, "", tracker.ErrNotFound
}

// With no checkout holding the trackers document and the run naming its
// system, the document is read through the forge the checkout's origin is
// on, and the program's tracking system opens; with neither, the refusal
// names the folders searched and both remedies (srd042; GH-2743).
func TestOpenForReadsTheTrackersThroughTheForge(t *testing.T) {
	t.Setenv("SCRUM_MASTER_TRACKER", "")
	t.Setenv("SCRUM_MASTER_SYSTEM", "")
	dir := filepath.Join(t.TempDir(), "platform", "tpm")
	os.MkdirAll(dir, 0o755)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", "https://gitlab.example.com/acme/platform/tpm.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	host := hostFake{Fake: fake.New(), served: map[string][]byte{"acme/tpm:" + File: Document(Config{Group: "acme", Default: System{Name: GitLab}})}}
	saved := openers[GitLab]
	t.Cleanup(func() { openers[GitLab] = saved })
	openers[GitLab] = func(System) (tracker.Tracker, error) { return host, nil }

	got, err := OpenFor(dir, "acme/tpm")
	if err != nil {
		t.Fatalf("OpenFor with --system: %v", err)
	}
	if _, ok := got.(hostFake); !ok {
		t.Errorf("OpenFor opened %T, want the program's glab", got)
	}
	_, err = OpenFor(dir, "")
	if err == nil || !strings.Contains(err.Error(), filepath.Dir(dir)) || !strings.Contains(err.Error(), "SCRUM_MASTER_SYSTEM") || !strings.Contains(err.Error(), "SCRUM_MASTER_TRACKER") {
		t.Fatalf("with nothing to read: %v", err)
	}
}
