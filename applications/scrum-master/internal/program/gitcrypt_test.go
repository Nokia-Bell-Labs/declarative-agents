// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

// TestWorktreeOfAGitCryptRepository checks out a worktree whose smudge
// filter, like git-crypt's, needs the key directory in the worktree's own git
// dir: addWorktree links the shared one there before checking out.
func TestWorktreeOfAGitCryptRepository(t *testing.T) {
	root := filepath.Join(t.TempDir(), "system")
	gitIn(t, filepath.Dir(root), "init", "-q", "-b", "main", root)
	gitIn(t, root, "config", "user.name", "operator")
	gitIn(t, root, "config", "user.email", "operator@example.com")
	gitIn(t, root, "config", "filter.keyed.clean", "cat")
	gitIn(t, root, "config", "filter.keyed.smudge", `sh -c 'test -f "$(git rev-parse --git-dir)/git-crypt/keys/default" && cat'`)
	gitIn(t, root, "config", "filter.keyed.required", "true")
	if err := os.MkdirAll(filepath.Join(root, ".git", "git-crypt", "keys"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "git-crypt", "keys", "default"), []byte("key"), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{".gitattributes": "AGENTS.md filter=keyed\n", "AGENTS.md": "# Agents\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "first")
	gitIn(t, root, "branch", "1-work")

	wt := filepath.Join(filepath.Dir(root), "system-1-work")
	if err := addWorktree(ExecGit{}, root, wt, wt, "1-work"); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(wt, "AGENTS.md")); err != nil || string(b) != "# Agents\n" {
		t.Fatalf("AGENTS.md in the worktree = %q, %v", b, err)
	}
}

// TestWorktreeWithoutGitCrypt is a plain worktree add.
func TestWorktreeWithoutGitCrypt(t *testing.T) {
	root := filepath.Join(t.TempDir(), "po-1")
	gitIn(t, filepath.Dir(root), "init", "-q", "-b", "main", root)
	gitIn(t, root, "config", "user.name", "operator")
	gitIn(t, root, "config", "user.email", "operator@example.com")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# po-1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, root, "add", "-A")
	gitIn(t, root, "commit", "-q", "-m", "first")
	gitIn(t, root, "branch", "1-work")
	wt := filepath.Join(filepath.Dir(root), "po-1-1-work")
	if err := addWorktree(ExecGit{}, root, wt, wt, "1-work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wt, "README.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".git", "worktrees", "po-1-1-work", "git-crypt")); !os.IsNotExist(err) {
		t.Fatalf("a key link appeared without git-crypt: %v", err)
	}
}
