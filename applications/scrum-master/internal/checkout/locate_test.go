// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package checkout

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repoAt is a git checkout at dir whose origin is origin, holding files.
func repoAt(t *testing.T, dir, origin string, files ...string) {
	t.Helper()
	for _, f := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, f), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(dir, 0o755)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", origin}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

// mirrored lays checkouts out as the forge's groups are, under a home
// folder: the system acme/tpm and the Platform scrum's repository
// acme/platform/tpm, which shares the system's last segment.
func mirrored(t *testing.T) (home, system, scrum string) {
	t.Helper()
	home = t.TempDir()
	if h, err := filepath.EvalSymlinks(home); err == nil {
		home = h
	}
	t.Setenv("HOME", home)
	system = filepath.Join(home, "GITLABS", "acme", "tpm")
	scrum = filepath.Join(home, "GITLABS", "acme", "platform", "tpm")
	repoAt(t, system, "https://gitlab.example.com/acme/tpm.git", TrackersFile)
	repoAt(t, scrum, "https://gitlab.example.com/acme/platform/tpm.git")
	return home, system, scrum
}

// Where checkouts mirror the forge's groups, a scrum repository finds the
// system's trackers document beside a folder above it, with no environment
// variable (srd042; GH-2743).
func TestFindTrackersSearchesTheGroupLayout(t *testing.T) {
	_, system, scrum := mirrored(t)
	got, err := FindTrackers(scrum)
	if err != nil || got != filepath.Join(system, TrackersFile) {
		t.Fatalf("FindTrackers = %q, %v, want the system's", got, err)
	}
}

// A system checkout of another group above the checkout is never taken,
// and two of this group's at one level are refused, naming both (GH-2743).
func TestFindTrackersRefusesAmbiguityAndOtherGroups(t *testing.T) {
	home, system, scrum := mirrored(t)
	repoAt(t, filepath.Join(home, "GITLABS", "other"), "https://gitlab.example.com/other-group/system.git", TrackersFile)
	if got, err := FindTrackers(scrum); err != nil || got != filepath.Join(system, TrackersFile) {
		t.Fatalf("with another group's system above: %q, %v", got, err)
	}
	second := filepath.Join(home, "GITLABS", "acme", "tpm-next")
	repoAt(t, second, "https://gitlab.example.com/acme/tpm-next.git", TrackersFile)
	_, err := FindTrackers(scrum)
	if err == nil || !strings.Contains(err.Error(), system) || !strings.Contains(err.Error(), second) {
		t.Fatalf("two systems of the group: %v, want both named", err)
	}
}

// A project's checkout is the one whose origin is the project: from the
// Platform scrum's repository, acme/tpm is the system's checkout,
// never the scrum's own though both are named tpm, and a project with no
// checkout is an error naming the folders searched, never another
// repository's files (GH-2744).
func TestLocateMatchesTheOrigin(t *testing.T) {
	_, system, scrum := mirrored(t)
	if got, ok := Locate(scrum, "acme/tpm"); !ok || got != system {
		t.Fatalf("Locate(acme/tpm) = %q, %v, want %s", got, ok, system)
	}
	if got, ok := Locate(scrum, "acme/platform/tpm"); !ok || got != scrum {
		t.Fatalf("Locate(its own) = %q, %v", got, ok)
	}
	c := &Code{Dir: scrum}
	if _, err := c.GetFile("acme/elsewhere", TrackersFile, "main"); !errors.Is(err, ErrNoCheckout) || !strings.Contains(err.Error(), "acme/elsewhere") {
		t.Fatalf("a project with no checkout: %v", err)
	}
	if c.HasCheckout("acme/elsewhere") || !c.HasCheckout("acme/tpm") {
		t.Error("HasCheckout does not follow Locate")
	}
}

