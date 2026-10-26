// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
)

// areaProgram lays a system checkout beside po-1's: the system declares
// two teams, po-1 listing grader and po-2 listing scorecard, and the
// system itself; it holds a one-team epic a Product Owner owns and a
// cross-team epic the technical program manager owns. po-1 holds a story
// in its area and one naming po-2's component.
func areaProgram(t *testing.T) (system, team string) {
	t.Helper()
	root := t.TempDir()
	system, team = filepath.Join(root, "system"), filepath.Join(root, "po-1")
	write := func(dir, p string, b []byte) {
		t.Helper()
		os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fixture := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hierarchy", "valid", p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	write(system, "control/trackers.md", []byte("---\nid: trackers\ntitle: Tracking systems\n---\n\n<!-- field: version -->\n## Version\n\n1\n\n<!-- field: default -->\n## Default\n\n<!-- field: system -->\n### System\n\nbd\n"))
	write(system, "docs/specs/software-requirements/srd001-system.md", teamSRD("srd001-system", "technical program manager", "system", "g/system"))
	write(system, "docs/specs/software-requirements/srd010-po-1.md", teamSRD("srd010-po-1", "po-1", "grader", "g/po-1"))
	write(system, "docs/specs/software-requirements/srd011-po-2.md", teamSRD("srd011-po-2", "po-2", "scorecard", "g/po-2"))
	epic := fixture("docs/specs/epics/ep001-agent-evaluation.md")
	oneTeam := strings.Replace(epic, "- parent: in001-reliable-agents", "- parent: in001-reliable-agents\n- architecture:\n  1. grader", 1)
	write(system, "docs/specs/epics/ep001-agent-evaluation.md", []byte(oneTeam))
	cross := strings.Replace(strings.Replace(strings.Replace(epic, "ep001-agent-evaluation", "ep002-scored-agents", 1),
		"- owner: Product Owner, po-1", "- owner: technical program manager", 1),
		"- parent: in001-reliable-agents", "- parent: in001-reliable-agents\n- architecture:\n  1. grader\n  1. scorecard", 1)
	write(system, "docs/specs/epics/ep002-scored-agents.md", []byte(cross))
	story := fixture("docs/specs/stories/st001-score-trajectories.md")
	write(team, "docs/specs/stories/st001-score-trajectories.md", []byte(strings.Replace(story,
		"  1. https://gitlab.example.com/program/platform/-/blob/v1.0.0/docs/ARCHITECTURE.md#run_store", "  1. scorecard", 1)))
	write(team, "docs/specs/stories/st002-store-scores.md", []byte(strings.Replace(strings.Replace(story, "st001-score-trajectories", "st002-store-scores", 1),
		"  1. https://gitlab.example.com/program/platform/-/blob/v1.0.0/docs/ARCHITECTURE.md#run_store", "  1. https://gitlab.example.com/g/system/-/blob/v1/docs/ARCHITECTURE.md#grader", 1)))
	for dir, origin := range map[string]string{system: "https://gitlab.example.com/g/system.git", team: "https://gitlab.example.com/g/po-1.git"} {
		for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", origin}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, out)
			}
		}
	}
	return system, team
}

func messages(fs []hierarchy.Finding, field string) []string {
	var out []string
	for _, f := range fs {
		if f.Field == field {
			out = append(out, filepath.Base(f.File)+": "+f.Message)
		}
	}
	return out
}

// backlog check warns for a story naming a component its repository's
// subsystem does not list, for a system-repository epic a Product Owner
// owns, and for one whose components all belong to one subsystem, and
// says nothing of a story in its area or a cross-team epic; under
// area_checks refuse the warnings are findings (srd045 R6.5, R6.6; GH-2726).
func TestBacklogCheckHoldsDocumentsToTheirArea(t *testing.T) {
	system, team := areaProgram(t)
	r, err := Check(team)
	if err != nil {
		t.Fatal(err)
	}
	got := messages(r.Warnings, "architecture")
	if len(got) != 1 || !strings.Contains(got[0], "st001-score-trajectories.md: outside its area: names scorecard, which srd010-po-1") || !strings.Contains(got[0], "srd011-po-2 lists it") {
		t.Errorf("po-1's area warnings = %v", got)
	}
	r, err = Check(system)
	if err != nil {
		t.Fatal(err)
	}
	owner, arch := messages(r.Warnings, "owner"), messages(r.Warnings, "architecture")
	if len(owner) != 1 || !strings.HasPrefix(owner[0], "ep001-agent-evaluation.md: a cross-team epic in the system repository is the technical program manager's") {
		t.Errorf("owner warnings = %v", owner)
	}
	if len(arch) != 1 || !strings.Contains(arch[0], "ep001-agent-evaluation.md: one team's epic: every component it names belongs to srd010-po-1") {
		t.Errorf("one-team epic warnings = %v", arch)
	}

	os.WriteFile(filepath.Join(system, "control", "policy.md"), []byte("---\nid: policy\ntitle: Policy\n---\n\n<!-- field: area_checks -->\n## Area checks\n\nrefuse\n"), 0o644)
	r, err = Check(team)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(r.Findings, func(f hierarchy.Finding) bool { return strings.Contains(f.Message, "outside its area") }) ||
		slices.ContainsFunc(r.Warnings, func(f hierarchy.Finding) bool { return strings.Contains(f.Message, "outside its area") }) {
		t.Errorf("under refuse: findings %v, warnings %v", r.Findings, r.Warnings)
	}
}

// An epic's template names its owner by where it goes: the technical
// program manager in the system repository, the team's Product Owner in a
// team's repository (GH-2726).
func TestEpicTemplateOwnerFollowsTheRepository(t *testing.T) {
	system, team := areaProgram(t)
	for dir, want := range map[string]string{system: "- owner: technical program manager", team: "- owner: Product Owner, po-1"} {
		got, err := TemplateFor("epic", dir)
		if err != nil || !strings.Contains(got, want) {
			t.Errorf("%s: the epic template lacks %q: %v", filepath.Base(dir), want, err)
		}
	}
}
