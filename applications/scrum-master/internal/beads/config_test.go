// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"strings"
	"testing"
)

// TestCreateUsesTheTrackerPrefix names a new issue with the prefix the
// tracker's config file sets, and with scrum-master when there is none (srd042).
func TestCreateUsesTheTrackerPrefix(t *testing.T) {
	f := newFixture(t)
	alice := f.clone("alice")
	before, err := alice.Create(Issue{Repository: "g/po-1", Title: "before"})
	if err != nil || !strings.HasPrefix(before.ID, "sm-") || len(before.ID) != len("sm-")+8 {
		t.Fatalf("create without a config file = %+v, %v, want an sm- ID", before, err)
	}
	if err := WriteConfig(alice.Dir, Config{IssuePrefix: "aeop"}); err != nil {
		t.Fatal(err)
	}
	f.git(alice.Dir, "add", ConfigFile)
	f.git(alice.Dir, "commit", "-q", "-m", "prefix")
	f.git(alice.Dir, "push", "-q", "origin", "HEAD:main")
	bob := f.clone("bob")
	after, err := bob.Create(Issue{Repository: "g/po-1", Title: "after"})
	if err != nil || !strings.HasPrefix(after.ID, "aeop-") {
		t.Fatalf("create with issue-prefix aeop = %+v, %v", after, err)
	}
	if got := f.remoteIssue(before.ID); got.Title != "before" {
		t.Fatalf("the earlier issue was renamed: %+v", got)
	}
}

func TestPrefixForTakesTheGroupsLastSegment(t *testing.T) {
	for group, want := range map[string]string{"acme-labs/aeop": "aeop", "Acme": "acme", "g/My_Program/": "myprogram", "g/__": "sm", "": "sm"} {
		if got := PrefixFor(group); got != want {
			t.Errorf("PrefixFor(%q) = %q, want %q", group, got, want)
		}
	}
}
