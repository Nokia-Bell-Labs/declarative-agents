// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// origin is a local tracker repository with one commit.
func origin(t *testing.T, path, message string) {
	t.Helper()
	gitIn(t, "init", "-q", "-b", "main", path)
	gitIn(t, "-C", path, "commit", "-q", "--allow-empty", "-m", message)
}

// cache points the user cache at a fresh directory and returns its
// tracker clones' root.
func cache(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	root, err := cachedClones()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// A cached tracker clone whose origin is a local path that is gone is
// named; one whose origin exists, or is a URL, is not (GH-116). One whose
// origin was made again at its path, holding none of its history, is named
// too (GH-141).
func TestOrphanedClones(t *testing.T) {
	root := cache(t)
	remotes := t.TempDir()
	for _, name := range []string{"gone", "alive", "remade"} {
		origin(t, filepath.Join(remotes, name), "first")
		gitIn(t, "clone", "-q", filepath.Join(remotes, name), filepath.Join(root, name))
	}
	os.RemoveAll(filepath.Join(remotes, "gone"))
	os.RemoveAll(filepath.Join(remotes, "remade"))
	origin(t, filepath.Join(remotes, "remade"), "another")
	url := filepath.Join(root, "url")
	gitIn(t, "init", "-q", url)
	gitIn(t, "-C", url, "remote", "add", "origin", "https://git.example.com/acme/tracker.git")

	got := OrphanedClones()
	if len(got) != 2 || !strings.HasPrefix(got[0], filepath.Join(root, "gone")+" (origin ") || !strings.Contains(got[1], "made again") {
		t.Fatalf("orphaned clones %v", got)
	}
}

// Opening a tracker whose clone's origin is gone reads the clone as it is
// and removes nothing: the clone may be the tracker's last copy, as a
// demo built from a torn-down simulation's tracker is (GH-141).
func TestOpenRepositoryKeepsACloneWhoseOriginIsGone(t *testing.T) {
	cache(t)
	t.Setenv("SCRUM_MASTER_USER", "alice")
	clone := filepath.Join(t.TempDir(), "tracker-clone")
	t.Setenv("SCRUM_MASTER_TRACKER_CLONE", clone)
	remote := filepath.Join(t.TempDir(), "tracker.git")
	origin(t, remote, "the only tracker")
	if _, err := OpenRepository(remote, ""); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(remote)
	tr, err := OpenRepository(remote, "")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command("git", "-C", tr.Store.Dir, "log", "-1", "--format=%s").Output()
	if strings.TrimSpace(string(out)) != "the only tracker" {
		t.Fatalf("the clone whose origin is gone reads %q", out)
	}
}

// Opening a local tracker made again at a cached clone's origin sets the
// old clone aside, renamed, never removed, and clones the new tracker, so
// the old tracker's history is never read as the new one's (GH-141).
func TestOpenRepositorySetsARemadeTrackersCloneAside(t *testing.T) {
	cache(t)
	t.Setenv("SCRUM_MASTER_USER", "alice")
	t.Setenv("SCRUM_MASTER_TRACKER_CLONE", "")
	remote := filepath.Join(t.TempDir(), "tracker.git")
	origin(t, remote, "the first tracker")
	tr, err := OpenRepository(remote, "")
	if err != nil {
		t.Fatal(err)
	}
	first := tr.Store.Dir
	os.RemoveAll(remote)
	origin(t, remote, "the second tracker")
	if tr, err = OpenRepository(remote, ""); err != nil {
		t.Fatal(err)
	}
	out, _ := exec.Command("git", "-C", tr.Store.Dir, "log", "-1", "--format=%s").Output()
	if strings.TrimSpace(string(out)) != "the second tracker" {
		t.Fatalf("the clone reads %q, not the remade tracker", out)
	}
	aside, _ := filepath.Glob(first + ".stale-*")
	if len(aside) != 1 {
		t.Fatalf("the old clone was not set aside: %v", aside)
	}
	if out, _ := exec.Command("git", "-C", aside[0], "log", "-1", "--format=%s").Output(); strings.TrimSpace(string(out)) != "the first tracker" {
		t.Fatalf("the set-aside clone reads %q", out)
	}
	if got := OrphanedClones(); len(got) != 1 || !strings.HasPrefix(got[0], aside[0]+" ") {
		t.Fatalf("orphaned clones %v, want the set-aside one named", got)
	}
}
