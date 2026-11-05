// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package github

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// A group is a whole account here, so its labels and milestones reach only what
// the program names, else the checkout's own repository, and with neither the
// word refuses: nothing lists the owner to write to it (GH-2903).
func TestAGroupNeverReachesTheWholeOwner(t *testing.T) {
	m := tracker.Milestone{Title: "01.0", DueDate: "2026-11-20"}
	h := newHub()
	stranger := h.repo("acme/stranger")
	named := New(t.TempDir(), h.as("alice"), []string{"acme", "acme/fleet", "other/elsewhere"})
	if _, err := named.EnsureMilestone("acme", m); err != nil {
		t.Fatal(err)
	}
	if len(h.repo("acme/fleet").milestones) != 1 || len(stranger.milestones) != 0 || len(h.repo("other/elsewhere").milestones) != 0 {
		t.Fatalf("milestones reached fleet %d, stranger %d, elsewhere %d", len(h.repo("acme/fleet").milestones), len(stranger.milestones), len(h.repo("other/elsewhere").milestones))
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "https://github.com/acme/own.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	if _, err := New(dir, h.as("alice"), nil).EnsureMilestone("acme", m); err != nil {
		t.Fatal(err)
	}
	if len(h.repo("acme/own").milestones) != 1 || len(stranger.milestones) != 0 {
		t.Fatalf("milestones reached own %d, stranger %d", len(h.repo("acme/own").milestones), len(stranger.milestones))
	}
	_, err := New(t.TempDir(), h.as("alice"), nil).EnsureMilestone("acme", m)
	if err == nil || !strings.Contains(err.Error(), "no repository of acme is known") {
		t.Fatalf("with nothing named = %v, want a refusal", err)
	}
	if err := New(t.TempDir(), h.as("alice"), nil).CreateGroupLabel("acme", tracker.Label{Name: "x", Color: "#ffffff"}); err == nil {
		t.Error("a label went somewhere with nothing named")
	}
	if len(stranger.labels) != 0 || len(stranger.milestones) != 0 {
		t.Error("the owner's other repository was written to")
	}
}
