// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package sprintgoal

import (
	"slices"
	"testing"
)

func TestMarkerRoundTrips(t *testing.T) {
	m := Marker("01.2", []string{"in001-reliable-agents#R1", "in001-reliable-agents#R2"})
	r, ok := Parse("Sprint Goal\n\n## Notes\n\n" + m + "\n")
	if !ok || r.Legacy || r.Release != "01.2" || !slices.Equal(r.Serves, []string{"in001-reliable-agents#R1", "in001-reliable-agents#R2"}) {
		t.Fatalf("parsed %+v %v from %s", r, ok, m)
	}
	if !Is(m) {
		t.Error("Is misses the Sprint Goal marker")
	}
}

// TestObjectiveMarkerIsRead reads an objective issue's marker, written
// before release 04.0, as a Sprint Goal's (srd034 R3.7).
func TestObjectiveMarkerIsRead(t *testing.T) {
	desc := "body\n<!-- scrum-master goals objective=obj001-reliable-agents#R1 release=01.0 -->\n"
	r, ok := Parse(desc)
	if !ok || !r.Legacy || r.Release != "01.0" || !slices.Equal(r.Serves, []string{"obj001-reliable-agents#R1"}) {
		t.Fatalf("parsed %+v %v", r, ok)
	}
	if !Is(desc) {
		t.Error("Is misses the objective marker")
	}
	if _, ok := Parse("<!-- scrum-master plan sprint release=01.0 -->"); ok || Is("<!-- scrum-master plan sprint release=01.0 -->") {
		t.Error("a sprint issue marker read as a Sprint Goal")
	}
}
