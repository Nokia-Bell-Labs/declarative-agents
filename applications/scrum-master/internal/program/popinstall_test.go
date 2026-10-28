// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
)

// checkoutWithWorktree is a git checkout with one commit and a linked
// worktree beside it, as scrum-master issue pop leaves them.
func checkoutWithWorktree(t *testing.T) (checkout, worktree string, git func(dir string, args ...string) string) {
	t.Helper()
	base, _ := filepath.EvalSymlinks(t.TempDir())
	checkout, worktree = filepath.Join(base, "po-1"), filepath.Join(base, "po-1-3-issue")
	git = func(dir string, args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com"}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	os.MkdirAll(checkout, 0o755)
	git(checkout, "init", "-q")
	git(checkout, "commit", "-q", "--allow-empty", "-m", "first")
	git(checkout, "worktree", "add", "-q", "-b", "3-issue", worktree)
	return checkout, worktree, git
}

func TestPopInstallsSkillsIntoWorktree(t *testing.T) {
	checkout, worktree, git := checkoutWithWorktree(t)
	skills, err := install.EmbeddedSkills()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := install.Install(checkout, []string{"claude"}, skills, install.Version(), false, false); err != nil {
		t.Fatal(err)
	}

	agents, err := InstallSkills(checkout, worktree)
	if err != nil || !slices.Equal(agents, []string{"claude"}) {
		t.Fatalf("agents=%v err=%v", agents, err)
	}
	for _, s := range skills {
		b, err := os.ReadFile(filepath.Join(worktree, ".claude/skills", s.Name, "SKILL.md"))
		if err != nil || !strings.Contains(string(b), "<!-- skill install version=") {
			t.Fatalf("%s not installed in the worktree: %v", s.Name, err)
		}
	}
	for _, other := range []string{".cursor", ".github"} {
		if _, err := os.Stat(filepath.Join(worktree, other)); err == nil {
			t.Fatalf("installed %s, which the checkout does not have", other)
		}
	}
	if status := git(worktree, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Fatalf("worktree status %q", status)
	}
	// A second pop into the same worktree writes nothing, so it reports no
	// agent installed.
	if again, err := InstallSkills(checkout, worktree); err != nil || again == nil || len(again) != 0 {
		t.Fatalf("a second pop: agents=%#v err=%v, want an empty list, which prints [] (GH-2792)", again, err)
	}
}

func TestPopWithoutInstalledSkillsInstallsNothing(t *testing.T) {
	checkout, worktree, git := checkoutWithWorktree(t)
	agents, err := InstallSkills(checkout, worktree)
	if err != nil || agents == nil || len(agents) != 0 {
		t.Fatalf("agents=%#v err=%v, want an empty list (GH-2792)", agents, err)
	}
	if status := git(worktree, "status", "--porcelain", "--untracked-files=all"); status != "" {
		t.Fatalf("worktree status %q", status)
	}
}
