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
// folder: the system an-gc-group/tpm and the Platform scrum's repository
// an-gc-group/platform/tpm, which shares the system's last segment.
func mirrored(t *testing.T) (home, system, scrum string) {
	t.Helper()
	home = t.TempDir()
	if h, err := filepath.EvalSymlinks(home); err == nil {
		home = h
	}
	t.Setenv("HOME", home)
	system = filepath.Join(home, "GITLABS", "an-gc-group", "tpm")
	scrum = filepath.Join(home, "GITLABS", "an-gc-group", "platform", "tpm")
	repoAt(t, system, "https://gitlab.example.com/an-gc-group/tpm.git", TrackersFile)
	repoAt(t, scrum, "https://gitlab.example.com/an-gc-group/platform/tpm.git")
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
	second := filepath.Join(home, "GITLABS", "an-gc-group", "tpm-next")
	repoAt(t, second, "https://gitlab.example.com/an-gc-group/tpm-next.git", TrackersFile)
	_, err := FindTrackers(scrum)
	if err == nil || !strings.Contains(err.Error(), system) || !strings.Contains(err.Error(), second) {
		t.Fatalf("two systems of the group: %v, want both named", err)
	}
}

// A project's checkout is the one whose origin is the project: from the
// Platform scrum's repository, an-gc-group/tpm is the system's checkout,
// never the scrum's own though both are named tpm, and a project with no
// checkout is an error naming the folders searched, never another
// repository's files (GH-2744).
func TestLocateMatchesTheOrigin(t *testing.T) {
	_, system, scrum := mirrored(t)
	if got, ok := Locate(scrum, "an-gc-group/tpm"); !ok || got != system {
		t.Fatalf("Locate(an-gc-group/tpm) = %q, %v, want %s", got, ok, system)
	}
	if got, ok := Locate(scrum, "an-gc-group/platform/tpm"); !ok || got != scrum {
		t.Fatalf("Locate(its own) = %q, %v", got, ok)
	}
	c := &Code{Dir: scrum}
	if _, err := c.GetFile("an-gc-group/elsewhere", TrackersFile, "main"); !errors.Is(err, ErrNoCheckout) || !strings.Contains(err.Error(), "an-gc-group/elsewhere") {
		t.Fatalf("a project with no checkout: %v", err)
	}
	if c.HasCheckout("an-gc-group/elsewhere") || !c.HasCheckout("an-gc-group/tpm") {
		t.Error("HasCheckout does not follow Locate")
	}
}
