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
