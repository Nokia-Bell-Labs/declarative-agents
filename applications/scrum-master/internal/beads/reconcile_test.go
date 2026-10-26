// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// TestReconcileCommandConfirmsProvisionalPops runs scrum-master tracker reconcile on
// a clone that popped offline: back online, the pop is confirmed, and the
// output names the tracker commit it read.
func TestReconcileCommandConfirmsProvisionalPops(t *testing.T) {
	root := t.TempDir()
	remote := beadstest.TrackerRepository(t, root)
	clone := filepath.Join(root, "alice-tracker")
	beadstest.Clone(t, remote, clone, "alice")
	tr := NewTracker(clone, "alice", beadstest.CodeRepository(t, root, "po-1", "alice"))
	i, err := tr.CreateIssue("g/po-1", tracker.NewIssue{Title: "work"})
	if err != nil {
		t.Fatal(err)
	}

	beadstest.Git(t, clone, "remote", "set-url", "origin", filepath.Join(root, "unreachable.git"))
	me, _ := tr.CurrentUser()
	if err := tr.AssignIssue("g/po-1", i.IID, me); err != nil {
		t.Fatal(err)
	}
	beadstest.Git(t, clone, "remote", "set-url", "origin", remote)

	var out bytes.Buffer
	err = cli.Default.Run([]string{"tracker", "reconcile", "--json"}, &out, func() (tracker.Tracker, error) { return tr, nil })
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		ReconcileResult
		Read tracker.Read `json:"tracker_read"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("%v: %s", err, out.String())
	}
	if len(got.Reconciled) != 1 || got.Reconciled[0].Outcome != "confirmed" || got.Read.Commit == "" || got.Read.Offline {
		t.Fatalf("reconcile = %+v, want one confirmed pop read online", got)
	}
	// The queued write is reported, and the read names the commit after
	// the push, which the tracker repository now holds.
	head := beadstest.Git(t, root, "--git-dir", remote, "rev-parse", "main")
	if len(got.Pushed) != 1 || got.Commit != head || got.Read.Commit != head {
		t.Fatalf("reconcile pushed %v at %s, read %s; the tracker is at %s", got.Pushed, got.Commit, got.Read.Commit, head)
	}
}
