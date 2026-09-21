// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package chartconf

import (
	"strings"
	"testing"
)

func exception() Exception {
	return Exception{
		Chart: "chatbot-mesh", Overlay: "defaults", Rule: "R1.1",
		Resource: "Deployment/ollama container/ollama", Value: "ollama/ollama:latest",
		Issue: "GH-2409", Reason: "pinned when the model pull path is proven on a fixed tag",
	}
}

func finding() Finding {
	e := exception()
	return Finding{Chart: e.Chart, Overlay: e.Overlay, Rule: e.Rule,
		Resource: e.Resource, Value: e.Value, Detail: "image tag floats"}
}

// R7.1: a listed violation does not fail.
func TestListedViolationIsSuspended(t *testing.T) {
	unaccepted, stale := Reconcile([]Finding{finding()}, Baseline{Exceptions: []Exception{exception()}})
	if len(unaccepted) != 0 || len(stale) != 0 {
		t.Fatalf("listed violation not suspended: %v %v", unaccepted, stale)
	}
	if err := Report(unaccepted, stale); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// R7.2: an unlisted violation fails.
func TestUnlistedViolationFails(t *testing.T) {
	unaccepted, stale := Reconcile([]Finding{finding()}, Baseline{})
	if len(unaccepted) != 1 || len(stale) != 0 {
		t.Fatalf("unlisted violation not reported: %v %v", unaccepted, stale)
	}
	err := Report(unaccepted, stale)
	if err == nil || !strings.Contains(err.Error(), "ollama/ollama:latest") {
		t.Fatalf("report does not name the violation: %v", err)
	}
}

// R7.3: an entry whose violation no longer reproduces fails, and says so.
func TestStaleEntryFails(t *testing.T) {
	unaccepted, stale := Reconcile(nil, Baseline{Exceptions: []Exception{exception()}})
	if len(unaccepted) != 0 || len(stale) != 1 {
		t.Fatalf("stale entry not reported: %v %v", unaccepted, stale)
	}
	err := Report(unaccepted, stale)
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"stale baseline entry", "remove it", "GH-2409"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("report %q does not contain %q", err.Error(), want)
		}
	}
}

// A near-miss must not match: an entry suspends one resource's violation,
// not the same rule everywhere.
func TestEntryDoesNotSuspendADifferentResource(t *testing.T) {
	other := finding()
	other.Resource = "Deployment/dolt container/dolt"
	unaccepted, stale := Reconcile([]Finding{other}, Baseline{Exceptions: []Exception{exception()}})
	if len(unaccepted) != 1 || len(stale) != 1 {
		t.Fatalf("near-miss matched: unaccepted %v stale %v", unaccepted, stale)
	}
}

// Rewording a message must not invalidate an entry, so Detail is out of the key.
func TestDetailIsNotPartOfTheKey(t *testing.T) {
	reworded := finding()
	reworded.Detail = "a different sentence entirely"
	if unaccepted, stale := Reconcile([]Finding{reworded},
		Baseline{Exceptions: []Exception{exception()}}); len(unaccepted) != 0 || len(stale) != 0 {
		t.Fatalf("reworded detail broke the match: %v %v", unaccepted, stale)
	}
}

// R7.1: an entry that cannot say which issue removes it, or why it stands,
// is rejected at load.
func TestBaselineRejectsIncompleteEntry(t *testing.T) {
	for name, document := range map[string]string{
		"no issue":  "exceptions:\n  - chart: c\n    rule: R1.1\n    resource: r\n    reason: because\n",
		"no reason": "exceptions:\n  - chart: c\n    rule: R1.1\n    resource: r\n    issue: GH-1\n",
		"no rule":   "exceptions:\n  - chart: c\n    resource: r\n    issue: GH-1\n    reason: because\n",
		"no chart":  "exceptions:\n  - rule: R1.1\n    resource: r\n    issue: GH-1\n    reason: because\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := LoadBaseline(document); err == nil {
				t.Fatal("accepted an incomplete entry")
			}
		})
	}
}

func TestBaselineRejectsDuplicateEntry(t *testing.T) {
	entry := "  - chart: c\n    overlay: o\n    rule: R1.1\n    resource: r\n    value: v\n" +
		"    issue: GH-1\n    reason: because\n"
	if _, err := LoadBaseline("exceptions:\n" + entry + entry); err == nil {
		t.Fatal("accepted a duplicate entry")
	}
}

func TestEmptyBaselineLoads(t *testing.T) {
	baseline, err := LoadBaseline("exceptions: []\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(baseline.Exceptions) != 0 {
		t.Fatalf("expected no exceptions, got %v", baseline.Exceptions)
	}
}