// A project named by a checkout's path, as a word's --system may be, is
// that checkout whatever its origin, a worktree's included (GH-2769).
func TestAPathNamesItsCheckout(t *testing.T) {
	_, system, scrum := mirrored(t)
	for _, args := range [][]string{{"-C", system, "add", "-A"}, {"-C", system, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "seed"},
		{"-C", system, "worktree", "add", "-q", system + "-4-plan", "-b", "4-plan"}} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	wt := system + "-4-plan"
	if got, ok := Locate(scrum, wt); !ok || got != wt {
		t.Fatalf("Locate(%s) = %q, %v, want the worktree", wt, got, ok)
	}
	c := &Code{Dir: wt}
	if _, err := c.GetFile(wt, TrackersFile, "4-plan"); err != nil {
		t.Fatalf("reading the worktree by its path: %v", err)
	}
	if _, ok := Locate(scrum, "/no/such/checkout"); ok {
		t.Error("a path with no checkout located one")
	}
}

// homeRoot is a home folder for a test whose checkouts sit below it, so
// Levels searches up to it.
func homeRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	t.Setenv("HOME", root)
	return root
}

// A system repository at the top of its group finds a project in a
// subgroup checked out at its group path, below the group's folder or
// below any level above it, and two projects sharing a last segment each
// find their own; the side-by-side layout still works (GH-2803).
func TestLocateFindsCheckoutsAtTheirGroupPath(t *testing.T) {
	root := homeRoot(t)
	g := filepath.Join(root, "GITLABS", "g")
	system := filepath.Join(g, "program")
	repoAt(t, system, "https://gitlab.example.com/g/program.git", TrackersFile)
	sub := filepath.Join(g, "sub", "tpm")
	other := filepath.Join(g, "other", "tpm")
	repoAt(t, sub, "https://gitlab.example.com/g/sub/tpm.git")
	repoAt(t, other, "https://gitlab.example.com/g/other/tpm.git")
	for project, want := range map[string]string{"g/sub/tpm": sub, "g/other/tpm": other} {
		if got, ok := Locate(system, project); !ok || got != want {
			t.Errorf("Locate(%s) from the system = %q, %v; want %s", project, got, ok, want)
		}
	}
	// Below a level above the group's folder, at the full group path.
	x := filepath.Join(root, "x")
	far := filepath.Join(x, "g", "deep", "tool")
	repoAt(t, far, "https://gitlab.example.com/g/deep/tool.git")
	inX := filepath.Join(x, "g", "program2")
	repoAt(t, inX, "https://gitlab.example.com/g/program2.git")
	if got, ok := Locate(inX, "g/deep/tool"); !ok || got != far {
		t.Errorf("Locate(g/deep/tool) from %s = %q, %v; want %s", inX, got, ok, far)
	}
	// Side by side, named for the last segment, as today.
	beside := filepath.Join(g, "lone")
	repoAt(t, beside, "https://gitlab.example.com/g/team/lone.git")
	if got, ok := Locate(system, "g/team/lone"); !ok || got != beside {
		t.Errorf("Locate(g/team/lone) side by side = %q, %v; want %s", got, ok, beside)
	}
}

// A folder at a matching path whose origin is another project is passed
// over, and the lookup goes on to the next candidate (GH-2803, GH-2744).
func TestLocatePassesOverAPathOfAnotherProject(t *testing.T) {
	root := homeRoot(t)
	g := filepath.Join(root, "GITLABS", "g")
	system := filepath.Join(g, "program")
	repoAt(t, system, "https://gitlab.example.com/g/program.git", TrackersFile)
	repoAt(t, filepath.Join(g, "sub", "tpm"), "https://gitlab.example.com/g/elsewhere/tpm.git")
	right := filepath.Join(g, "tpm")
	repoAt(t, right, "https://gitlab.example.com/g/sub/tpm.git")
	if got, ok := Locate(system, "g/sub/tpm"); !ok || got != right {
		t.Fatalf("Locate(g/sub/tpm) = %q, %v; want %s, passing over the other project's checkout", got, ok, right)
	}
}
