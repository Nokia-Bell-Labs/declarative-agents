// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"encoding/json"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// acmeProgram puts the status program on the acme group's forge: its origin
// is acme/program, its tracker the fake, as the program template lays a
// program out (GH-2811).
func acmeProgram(t *testing.T) (StatusInput, string, *fake.Fake) {
	t.Helper()
	in, system := statusProgram(t)
	os.MkdirAll(filepath.Join(system, "control"), 0o755)
	os.WriteFile(filepath.Join(system, "control", "trackers.md"), []byte("---\nid: trackers\ntitle: Tracking systems\n---\n\n<!-- field: version -->\n## Version\n\n1\n\n<!-- field: default -->\n## Default\n\n<!-- field: system -->\n### System\n\nbd\n"), 0o644)
	if out, err := exec.Command("git", "-C", system, "remote", "add", "origin", "https://gitlab.example.com/acme/program.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, out)
	}
	return in, system, in.Tracker.(*fake.Fake)
}

// subsystemSRD is a subsystem SRD for the fake registry: its owner, extra
// context items, and one implementation-targets row.
func subsystemSRD(id, owner, context, component, repo string) []byte {
	return []byte("---\nid: " + id + "\ntitle: \"Subsystem\"\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: " + owner + context +
		"\n\n<!-- req: R1 -->\n## R1 - Work\n\n<!-- req: R1.1 -->\n### R1.1\n\nWork arrives.\n\n<!-- field: implementation_targets -->\n## Implementation targets\n\n| name | repository | path |\n| --- | --- | --- |\n| " + component + " | https://gitlab.example.com/" + repo + " | / |\n")
}

// fixtureDoc is a fixture hierarchy document with replacements applied.
func fixtureDoc(t *testing.T, rel string, replace ...string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hierarchy", rel))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.NewReplacer(replace...).Replace(string(b)))
}

