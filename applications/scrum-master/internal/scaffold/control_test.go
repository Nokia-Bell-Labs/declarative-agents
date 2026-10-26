// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// StatesDocument round-trips: the beads parser reads back exactly the rows
// init writes, so control/states.md and the embedded default are one table
// (GH-130; srd041 R8, srd043 R1.3).
func TestStatesDocumentRoundTrips(t *testing.T) {
	rows, err := beads.ParseStates([]byte(StatesDocument(beads.Table)))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(rows, beads.Table) {
		t.Fatalf("parsed rows differ from the table:\n%v\n%v", rows, beads.Table)
	}
}

// The policy document lists the admission rules, and the policy reader
// takes them back, so removing one from the file disables it (GH-130).
func TestPolicyDocumentListsTheAdmissionRules(t *testing.T) {
	doc := PolicyDocument(program.Defaults())
	for _, name := range program.RuleNames() {
		if !strings.Contains(doc, "- "+name+"\n") {
			t.Errorf("policy document misses rule %s", name)
		}
	}
}

// The skills line counts skill files, not the exclude-file update an
// install also reports, so a first install and a rerun agree (GH-22).
func TestSkillFilesCountsOnlySkills(t *testing.T) {
	results := []install.Result{
		{Agent: "claude", Skill: "implement", Path: ".claude/skills/implement/SKILL.md", Action: "created"},
		{Agent: "claude", Skill: "plan", Path: ".claude/skills/plan/SKILL.md", Action: "created"},
		{Path: ".git/info/exclude", Action: "created"},
	}
	if n := skillFiles(results); n != 2 {
		t.Fatalf("skillFiles = %d, want 2", n)
	}
}
