// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// codeFiles is a tracker whose files are the code half's, read from the
// checkouts beside Dir as every tracking system reads them.
type codeFiles struct {
	*fake.Fake
	code *checkout.Code
}

func (c codeFiles) GetFile(project, path, ref string) ([]byte, error) {
	return c.code.GetFile(project, path, ref)
}

const approvalPolicy = `---
id: policy
title: Policy
---

<!-- field: admission -->
## Admission

<!-- field: require_approval -->
### Require approval

true
`

// checkoutWith is a git checkout at dir on main holding files, committed.
func checkoutWith(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, dir, "init", "-q", "-b", "main")
	for name, body := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	beadstest.Git(t, dir, "add", "-A")
	beadstest.Git(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "--allow-empty", "-m", "seed")
}

// A team repository's checkout reads the policy of the system repository
// beside it, so the system's Require approval holds its merges; an explicit
// --control wins, and a checkout with no system beside it keeps the word's
// own fallback (GH-46).
func TestControlForIsTheSystemBesideTheCheckout(t *testing.T) {
	root := t.TempDir()
	system, team, lone := filepath.Join(root, "checkouts", "system"), filepath.Join(root, "checkouts", "po-1"), filepath.Join(root, "alone", "po-2")
	checkoutWith(t, system, map[string]string{checkout.TrackersFile: "bd\n", PolicyFile: approvalPolicy})
	beadstest.Git(t, system, "remote", "add", "origin", "https://example.com/group/system.git")
	checkoutWith(t, team, nil)
	checkoutWith(t, lone, nil)

	control := ControlFor("", team, "g/po-1")
	if control != "group/system" {
		t.Fatalf("a team checkout's control = %q, want the system repository group/system", control)
	}
	p, err := LoadPolicy(codeFiles{fake.New(), &checkout.Code{Dir: team}}, control, "main")
	if err != nil || !p.RequireApproval {
		t.Fatalf("the team checkout's policy = %+v, %v, want the system's Require approval", p, err)
	}
	if got := ControlFor("", system, ""); got != "group/system" {
		t.Errorf("the system checkout's own control = %q", got)
	}
	if got := ControlFor("g/other", team, "g/po-1"); got != "g/other" {
		t.Errorf("an explicit --control = %q, want it kept", got)
	}
	if got := ControlFor("", lone, "g/po-2"); got != "g/po-2" {
		t.Errorf("a checkout with no system beside it = %q, want the fallback", got)
	}

	beadstest.Git(t, system, "remote", "remove", "origin")
	if got := ControlFor("", team, ""); got != "system" {
		t.Errorf("a system checkout with no origin = %q, want its directory's name", got)
	}
}
