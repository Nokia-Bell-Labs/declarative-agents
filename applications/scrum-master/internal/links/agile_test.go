// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package links

import (
	"slices"
	"strings"
	"testing"
)

const story = "https://gitlab.example.com/program/po-1/-/blob/v1.0.0/docs/specs/stories/st001-score-trajectories.md#R1"

func TestAgileLinkLines(t *testing.T) {
	body := strings.Join([]string{
		"Score one agent",
		"",
		"Blocked-by: program/po-1#3",
		"Blocks: program/po-2#7",
		"Depends-on: program/po-1#4",
		"Relates-to: ep001-agent-evaluation",
		"Duplicates: program/po-1#2",
		"Implements: " + story,
		"Cites: " + story,
	}, "\n")
	got := ParseLinks(body)
	want := []Link{{BlockedBy, "program/po-1#3"}, {Blocks, "program/po-2#7"}, {DependsOn, "program/po-1#4"},
		{RelatesTo, "ep001-agent-evaluation"}, {Duplicates, "program/po-1#2"}, {Implements, story}}
	if !slices.Equal(got, want) {
		t.Fatalf("links %+v", got)
	}
	for _, l := range got {
		if err := CheckTarget(l); err != nil {
			t.Errorf("%s: %v", l.Kind, err)
		}
	}
	for _, bad := range []Link{
		{BlockedBy, ""},
		{RelatesTo, "not a target"},
		{DependsOn, "https://gitlab.example.com/program/po-1/-/blob/main/docs/x.md#R1"},
		{Implements, "program/po-1#3"},
		{Implements, "ep001-agent-evaluation"},
		{Implements, "https://gitlab.example.com/program/po-1/-/blob/v1.0.0/docs/specs/epics/ep001-a.md#R1"},
	} {
		if CheckTarget(bad) == nil {
			t.Errorf("%s %q accepted", bad.Kind, bad.Target)
		}
	}
}

// TestBlocksAndBlockedByAreOneLink reads a Blocks line on one issue and the
// matching Blocked-by line on the other as one edge, and never a Depends-on
// or Relates-to line (srd007 R6.2, R6.3).
func TestBlocksAndBlockedByAreOneLink(t *testing.T) {
	edges := BlockingEdges(map[string]string{
		"g/po-1#1": "Blocks: g/po-1#2\nDepends-on: g/po-1#3\n",
		"g/po-1#2": "Blocked-by: g/po-1#1\nRelates-to: g/po-1#3\n",
	})
	if !slices.Equal(edges, []Edge{{Waiting: "g/po-1#2", On: "g/po-1#1"}}) {
		t.Fatalf("edges %+v", edges)
	}
}
