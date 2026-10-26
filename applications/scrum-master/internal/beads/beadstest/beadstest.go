// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package beadstest builds the git fixtures beads tests run against: a
// tracker repository and code repositories, all on local disk, with no
// network.
package beadstest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TrackerRepository creates an empty tracker repository under root and
// returns its path, the remote every clone pulls from and pushes to.
func TrackerRepository(t testing.TB, root string) string {
	t.Helper()
	remote := filepath.Join(root, "tracker.git")
	Git(t, root, "init", "-q", "--bare", "-b", "main", remote)
	seed := filepath.Join(root, "tracker-seed")
	Git(t, root, "clone", "-q", remote, seed)
	Identity(t, seed, "seed")
	if err := os.MkdirAll(filepath.Join(seed, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(seed, ".beads", "issues.jsonl"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	Git(t, seed, "add", "-A")
	Git(t, seed, "commit", "-q", "-m", "seed")
	Git(t, seed, "push", "-q", "origin", "HEAD:main")
	return remote
}

// CodeRepository creates a code repository named name under root, with a
// bare origin holding one commit on main, and returns the checkout's path.
func CodeRepository(t testing.TB, root, name, user string) string {
	t.Helper()
	origin := filepath.Join(root, name+".git")
	Git(t, root, "init", "-q", "--bare", "-b", "main", origin)
	checkout := filepath.Join(root, name)
	Git(t, root, "clone", "-q", origin, checkout)
	Identity(t, checkout, user)
	if err := os.WriteFile(filepath.Join(checkout, "README.md"), []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	Git(t, checkout, "add", "-A")
	Git(t, checkout, "commit", "-q", "-m", "first commit")
	Git(t, checkout, "push", "-q", "-u", "origin", "main")
	Git(t, checkout, "remote", "set-head", "origin", "main")
	return checkout
}

// Clone clones remote into dir as user.
func Clone(t testing.TB, remote, dir, user string) {
	t.Helper()
	Git(t, filepath.Dir(dir), "clone", "-q", remote, dir)
	Identity(t, dir, user)
}

// Identity sets the repository-local git identity of user.
func Identity(t testing.TB, dir, user string) {
	t.Helper()
	Git(t, dir, "config", "user.name", user)
	Git(t, dir, "config", "user.email", user+"@example.com")
}

// Git runs git in dir and returns its trimmed output, failing the test on
// error.
func Git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}