// An initiative repository — a subsystem SRD whose initiative item names
// in001 — holds an epic and a feature: the site places both under in001,
// counts a pinned story's points there, writes no scrum page or tile for
// the repository, and in001's page names the repository and links its
// ARCHITECTURE; a story in a pod's scrum repository pinned to the feature
// attaches under it, links reciprocated (srd048 R8.1, R8.6; GH-2811).
func TestInitiativeRepositoryDocsAttachUnderTheInitiative(t *testing.T) {
	in, system, f := acmeProgram(t)
	const initiative, scrum = "acme/analytics/model-evaluation", "acme/pods/platform/tpm"
	f.AddFile("acme/program", "main", "docs/specs/software-requirements/srd010-model-evaluation.md",
		subsystemSRD("srd010-model-evaluation", "Product Owner, alice", "\n- initiative: in001-reliable-agents", "model-evaluation", initiative))
	f.AddFile("acme/program", "main", "docs/specs/software-requirements/srd011-platform.md",
		subsystemSRD("srd011-platform", "Product Owner, bob", "\n- scrum_repository: https://gitlab.example.com/"+scrum, "platform", "acme/pods/platform/code"))
	// The initiative repository's plan: a top epic, no parent, and a
	// feature under it; the initiative item attaches the epic under in001.
	f.AddFile(initiative, "main", "docs/specs/epics/ep100-benchmarks.md",
		fixtureDoc(t, "valid/docs/specs/epics/ep001-agent-evaluation.md",
			"ep001-agent-evaluation", "ep100-benchmarks", "Agent evaluation", "Benchmarks", "- parent: in001-reliable-agents\n", ""))
	f.AddFile(initiative, "main", "docs/specs/features/ft100-suite.md",
		fixtureDoc(t, "valid/docs/specs/features/ft001-grader.md",
			"ft001-grader", "ft100-suite", "The grader", "The suite", "ep001-agent-evaluation", "ep100-benchmarks"))
	// A pod's story pins the initiative repository's feature by URL, as it
	// pins a system document (GH-2738).
	f.AddFile(scrum, "main", "docs/specs/stories/st042-run-suite.md",
		fixtureDoc(t, "valid/docs/specs/stories/st001-score-trajectories.md",
			"st001-score-trajectories", "st042-run-suite",
			"ft001-grader", "https://gitlab.example.com/"+initiative+"/-/blob/main/docs/specs/features/ft100-suite.md"))

	s, err := LoadSources(system, f, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	facts, err := Facts(system, "", s)
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]timeline.Item{}
	for _, it := range facts.Items {
		items[it.ID] = it
	}
	epic := items["analytics/model-evaluation/ep100-benchmarks"]
	if epic.Parent != "in001-reliable-agents" {
		t.Fatalf("the initiative repository's epic = %+v", epic)
	}
	if ft := items["analytics/model-evaluation/ft100-suite"]; ft.Parent != "analytics/model-evaluation/ep100-benchmarks" {
		t.Fatalf("its feature = %+v", ft)
	}
	story := items["pods/platform/tpm/st042-run-suite"]
	if story.Parent != "analytics/model-evaluation/ft100-suite" || story.Points != 5 {
		t.Fatalf("the pod's story = %+v", story)
	}
	if epic.Points != 5 {
		t.Errorf("the epic counts %d points, want the story's 5", epic.Points)
	}
	in001 := items["in001-reliable-agents"]
	if in001.Points < 5 {
		t.Errorf("in001 counts %d points, want the story's 5 among them", in001.Points)
	}
	var tm timeline.Team
	for _, x := range facts.Teams {
		if x.Initiative == "in001-reliable-agents" {
			tm = x
		}
	}
	if tm.Name != "analytics/model-evaluation" {
		t.Fatalf("the facts do not name the initiative repository: %+v", facts.Teams)
	}

	j, _ := json.Marshal(facts)
	os.MkdirAll(filepath.Join(in.Reports, Dir, "timeline"), 0o755)
	if err := os.WriteFile(filepath.Join(in.Reports, Dir, "timeline", "facts.json"), j, 0o644); err != nil {
		t.Fatal(err)
	}
	scrums, err := ScrumPages(in.Reports)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := scrums[Dir+"/"+ScrumsDir+"/analytics/model-evaluation.md"]; ok {
		t.Error("the initiative repository got a scrum page")
	}
	index := string(scrums[Dir+"/"+ScrumsDir+"/index.md"])
	if strings.Contains(index, "model-evaluation") {
		t.Errorf("the scrums' index has a tile for the initiative repository:\n%s", index)
	}
	if _, ok := scrums[Dir+"/"+ScrumsDir+"/pods/platform/tpm.md"]; !ok {
		t.Errorf("no scrum page at the pod's group path; pages: %v", slices.Sorted(maps.Keys(scrums)))
	}

	pages, _, _, sidebar, err := HierarchyPages(in.Reports, nil)
	if err != nil {
		t.Fatal(err)
	}
	in001Page := string(pages[Dir+"/"+HierarchyDir+"/in001-reliable-agents.md"])
	for _, want := range []string{
		"[acme/analytics/model-evaluation](https://gitlab.example.com/acme/analytics/model-evaluation)",
		"(https://gitlab.example.com/acme/analytics/model-evaluation/-/blob/HEAD/docs/ARCHITECTURE.md)",
		"analytics/model-evaluation/ep100-benchmarks.md",
	} {
		if !strings.Contains(in001Page, want) {
			t.Errorf("in001's page lacks %q:\n%s", want, in001Page)
		}
	}
	epicPage := string(pages[Dir+"/"+HierarchyDir+"/analytics/model-evaluation/ep100-benchmarks.md"])
	if !strings.Contains(epicPage, "(../../../hierarchy/in001-reliable-agents.md)") {
		t.Errorf("the epic's page does not link up to in001:\n%s", epicPage)
	}
	ftPage := string(pages[Dir+"/"+HierarchyDir+"/analytics/model-evaluation/ft100-suite.md"])
	if !strings.Contains(ftPage, "(../../../hierarchy/pods/platform/tpm/st042-run-suite.md)") {
		t.Errorf("the feature's page does not link down to the pod's story:\n%s", ftPage)
	}
	storyPage := string(pages[Dir+"/"+HierarchyDir+"/pods/platform/tpm/st042-run-suite.md"])
	if !strings.Contains(storyPage, "(../../../../hierarchy/analytics/model-evaluation/ft100-suite.md)") {
		t.Errorf("the story's page does not link up to the feature:\n%s", storyPage)
	}
	nav := string(sidebar)
	if !strings.Contains(nav, "(/scrums/pods/platform/tpm.md") {
		t.Errorf("the navigator lacks the pod's scrum:\n%s", nav)
	}
	if strings.Contains(nav, "/scrums/analytics/model-evaluation.md") {
		t.Errorf("the navigator lists the initiative repository as a scrum:\n%s", nav)
	}
}

