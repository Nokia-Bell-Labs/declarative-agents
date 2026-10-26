// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import "testing"

// TestRecordsMergeByLaterWriter holds mergeRecords to the eng04 rule for
// scrum-master's record files: a record one side changed takes that side, a record
// both changed takes the later writer, and a record only one side added is
// kept.
func TestRecordsMergeByLaterWriter(t *testing.T) {
	base := []Label{
		{Scope: "g", Name: "a", Color: "#000000", UpdatedAt: "2026-11-02T09:00:00Z"},
		{Scope: "g", Name: "b", Color: "#000000", UpdatedAt: "2026-11-02T09:00:00Z"},
	}
	ours := []Label{
		{Scope: "g", Name: "a", Color: "#111111", UpdatedAt: "2026-11-02T09:05:00Z"},
		{Scope: "g", Name: "b", Color: "#222222", UpdatedAt: "2026-11-02T09:01:00Z"},
		{Scope: "g", Name: "c", Color: "#333333", UpdatedAt: "2026-11-02T09:02:00Z"},
	}
	theirs := []Label{
		base[0],
		{Scope: "g", Name: "b", Color: "#444444", UpdatedAt: "2026-11-02T09:03:00Z"},
	}
	got := map[string]string{}
	for _, l := range mergeRecords(base, ours, theirs) {
		got[l.Name] = l.Color
	}
	want := map[string]string{"a": "#111111", "b": "#444444", "c": "#333333"}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("merged = %v, want %v", got, want)
		}
	}
}
