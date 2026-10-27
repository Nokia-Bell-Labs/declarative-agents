// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

const trackersDoc = "---\nid: trackers\ntitle: Tracking systems\n---\n\n<!-- field: version -->\n## Version\n\n1\n\n<!-- field: default -->\n## Default\n\n<!-- field: system -->\n### System\n\nbd\n"

// writeFacts writes the facts where the hierarchy and overview words read
// them.
func writeFacts(t *testing.T, reports string, f timeline.Facts) {
	t.Helper()
	j, _ := json.Marshal(f)
	os.MkdirAll(filepath.Join(reports, Dir, "timeline"), 0o755)
	if err := os.WriteFile(filepath.Join(reports, Dir, "timeline", "facts.json"), j, 0o644); err != nil {
		t.Fatal(err)
	}
}

// A team whose scrum repository is a program's system repository is a
// sub-program: the upper site counts, under its own epic, the sub-program's
// epic that pins it, with the points the sub-program's site publishes,
// links that epic to the sub-program's site, and shows none of its
// stories; the sub-program has a program tile and page linking its site
// (srd048 R8.7; GH-2740).
func TestASubProgramServesTheUpperPlan(t *testing.T) {
	in, system := statusProgram(t)
	os.MkdirAll(filepath.Join(system, "control"), 0o755)
	os.WriteFile(filepath.Join(system, "control", "trackers.md"), []byte(trackersDoc), 0o644)
	if out, err := exec.Command("git", "-C", system, "remote", "add", "origin", "https://gitlab.example.com/g/system.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, out)
	}
	const sub, reports = "g/eval-program", "g/eval-reports"
	f := in.Tracker.(*fake.Fake)
	f.AddFile("g/system", "main", "docs/specs/software-requirements/srd012-eval.md", []byte("---\nid: srd012-eval\ntitle: \"Subsystem\"\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: tpm-eval\n- scrum_repository: https://gitlab.example.com/"+sub+"\n\n<!-- req: R1 -->\n## R1 - Work\n\n<!-- req: R1.1 -->\n### R1.1\n\nWork arrives.\n\n<!-- field: implementation_targets -->\n## Implementation targets\n\n| name | repository | path |\n| --- | --- | --- |\n| eval | https://gitlab.example.com/g/eval-code | / |\n"))
	f.AddFile(sub, "main", "control/trackers.md", []byte(trackersDoc))
	f.AddFile(sub, "main", "control/program.md", []byte("---\nid: program\ntitle: Program\n---\n\n<!-- field: site -->\n## Site\n\nhttps://eval.example.com\n\n<!-- field: site_repository -->\n## Site repository\n\n"+reports+"\n"))
	nodes := []Node{
		{Item: timeline.Item{ID: "ep010-batch-grading", Kind: "epic", Title: "Batch grading", Status: timeline.Active, Points: 8, PointsDone: 3,
			Parent: "https://gitlab.example.com/g/system/-/blob/corpus/v0.1.0/docs/specs/epics/ep001-agent-evaluation.md"}, Page: "hierarchy/ep010-batch-grading.md"},
		{Item: timeline.Item{ID: "st010-grade-a-batch", Kind: "story", Title: "Grade a batch", Status: timeline.Active, Points: 8, PointsDone: 3, Parent: "ep010-batch-grading"}, Page: "hierarchy/st010-grade-a-batch.md"},
	}
	j, _ := json.Marshal(nodes)
	f.AddFile(reports, "main", Dir+"/data/hierarchy/hierarchy.json", j)

	before, err := Facts(system, "", in.Sources)
	if err != nil {
		t.Fatal(err)
	}
	s, err := LoadSources(system, f, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	facts, err := Facts(system, "", s)
	if err != nil {
		t.Fatal(err)
	}
	points := func(f timeline.Facts, id string) (int, int) {
		for _, it := range f.Items {
			if it.ID == id {
				return it.PointsDone, it.Points
			}
		}
		t.Fatalf("no %s", id)
		return 0, 0
	}
	d0, t0 := points(before, "ep001-agent-evaluation")
	if d1, t1 := points(facts, "ep001-agent-evaluation"); d1 != d0+3 || t1 != t0+8 {
		t.Errorf("ep001 = %d of %d, want %d of %d with the sub-program's epic", d1, t1, d0+3, t0+8)
	}
	writeFacts(t, in.Reports, facts)
	pages, _, _, sidebar, err := HierarchyPages(in.Reports, nil)
	if err != nil {
		t.Fatal(err)
	}
	epic := string(pages[Dir+"/"+HierarchyDir+"/ep001-agent-evaluation.md"])
	if !strings.Contains(epic, "eval-program/ep010-batch-grading.md)") {
		t.Errorf("the upper epic does not link the sub-program's epic:\n%s", epic)
	}
	served := string(pages[Dir+"/"+HierarchyDir+"/eval-program/ep010-batch-grading.md"])
	if !strings.Contains(served, "[open it on eval-program's site](https://eval.example.com/#/hierarchy/ep010-batch-grading)") || !strings.Contains(served, "3 of 8 points") {
		t.Errorf("the sub-program's epic page:\n%s", served)
	}
	for p := range pages {
		if strings.Contains(p, "st010") {
			t.Errorf("the upper site renders the sub-program's story %s", p)
		}
	}
	if !strings.Contains(string(sidebar), `<span class="kb kb-program" title="Program">P</span> eval-program`) {
		t.Errorf("the navigator does not mark the sub-program:\n%s", sidebar)
	}
	scrums, err := ScrumPages(in.Reports)
	if err != nil {
		t.Fatal(err)
	}
	if idx := string(scrums[Dir+"/"+ScrumsDir+"/index.md"]); !strings.Contains(idx, `class="tile tile-program" href="eval-program.md"`) || !strings.Contains(idx, "1 document serving this plan") {
		t.Errorf("the scrums' index:\n%s", idx)
	}
	page := string(scrums[Dir+"/"+ScrumsDir+"/eval-program.md"])
	for _, want := range []string{"[eval-program's site](https://eval.example.com)", "## What it serves", `href="../hierarchy/eval-program/ep010-batch-grading.md"`, "3 of 8 story points done"} {
		if !strings.Contains(page, want) {
			t.Errorf("the sub-program's page lacks %q:\n%s", want, page)
		}
	}
}

// A program whose control/program.md names a parent says it is part of
// it, in the navigator and on the overview, gathers the documents that
// serve the parent's plan under a band naming it, and links each one up
// to the parent document's page on the parent's site (srd048 R8.7;
// GH-2740).
func TestASubProgramLinksUpToItsParent(t *testing.T) {
	in, system := statusProgram(t)
	os.MkdirAll(filepath.Join(system, "control"), 0o755)
	os.WriteFile(filepath.Join(system, "control", "program.md"), []byte("---\nid: program\ntitle: Program\n---\n\n<!-- field: parent -->\n## Parent\n\nhttps://gitlab.example.com/g/portfolio\n\n<!-- field: parent_site -->\n## Parent site\n\nhttps://portfolio.example.com\n"), 0o644)
	b, err := os.ReadFile(filepath.Join(system, "docs", "specs", "epics", "ep001-agent-evaluation.md"))
	if err != nil {
		t.Fatal(err)
	}
	epic := strings.NewReplacer("ep001-agent-evaluation", "ep099-portfolio-grading", "- parent: in001-reliable-agents",
		"- parent: https://gitlab.example.com/g/portfolio/-/blob/v1.0.0/docs/specs/initiatives/in007-trusted-agents.md").Replace(string(b))
	os.WriteFile(filepath.Join(system, "docs", "specs", "epics", "ep099-portfolio-grading.md"), []byte(epic), 0o644)
	s, err := LoadSources(system, in.Tracker, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	facts, err := Facts(system, "", s)
	if err != nil {
		t.Fatal(err)
	}
	if facts.Parent == nil || facts.Parent.Name != "portfolio" || facts.Parent.Site != "https://portfolio.example.com" {
		t.Fatalf("parent = %+v", facts.Parent)
	}
	writeFacts(t, in.Reports, facts)
	pages, _, _, sidebar, err := HierarchyPages(in.Reports, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(strings.SplitN(string(sidebar), "\n", 3)[1], "- [") || !strings.Contains(string(sidebar), "Part of portfolio](https://portfolio.example.com)") {
		t.Errorf("the navigator does not open on the parent:\n%s", sidebar)
	}
	page := string(pages[Dir+"/"+HierarchyDir+"/ep099-portfolio-grading.md"])
	if !strings.Contains(page, "- [in007-trusted-agents](https://portfolio.example.com/#/hierarchy/in007-trusted-agents) in portfolio's plan") {
		t.Errorf("the epic does not link up to the parent's page:\n%s", page)
	}
	if tree := string(pages[Dir+"/"+HierarchyDir+"/index.md"]); !strings.Contains(tree, "Serves <a class=\"band-title\" href=\"https://portfolio.example.com\">portfolio</a>'s plan") || !strings.Contains(tree, "ep099-portfolio-grading.md") {
		t.Errorf("the plan has no band for the parent:\n%s", tree)
	}
	overview, err := OverviewPage(in.Reports, "program")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(overview), "Part of [portfolio](https://portfolio.example.com)") {
		t.Errorf("the overview does not name the parent:\n%s", overview)
	}
}

// The status page's roll-up counts the sub-program's served points too, as
// the plan's pages do (GH-2740).
func TestStatusRollUpCountsASubProgram(t *testing.T) {
	in, _ := statusProgram(t)
	row := func() string {
		t.Helper()
		pages, _, _, err := StatusPages(in)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(pages[Dir+"/status/roll-up.md"]), "\n") {
			if strings.HasPrefix(line, "| ep001-agent-evaluation |") {
				return line
			}
		}
		t.Fatal("no ep001 row")
		return ""
	}
	if before := row(); !strings.HasSuffix(before, "| 5 | 16 |") {
		t.Fatalf("without the sub-program: %s", before)
	}
	in.Sources.SubPrograms = []SubProgram{{Name: "eval-program", Repository: "g/eval-program", Nodes: []Node{{Item: timeline.Item{ID: "ep010-batch-grading", Kind: "epic",
		Points: 8, PointsDone: 3, Parent: "https://gitlab.example.com/g/system/-/blob/corpus/v0.1.0/docs/specs/epics/ep001-agent-evaluation.md"}}}}}
	if after := row(); !strings.HasSuffix(after, "| 8 | 24 |") {
		t.Fatalf("with the sub-program's 3 of 8: %s", after)
	}
}