// An initiative item naming a document the system repository does not hold
// is a note on the timeline facts, never skipped silently (GH-2811).
func TestInitiativeNamingAMissingDocumentIsANote(t *testing.T) {
	in, system, f := acmeProgram(t)
	f.AddFile("acme/program", "main", "docs/specs/software-requirements/srd010-model-evaluation.md",
		subsystemSRD("srd010-model-evaluation", "Product Owner, alice", "\n- initiative: in999-missing", "model-evaluation", "acme/analytics/model-evaluation"))
	f.AddFile("acme/analytics/model-evaluation", "main", "docs/specs/epics/ep100-benchmarks.md",
		fixtureDoc(t, "valid/docs/specs/epics/ep001-agent-evaluation.md",
			"ep001-agent-evaluation", "ep100-benchmarks", "- parent: in001-reliable-agents\n", ""))
	s, err := LoadSources(system, f, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	facts, err := Facts(system, "", s)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(facts.Notes, func(n string) bool {
		return strings.Contains(n, "in999-missing") && strings.Contains(n, "srd010-model-evaluation")
	}) {
		t.Fatalf("the facts do not note the missing initiative document: %v", facts.Notes)
	}
}

// Two pods name their scrum repository alike: each story's page lands under
// its own repository's group path, and no page overwrites the other's
// (GH-2811).
func TestTwoScrumRepositoriesOfTheSameNameStayApart(t *testing.T) {
	in, system, f := acmeProgram(t)
	for _, pod := range []string{"p1", "p2"} {
		f.AddFile("acme/program", "main", "docs/specs/software-requirements/srd01"+pod[1:]+"-"+pod+".md",
			subsystemSRD("srd01"+pod[1:]+"-"+pod, "Product Owner, "+pod, "\n- scrum_repository: https://gitlab.example.com/acme/pods/"+pod+"/tpm", pod, "acme/pods/"+pod+"/code"))
		f.AddFile("acme/pods/"+pod+"/tpm", "main", "docs/specs/stories/st001-score-trajectories.md",
			fixtureDoc(t, "valid/docs/specs/stories/st001-score-trajectories.md",
				"- parent: ft001-grader", "- parent: https://gitlab.example.com/acme/program/-/blob/corpus/v0.1.0/docs/specs/epics/ep001-agent-evaluation.md"))
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
	j, _ := json.Marshal(facts)
	os.MkdirAll(filepath.Join(in.Reports, Dir, "timeline"), 0o755)
	if err := os.WriteFile(filepath.Join(in.Reports, Dir, "timeline", "facts.json"), j, 0o644); err != nil {
		t.Fatal(err)
	}
	pages, _, _, _, err := HierarchyPages(in.Reports, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, pod := range []string{"p1", "p2"} {
		page, ok := pages[Dir+"/"+HierarchyDir+"/pods/"+pod+"/tpm/st001-score-trajectories.md"]
		if !ok {
			t.Fatalf("no page for %s's story; pages: %v", pod, slices.Sorted(maps.Keys(pages)))
		}
		if !strings.Contains(string(page), "`pods/"+pod+"/tpm/st001-score-trajectories`") {
			t.Errorf("%s's story page is not its own:\n%s", pod, page)
		}
	}
	epic := string(pages[Dir+"/"+HierarchyDir+"/ep001-agent-evaluation.md"])
	for _, pod := range []string{"p1", "p2"} {
		if !strings.Contains(epic, "pods/"+pod+"/tpm/st001-score-trajectories.md)") {
			t.Errorf("the epic's page does not link %s's story:\n%s", pod, epic)
		}
	}
	scrums, err := ScrumPages(in.Reports)
	if err != nil {
		t.Fatal(err)
	}
	for _, pod := range []string{"p1", "p2"} {
		page, ok := scrums[Dir+"/"+ScrumsDir+"/pods/"+pod+"/tpm.md"]
		if !ok {
			t.Fatalf("no scrum page for %s; pages: %v", pod, slices.Sorted(maps.Keys(scrums)))
		}
		if !strings.Contains(string(page), `href="../../../hierarchy/pods/`+pod+`/tpm/st001-score-trajectories.md"`) {
			t.Errorf("%s's scrum page does not link its own story:\n%s", pod, page)
		}
	}
}
