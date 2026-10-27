// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// home is a home folder for the test, so checkouts mirror a forge's groups
// under it.
func home(t *testing.T) string {
	t.Helper()
	h := t.TempDir()
	if r, err := filepath.EvalSymlinks(h); err == nil {
		h = r
	}
	t.Setenv("HOME", h)
	return h
}

// worktree opens a worktree of the checkout at dir beside it, named as pop
// names one, <repository>-<iid>-<slug>.
func worktree(t *testing.T, dir, branch string) string {
	t.Helper()
	wt := filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-"+branch)
	beadstest.Git(t, dir, "worktree", "add", "-q", wt, "-b", branch)
	return wt
}

// In a worktree pop opened in a system checkout, the system is named by its
// origin, never the worktree's folder, and its policy reads from the
// worktree (GH-2752).
func TestASystemWorktreeIsNamedByItsOrigin(t *testing.T) {
	root := filepath.Join(home(t), "GITLABS", "an-gc-group")
	system := filepath.Join(root, "tpm")
	checkoutWith(t, system, map[string]string{checkout.TrackersFile: "bd\n", PolicyFile: approvalPolicy})
	beadstest.Git(t, system, "remote", "add", "origin", "https://gitlab.example.com/an-gc-group/tpm.git")
	wt := worktree(t, system, "46-take-scrum-master-v0-20261026-6")

	control := ControlFor("", wt, "")
	if control != "an-gc-group/tpm" {
		t.Fatalf("the worktree's control = %q, want an-gc-group/tpm", control)
	}
	p, err := LoadPolicy(codeFiles{fake.New(), &checkout.Code{Dir: wt}}, control, "main")
	if err != nil || !p.RequireApproval {
		t.Fatalf("the worktree's policy = %+v, %v, want the system's Require approval", p, err)
	}
}

// In a scrum repository's worktree, in a folder layout that mirrors the
// forge's groups, the system above is found and named by its origin, and
// its policy reads from its checkout (GH-2752, GH-2743).
func TestAScrumWorktreeFindsTheSystemAbove(t *testing.T) {
	root := filepath.Join(home(t), "GITLABS", "an-gc-group")
	system, scrum := filepath.Join(root, "tpm"), filepath.Join(root, "platform", "tpm")
	checkoutWith(t, system, map[string]string{checkout.TrackersFile: "bd\n", PolicyFile: approvalPolicy})
	beadstest.Git(t, system, "remote", "add", "origin", "https://gitlab.example.com/an-gc-group/tpm.git")
	checkoutWith(t, scrum, nil)
	beadstest.Git(t, scrum, "remote", "add", "origin", "https://gitlab.example.com/an-gc-group/platform/tpm.git")
	wt := worktree(t, scrum, "12-plan-the-sprint")

	control := ControlFor("", wt, "")
	if control != "an-gc-group/tpm" {
		t.Fatalf("the scrum worktree's control = %q, want an-gc-group/tpm", control)
	}
	p, err := LoadPolicy(codeFiles{fake.New(), &checkout.Code{Dir: wt}}, control, "main")
	if err != nil || !p.RequireApproval {
		t.Fatalf("the scrum worktree's policy = %+v, %v", p, err)
	}
}

// A system checkout whose folder is not its project's name is still named
// by its origin, and a refusal to read a project with no checkout names the
// project, never a folder (GH-2752).
func TestARefusalNamesTheProjectByOrigin(t *testing.T) {
	root := filepath.Join(home(t), "GITLABS", "an-gc-group")
	system := filepath.Join(root, "tpm-renamed")
	checkoutWith(t, system, map[string]string{checkout.TrackersFile: "bd\n"})
	beadstest.Git(t, system, "remote", "add", "origin", "https://gitlab.example.com/an-gc-group/tpm.git")
	if got := SystemControl(system); got != "an-gc-group/tpm" {
		t.Fatalf("SystemControl = %q, want an-gc-group/tpm", got)
	}
	_, err := (&checkout.Code{Dir: system}).GetFile("an-gc-group/elsewhere", PolicyFile, "main")
	if !errors.Is(err, checkout.ErrNoCheckout) || !strings.HasPrefix(err.Error(), "an-gc-group/elsewhere:") || strings.Contains(err.Error(), "tpm-renamed:") {
		t.Fatalf("refusal = %v, want it to name an-gc-group/elsewhere", err)
	}
}

// A word given the system as a worktree's path, as plan write's --system
// is documented, reads the policy there (GH-2769).
func TestThePolicyReadsFromASystemPath(t *testing.T) {
	root := filepath.Join(home(t), "GITLABS", "an-gc-group")
	system := filepath.Join(root, "tpm")
	checkoutWith(t, system, map[string]string{checkout.TrackersFile: "bd\n", PolicyFile: approvalPolicy})
	beadstest.Git(t, system, "remote", "add", "origin", "https://gitlab.example.com/an-gc-group/tpm.git")
	wt := worktree(t, system, "4-plan-release-1")
	p, err := LoadPolicy(codeFiles{fake.New(), &checkout.Code{Dir: wt}}, wt, "main")
	if err != nil || !p.RequireApproval {
		t.Fatalf("the policy at the system's path = %+v, %v", p, err)
	}
}
