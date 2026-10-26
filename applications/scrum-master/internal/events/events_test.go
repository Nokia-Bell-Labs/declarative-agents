// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package events

import (
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

func TestPlannedRoundTripsThroughMarker(t *testing.T) {
	e, text := Planned("01.0", []string{"https://h/g/s/-/blob/v1/o.md#R1.1", "https://h/g/s/-/blob/v1/o.md#R1.2"}, "dev")
	body, err := Comment(e, text)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(body, "Planned for release 01.0; evidence counts in dev for 2 requirement(s).\n\n<!-- scrum-master program event=planned release=01.0 requirement=") {
		t.Fatalf("body = %q", body)
	}
	got, ok := Parse(body)
	if !ok || got.Name != "planned" || len(got.Values("requirement")) != 2 || got.Values("environment")[0] != "dev" {
		t.Fatalf("parsed %+v ok=%v", got, ok)
	}
	if !Same(e, got, PlannedRequired) {
		t.Fatal("round trip changed the event")
	}
}

func TestMarkerRefusesValuesWithSpaces(t *testing.T) {
	if _, err := (Event{Name: "planned", Keys: []KV{{"environment", "dev env"}}}).Marker(); err == nil {
		t.Fatal("expected refusal")
	}
}

func TestParseIgnoresCommentsWithoutExactlyOneMarker(t *testing.T) {
	for _, body := range []string{"plain discussion", "<!-- scrum-master program event=a -->\n<!-- scrum-master program event=b -->"} {
		if _, ok := Parse(body); ok {
			t.Errorf("%q parsed", body)
		}
	}
}

func TestPostIsIdempotentAndDryRunWritesNothing(t *testing.T) {
	f := fake.New()
	issue, _ := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "objective"})
	writes := f.Writes
	e, text := Planned("01.0", []string{"https://h/g/s/-/blob/v1/o.md#R1.1"}, "dev")

	if posted, err := Post(f, "g/po-1", issue.IID, e, PlannedRequired, text, true); err != nil || !posted || f.Writes != writes {
		t.Fatalf("dry run: posted=%v err=%v writes=%d", posted, err, f.Writes-writes)
	}
	if posted, err := Post(f, "g/po-1", issue.IID, e, PlannedRequired, text, false); err != nil || !posted {
		t.Fatalf("first: posted=%v err=%v", posted, err)
	}
	if posted, err := Post(f, "g/po-1", issue.IID, e, PlannedRequired, text, false); err != nil || posted {
		t.Fatalf("second: posted=%v err=%v", posted, err)
	}
	other, otherText := Planned("01.1", []string{"https://h/g/s/-/blob/v1/o.md#R1.1"}, "dev")
	if posted, _ := Post(f, "g/po-1", issue.IID, other, PlannedRequired, otherText, false); !posted {
		t.Fatal("a different release is a new event")
	}
	if notes, _ := f.ListNotes("g/po-1", issue.IID); len(notes) != 2 {
		t.Fatalf("notes = %d", len(notes))
	}
}
