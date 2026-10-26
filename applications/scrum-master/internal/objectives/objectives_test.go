// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package objectives

import (
	"path/filepath"
	"strings"
	"testing"
)

const fixtures = "../../testdata/objectives"

func TestValidDocumentHasNoFindings(t *testing.T) {
	d, err := Load(filepath.Join(fixtures, "valid", "obj001-reliable-agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	if f := Validate(d); len(f) != 0 {
		t.Fatalf("findings: %v", f)
	}
	if got := d.ObjectiveIDs(); strings.Join(got, ",") != "R1,R2" {
		t.Fatalf("ObjectiveIDs = %v", got)
	}
	items := d.Requirements["R1"].ItemsOf()
	if len(items) != 2 || items[0].ID != "R1.1" || items[1].ID != "R1.2" {
		t.Fatalf("items = %+v", items)
	}
	if c := d.CriteriaFor("R2"); len(c) != 1 || c[0].ID != "AC2" {
		t.Fatalf("CriteriaFor(R2) = %+v", c)
	}
}

// TestReadsTheObjectiveFields reads each datum from the declared field
// that holds it (design.md schema.objective).
func TestReadsTheObjectiveFields(t *testing.T) {
	d, err := Load(filepath.Join(fixtures, "valid", "obj001-reliable-agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	if d.ID != "obj001-reliable-agents" || d.Status != "agreed" || d.Owner != "technical program manager" || d.Horizon != "2026-W43..2027-W03" ||
		d.Requirements["R1"].Title != "Evaluation of one agent" || d.Requirements["R1"].Due != "2026-11-06" || len(d.Requirements["R1"].DeliveredBy) != 1 ||
		len(d.Requirements["R2"].DeliveredBy) != 0 || len(d.Goals) != 1 || len(d.NonGoals) != 1 ||
		d.AcceptanceCriteria[0].Environment != "dev" || strings.Join(d.AcceptanceCriteria[0].Tests, ",") != "grader_scores_fixture_agent" ||
		strings.Join(d.AcceptanceCriteria[0].Traces, ",") != "R1.1,R1.2" {
		t.Fatalf("document = %+v", d)
	}
}

func TestLoadDirPrefersMarkdown(t *testing.T) {
	docs, err := LoadDir(filepath.Join(fixtures, "valid"))
	if err != nil || len(docs) != 1 || filepath.Ext(docs[0].Path) != ".md" {
		t.Fatalf("docs = %v, %v", docs, err)
	}
}

func TestEachMalformedDocumentYieldsExactlyItsFinding(t *testing.T) {
	cases := map[string]struct{ field, message string }{
		"obj002-missing-owner":                 {"owner", "missing"},
		"obj003-dangling-trace":                {"acceptance_criteria.AC2.traces", `"R2.9" names no item`},
		"obj004-unpinned-reference":            {"requirements.R1.delivered_by", `branch "main"`},
		"obj005-bad-status":                    {"status", `"approved" is not one of`},
		"obj006-missing-due":                   {"requirements.R2.due", "missing"},
		"obj007-criterion-without-environment": {"acceptance_criteria.AC2.environment", "missing"},
	}
	for name, want := range cases {
		t.Run(name, func(t *testing.T) {
			d, err := Load(filepath.Join(fixtures, "malformed", name+".md"))
			if err != nil {
				t.Fatal(err)
			}
			f := Validate(d)
			if len(f) != 1 || f[0].Field != want.field || !strings.Contains(f[0].Message, want.message) {
				t.Fatalf("findings = %v, want one %s %q", f, want.field, want.message)
			}
		})
	}
}

func TestIDMustMatchFileName(t *testing.T) {
	d, err := Load(filepath.Join(fixtures, "valid", "obj001-reliable-agents.md"))
	if err != nil {
		t.Fatal(err)
	}
	d.Path = "elsewhere/obj009-other.md"
	f := Validate(d)
	if len(f) != 1 || f[0].Field != "id" {
		t.Fatalf("findings = %v", f)
	}
}

func TestEmptyDocumentReportsEveryMissingField(t *testing.T) {
	f := Validate(&Document{Path: "obj001-x.md"})
	fields := map[string]bool{}
	for _, x := range f {
		fields[x.Field] = true
	}
	for _, want := range []string{"id", "title", "problem", "owner", "horizon", "status", "goals", "non_goals", "requirements", "acceptance_criteria"} {
		if !fields[want] {
			t.Errorf("no finding for %s in %v", want, f)
		}
	}
}

func TestLoadDirOfAbsentDirectoryIsEmpty(t *testing.T) {
	docs, err := LoadDir(filepath.Join(t.TempDir(), "none"))
	if err != nil || len(docs) != 0 {
		t.Fatalf("docs=%v err=%v", docs, err)
	}
}
