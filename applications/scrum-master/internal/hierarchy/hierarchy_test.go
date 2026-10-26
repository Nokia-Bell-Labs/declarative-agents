// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package hierarchy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const fixtures = "../../testdata/hierarchy"

func load(t *testing.T, dir string) []*Doc {
	t.Helper()
	docs, err := Load(filepath.Join(fixtures, dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) == 0 {
		t.Fatalf("no documents under %s", dir)
	}
	return docs
}

func TestValidHierarchyPasses(t *testing.T) {
	docs := load(t, "valid")
	if f := Validate(docs); len(f) != 0 {
		t.Fatalf("valid hierarchy: %v", f)
	}
	kinds := map[Kind]int{}
	for _, d := range docs {
		kinds[d.Kind]++
	}
	want := map[Kind]int{Theme: 1, Initiative: 2, Epic: 1, Feature: 1, Story: 4}
	for k, n := range want {
		if kinds[k] != n {
			t.Errorf("%d %s documents, want %d", kinds[k], k, n)
		}
	}
	for _, d := range docs {
		if d.ID == "st002-store-scores" && !slices.Equal(d.Links["blocked_by"], []string{"st001-score-trajectories"}) {
			t.Errorf("st002 blocked_by %v", d.Links["blocked_by"])
		}
		if d.ID == "st001-score-trajectories" && (d.PointsValue() != 5 || d.RankValue() != 1 || d.AsA == "" || len(d.Cites) != 1) {
			t.Errorf("st001 statement and sizing: %+v", d)
		}
	}
}

// TestGateReportsEachMalformedDocument proves each malformed repository
// yields exactly the finding its defect causes (srd045 R6.1, srd007 R8.2).
func TestGateReportsEachMalformedDocument(t *testing.T) {
	for dir, field := range map[string]string{
		"missing-owner":                 "owner",
		"story-without-points":          "points",
		"criterion-without-environment": "acceptance_criteria.AC1.applicability",
		"feature-without-parent":        "parent",
		"wrong-parent-kind":             "parent",
		"unresolved-parent":             "parent",
		"unresolved-link":               "context.blocked_by",
		"cycle":                         "context",
		"unpinned-parent":               "parent",
	} {
		t.Run(dir, func(t *testing.T) {
			f := Validate(load(t, filepath.Join("malformed", dir)))
			if len(f) != 1 || f[0].Field != field {
				t.Fatalf("findings %v, want exactly one on %s", f, field)
			}
			if dir == "cycle" && !strings.Contains(f[0].Message, "cycle") {
				t.Errorf("cycle finding says %q", f[0].Message)
			}
		})
	}
}

func TestObjectiveDocumentIsAnInitiative(t *testing.T) {
	for _, d := range load(t, "valid") {
		if d.ID != "obj001-reliable-agents" {
			continue
		}
		if d.Kind != Initiative || d.Due["R1"] == "" {
			t.Fatalf("objective document read as %s with due %q", d.Kind, d.Due["R1"])
		}
		return
	}
	t.Fatal("obj001-reliable-agents not loaded from docs/specs/objectives/")
}

// TestRollUpCountsDoneStoryPoints rolls story points up through parent
// links (srd045 R3.3): stories of 5, 3, and 8 points under one epic, with
// the 5 done, give the epic 5 of 16; a story whose parent is in another
// repository is reported, not counted (R3.4).
func TestRollUpCountsDoneStoryPoints(t *testing.T) {
	progress, external := RollUp(load(t, "valid"), map[string]bool{"st001-score-trajectories": true})
	got := map[string][2]int{}
	for _, p := range progress {
		got[p.ID] = [2]int{p.Done, p.Total}
	}
	for id, want := range map[string][2]int{
		"ft001-grader":            {5, 8},
		"ep001-agent-evaluation":  {5, 16},
		"in001-reliable-agents":   {5, 16},
		"th001-dependable-agents": {5, 16},
		"obj001-reliable-agents":  {0, 0},
	} {
		if got[id] != want {
			t.Errorf("%s: %v, want %v", id, got[id], want)
		}
	}
	if !slices.Equal(external, []string{"st004-shared-rubric"}) {
		t.Errorf("external %v", external)
	}
}

func TestKindOfReadsThePrefix(t *testing.T) {
	for id, want := range map[string]Kind{"th001-a": Theme, "in002-b": Initiative, "obj003-c": Initiative, "ep004-d": Epic, "ft005-e": Feature, "st006-f": Story} {
		if k, ok := KindOf(id); !ok || k != want {
			t.Errorf("%s: %s %v, want %s", id, k, ok, want)
		}
	}
	if _, ok := KindOf("srd001-labels"); ok {
		t.Error("srd001-labels read as a hierarchy document")
	}
}

// TestStoryParentMayNameTheWholeEpic admits a story whose parent is the
// epic document pinned to a tag with no anchor, and still refuses one on a
// branch (design.md story context parent).
func TestStoryParentMayNameTheWholeEpic(t *testing.T) {
	src := filepath.Join(fixtures, "malformed", "unpinned-parent", "docs", "specs", "stories", "st001-on-a-branch.md")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	for parent, findings := range map[string]int{
		"https://gitlab.example.com/program/system/-/blob/v0.1.0/docs/specs/epics/ep001-agent-evaluation.md": 0,
		"https://gitlab.example.com/program/system/-/blob/main/docs/specs/epics/ep001-agent-evaluation.md":   1,
	} {
		dir := t.TempDir()
		stories := filepath.Join(dir, "docs", "specs", "stories")
		if err := os.MkdirAll(stories, 0o755); err != nil {
			t.Fatal(err)
		}
		doc := strings.Replace(string(b), "https://gitlab.example.com/program/system/-/blob/main/docs/specs/epics/ep001-agent-evaluation.md#R1", parent, 1)
		if err := os.WriteFile(filepath.Join(stories, "st001-on-a-branch.md"), []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
		docs, err := Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		if f := Validate(docs); len(f) != findings {
			t.Errorf("parent %s: findings %v, want %d", parent, f, findings)
		}
	}
}

// TestUnparseableContextIsOneFinding reports a context with an unquoted
// colon in a value as a parse error, not as every item missing.
func TestUnparseableContextIsOneFinding(t *testing.T) {
	src := filepath.Join(fixtures, "malformed", "unpinned-parent", "docs", "specs", "stories", "st001-on-a-branch.md")
	b, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	i := strings.Index(doc, "- owner: ")
	if i < 0 {
		t.Fatalf("fixture has no owner item:\n%s", doc)
	}
	doc = doc[:i] + "- as_a: product owner: operations\n" + doc[i:]
	dir := t.TempDir()
	stories := filepath.Join(dir, "docs", "specs", "stories")
	os.MkdirAll(stories, 0o755)
	os.WriteFile(filepath.Join(stories, "st001-on-a-branch.md"), []byte(doc), 0o644)
	docs, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := Validate(docs)
	if len(f) != 1 || f[0].Field != "context" || !strings.Contains(f[0].Message, "does not parse") {
		t.Fatalf("findings %v, want one context parse finding", f)
	}
}

// A same-repository architecture citation resolves against the components
// docs/ARCHITECTURE.md declares; one it does not declare, and any when the
// repository has no ARCHITECTURE, is refused naming why; a citation of
// another repository must be pinned (GH-55).
func TestArchitectureCitationsResolveAgainstTheComponents(t *testing.T) {
	root := t.TempDir()
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	story := func(arch string) string {
		return "---\nid: st001-cite\ntitle: Cite\nstatus: agreed\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n" +
			"- owner: Product Owner, po-1\n- parent: https://gitlab.example.com/program/system/-/blob/v1.0.0/docs/specs/epics/ep001-a.md#R1\n" +
			"- as_a: lead\n- i_want: a cite\n- so_that: it is checked\n- points: 3\n- architecture:\n" + arch +
			"\n<!-- field: problem -->\n## Problem\n\nNone.\n\n<!-- field: goals -->\n## Goals\n\n- G1: one.\n\n" +
			"<!-- req: R1 -->\n## R1 - One\n\n<!-- req: R1.1 -->\n### R1.1\n\nOne.\n\n<!-- field: non_goals -->\n## Non-goals\n\n- none.\n\n" +
			"<!-- field: acceptance_criteria -->\n## Acceptance criteria\n\n<!-- ac: AC1 -->\n### AC1\n\nOne.\n\n<!-- field: traces -->\n#### Traces\n\n- [R1.1](#r11)\n\n" +
			"<!-- field: tests -->\n#### Tests\n\n- one\n\n<!-- field: applicability -->\n#### Applicability\n\ndev\n"
	}
	findings := func() []string {
		docs, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range Validate(docs) {
			if f.Field == "architecture" {
				out = append(out, f.Message)
			}
		}
		return out
	}
	write("docs/specs/stories/st001-cite.md", story("  1. grader\n"))
	if got := findings(); len(got) != 1 || !strings.Contains(got[0], "missing or does not parse") {
		t.Fatalf("a citation with no ARCHITECTURE: %v", got)
	}
	write(ArchitectureFile, "---\nid: architecture-x\ntitle: X\n---\n\n<!-- field: components -->\n## Components\n\n<!-- component: grader -->\n### grader\n\nGrades.\n")
	if got := findings(); len(got) != 0 {
		t.Fatalf("a declared component: %v", got)
	}
	write("docs/specs/stories/st001-cite.md", story("  1. scorer\n  1. https://gitlab.example.com/program/p/-/blob/main/docs/ARCHITECTURE.md#run_store\n"))
	got := findings()
	if len(got) != 2 || !strings.Contains(got[0]+got[1], `"scorer" is no component`) || !strings.Contains(got[0]+got[1], "not a release tag") {
		t.Fatalf("an undeclared and an unpinned citation: %v", got)
	}
}

// A finding's JSON keys are lowercase, as every other fact's are (GH-109).
func TestFindingJSONKeys(t *testing.T) {
	b, err := json.Marshal(Finding{File: "f", Field: "x", Message: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"file":"f","field":"x","message":"m"}` {
		t.Fatalf("finding JSON %s", b)
	}
}
