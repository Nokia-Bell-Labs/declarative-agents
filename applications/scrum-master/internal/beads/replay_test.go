// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestRejectedWriteReplays has another clone create an issue before each
// of bob's first three pushes: bob's create replays on the newer tracker
// each time, so it lands once, numbered after all of alice's, its finish
// applied once, and the tracker repository holds no merge commit (#145).
func TestRejectedWriteReplays(t *testing.T) {
	f := newFixture(t)
	alice, bob := f.clone("alice"), f.clone("bob")
	bob.sleep = func(time.Duration) {}
	rejections := 0
	bob.beforePush = func() {
		if rejections == 3 {
			return
		}
		rejections++
		if _, err := alice.Create(Issue{Repository: "g/po-1", Title: "alice's"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := bob.CreateWith(Issue{Repository: "g/po-1", Title: "bob's"}, func(i *Issue) error {
		i.Comments = append(i.Comments, Comment{ID: newID(), IssueID: i.ID, Author: "bob", Text: "admitted"})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if rejections != 3 || got.IID != 4 || len(got.Comments) != 1 {
		t.Fatalf("bob's issue after %d rejections = IID %d, %d comments; want IID 4 and one comment", rejections, got.IID, len(got.Comments))
	}
	if r := f.remoteIssue(got.ID); r.IID != 4 || len(r.Comments) != 1 {
		t.Fatalf("the tracker holds bob's issue as IID %d with %d comments", r.IID, len(r.Comments))
	}
	out, err := exec.Command("git", "--git-dir", f.remote, "log", "--merges", "--oneline", "main").Output()
	if err != nil {
		t.Fatal(err)
	}
	if merges := strings.TrimSpace(string(out)); merges != "" {
		t.Fatalf("a replayed write left merge commits:\n%s", merges)
	}
}

// TestRejectedWriteGivesUp stops after pushAttempts rejections and leaves
// nothing of the write behind.
func TestRejectedWriteGivesUp(t *testing.T) {
	f := newFixture(t, ready("sm-1"))
	alice, bob := f.clone("alice"), f.clone("bob")
	bob.sleep = func(time.Duration) {}
	n := 0
	bob.beforePush = func() {
		n++
		if err := alice.Comment("sm-1", "alice moves the tracker"); err != nil {
			t.Fatal(err)
		}
	}
	if err := bob.Comment("sm-1", "bob never lands"); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("comment = %v, want a rejection", err)
	}
	if n != pushAttempts {
		t.Fatalf("%d attempts, want %d", n, pushAttempts)
	}
	for _, c := range f.remoteIssue("sm-1").Comments {
		if c.Text == "bob never lands" {
			t.Fatal("the failed write landed")
		}
	}
}
