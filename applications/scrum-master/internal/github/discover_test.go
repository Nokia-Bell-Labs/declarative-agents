// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package github

import (
	"slices"
	"testing"
)

// A personal account's listing holds only public repositories, even for its
// owner, so discovery reads the caller's own for the private ones, keeping out
// other owners' (GH-2904).
func TestGroupProjectsSeesPrivateRepositories(t *testing.T) {
	h := newHub()
	h.users = map[string][]string{"alice": {"alice/public"}}
	h.mine = []string{"alice/public", "alice/private", "acme/shared"}
	got, err := New(t.TempDir(), h.as("alice"), nil).GroupProjects("alice")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alice/private", "alice/public"}; !slices.Equal(got, want) {
		t.Fatalf("projects = %v, want %v", got, want)
	}
	if got, err := New(t.TempDir(), h.as("alice"), nil).GroupProjects("nobody"); err != nil || len(got) != 0 {
		t.Fatalf("another owner, none visible = %v, %v", got, err)
	}
}

// GroupProjects leaves archived repositories out, whichever listing names
// them, so reconcile's sweep does not report what the program cannot act
// on (GH-2923).
func TestGroupProjectsLeavesArchivedOut(t *testing.T) {
	h := newHub()
	h.repo("acme/live")
	h.repo("acme/old")
	h.archived = map[string]bool{"acme/old": true, "alice/dusty": true}
	got, err := New(t.TempDir(), h.as("alice"), nil).GroupProjects("acme")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"acme/live"}; !slices.Equal(got, want) {
		t.Fatalf("organization projects = %v, want %v", got, want)
	}
	h.users = map[string][]string{"alice": {"alice/public"}}
	h.mine = []string{"alice/public", "alice/dusty"}
	got, err = New(t.TempDir(), h.as("alice"), nil).GroupProjects("alice")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"alice/public"}; !slices.Equal(got, want) {
		t.Fatalf("personal projects = %v, want %v", got, want)
	}
}
