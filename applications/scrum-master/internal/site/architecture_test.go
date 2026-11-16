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

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// architectureProgram is the status program with a registry of two teams:
// po-1 owning grader, whose repository holds grader's component SRD, and
// po-2 owning scorecard, whose repository holds none. The system's
// ARCHITECTURE declares both components and control_layer, which no
// subsystem SRD registers; the system's story cites grader, and the
// timeline facts are written into the reports checkout.
func architectureProgram(t *testing.T) (reports, system string, facts timeline.Facts) {
	t.Helper()
	in, system := statusProgram(t)
	os.MkdirAll(filepath.Join(system, "control"), 0o755)
	os.WriteFile(filepath.Join(system, "control", "trackers.md"), []byte("---\nid: trackers\ntitle: Tracking systems\n---\n\n<!-- field: version -->\n## Version\n\n1\n\n<!-- field: default -->\n## Default\n\n<!-- field: system -->\n### System\n\nbd\n"), 0o644)
	if out, err := exec.Command("git", "-C", system, "remote", "add", "origin", "https://gitlab.example.com/g/system.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, out)
	}
	write := func(p, s string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(filepath.Join(system, p)), 0o755)
		if err := os.WriteFile(filepath.Join(system, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srd := func(id, owner, component, repo string) string {
		return "---\nid: " + id + "\ntitle: \"Subsystem\"\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: " + owner + "\n\n<!-- req: R1 -->\n## R1 - Work\n\n<!-- req: R1.1 -->\n### R1.1\n\nWork arrives.\n\n<!-- field: implementation_targets -->\n## Implementation targets\n\n| name | repository | path |\n| --- | --- | --- |\n| " + component + " | https://gitlab.example.com/" + repo + " | / |\n"
	}
	// The registry is the subsystem SRDs, read through the tracker and
	// held by the system repository alike.
	f := in.Tracker.(*fake.Fake)
	for p, s := range map[string]string{
		"docs/specs/software-requirements/srd010-po-1.md": srd("srd010-po-1", "Product Owner, alice", "grader", "g/po-1"),
		"docs/specs/software-requirements/srd011-po-2.md": srd("srd011-po-2", "Product Owner, bob", "scorecard", "g/po-2"),
		"docs/ARCHITECTURE.md":                            "---\nid: architecture\ntitle: Architecture\n---\n\n<!-- field: components -->\n## Components\n\n<!-- component: grader -->\n### grader\n\nScores runs.\n\n<!-- component: scorecard -->\n### scorecard\n\nPublishes scorecards.\n\n<!-- component: control_layer -->\n### control_layer\n\nRuns the experiments; no subsystem SRD registers it.\n",
	} {
		write(p, s)
		f.AddFile("g/system", "main", p, []byte(s))
	}
	// The system's story cites grader, a component of this program, in
	// place of the fixture's pinned citation of another repository's.
	storyPath := filepath.Join(system, "docs", "specs", "stories", "st001-score-trajectories.md")
	b, err := os.ReadFile(storyPath)
	if err != nil {
		t.Fatal(err)
	}
	story := strings.Replace(string(b), "https://gitlab.example.com/program/platform/-/blob/v1.0.0/docs/ARCHITECTURE.md#run_store", "grader", 1)
	if err := os.WriteFile(storyPath, []byte(story), 0o644); err != nil {
		t.Fatal(err)
	}
	// po-1's repository holds grader's component SRD; po-2's holds none.
	f.AddFile("g/po-1", "main", "docs/specs/software-requirements/srd030-grader.md",
		[]byte("---\nid: srd030-grader\ntitle: \"Component: grader\"\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- component: grader\n\n<!-- req: R1 -->\n## R1 - Scoring\n\n<!-- req: R1.1 -->\n### R1.1\n\nIt scores a run.\n"))
	s, err := LoadSources(system, f, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	facts, err = Facts(system, "", s)
	if err != nil {
		t.Fatal(err)
	}
	j, _ := json.Marshal(facts)
	os.MkdirAll(filepath.Join(in.Reports, Dir, "timeline"), 0o755)
	if err := os.WriteFile(filepath.Join(in.Reports, Dir, "timeline", "facts.json"), j, 0o644); err != nil {
		t.Fatal(err)
	}
	return in.Reports, system, facts
}

// The timeline facts carry the architecture (srd048 R9.1): each subsystem
// of the registry with its owner, its scrum, and its contract, and each
// component with the lifecycle state the backlog architecture rule gives
// it and the component SRD its repository holds.
func TestFactsCarryTheArchitecture(t *testing.T) {
	_, _, f := architectureProgram(t)
	if len(f.Architecture) != 2 {
		t.Fatalf("want two subsystems, got %+v", f.Architecture)
	}
	bySub := map[string]timeline.SubsystemFact{}
	for _, sub := range f.Architecture {
		bySub[sub.ID] = sub
	}
	po1 := bySub["srd010-po-1"]
	if po1.Owner != "Product Owner, alice" || po1.Team != "po-1" {
		t.Errorf("po-1's subsystem: %+v", po1)
	}
	if po1.Contract == nil || po1.Contract.Path != "docs/specs/software-requirements/srd010-po-1.md" {
		t.Errorf("po-1's contract: %+v", po1.Contract)
	}
	if len(po1.Components) != 1 {
		t.Fatalf("po-1's components: %+v", po1.Components)
	}
	grader := po1.Components[0]
	// An agreed story cites grader, so it is in transition, landing in the
	// release the story is planned for (srd045 R8.5).
	if grader.ID != "grader" || grader.State != "in_transition" || grader.Release != "01.0" {
		t.Errorf("grader: %+v", grader)
	}
	if grader.Contract == nil || grader.Contract.Path != "docs/specs/software-requirements/srd030-grader.md" ||
		grader.Contract.Team != "po-1" || grader.Contract.Source != "https://gitlab.example.com/g/po-1/-/blob/HEAD/docs/specs/software-requirements/srd030-grader.md" {
		t.Errorf("grader's contract: %+v", grader.Contract)
	}
	scorecard := bySub["srd011-po-2"].Components[0]
	if scorecard.ID != "scorecard" || scorecard.State != "intended" {
		t.Errorf("scorecard: %+v", scorecard)
	}
	if scorecard.Contract != nil {
		t.Errorf("scorecard has no component SRD, got %+v", scorecard.Contract)
	}
}

// The architecture word writes the tree as the folder's index, a page per
// subsystem, and a page per component below it, each saying what it is,
// linking its contract, and listing the documents citing it (srd048 R9.1).
func TestArchitecturePages(t *testing.T) {
	reports, _, _ := architectureProgram(t)
	// The corpus copies the architecture pages link when they exist.
	for _, p := range []string{"ARCHITECTURE.md", "specs/software-requirements/srd010-po-1.md"} {
		full := filepath.Join(reports, Dir, CorpusDir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pages, err := ArchitecturePages(reports)
	if err != nil {
		t.Fatal(err)
	}
	index := string(pages[Dir+"/"+ArchitectureDir+"/index.md"])
	for _, want := range []string{
		"[ARCHITECTURE](../corpus/ARCHITECTURE.md)",
		"[srd010-po-1](srd010-po-1.md): Product Owner, alice",
		"[grader](srd010-po-1/grader.md): in transition, its work lands in release 01.0",
		"[scorecard](srd011-po-2/scorecard.md): intended",
	} {
		if !strings.Contains(index, want) {
			t.Errorf("the index lacks %q:\n%s", want, index)
		}
	}
	sub := string(pages[Dir+"/"+ArchitectureDir+"/srd010-po-1.md"])
	for _, want := range []string{
		"# srd010-po-1", "subsystem · in transition · Product Owner, alice",
		"The scrum owning it is [po-1](../scrums/po-1.md).",
		"[docs/specs/software-requirements/srd010-po-1.md](../corpus/specs/software-requirements/srd010-po-1.md)",
		"[grader](../architecture/srd010-po-1/grader.md): in transition",
	} {
		if !strings.Contains(sub, want) {
			t.Errorf("the subsystem's page lacks %q:\n%s", want, sub)
		}
	}
	grader := string(pages[Dir+"/"+ArchitectureDir+"/srd010-po-1/grader.md"])
	for _, want := range []string{
		"# grader", "component · in transition, its work lands in release 01.0 · in g/po-1",
		"[srd010-po-1](../../architecture/srd010-po-1.md), its subsystem",
		"[po-1's docs/specs/software-requirements/srd030-grader.md](https://gitlab.example.com/g/po-1/-/blob/HEAD/docs/specs/software-requirements/srd030-grader.md)",
		`<span class="kind kind-story">Story</span> [Score trajectories](../../hierarchy/st001-score-trajectories.md)`,
	} {
		if !strings.Contains(grader, want) {
			t.Errorf("grader's page lacks %q:\n%s", want, grader)
		}
	}
	scorecard := string(pages[Dir+"/"+ArchitectureDir+"/srd011-po-2/scorecard.md"])
	for _, want := range []string{"No planning document cites it yet.", "Its repository holds no component SRD yet"} {
		if !strings.Contains(scorecard, want) {
			t.Errorf("scorecard's page lacks %q:\n%s", want, scorecard)
		}
	}
}

// The links are reciprocated into the rest of the site (srd048 R9.2,
// R9.3): a hierarchy page links the components its citations name, the
// navigator lists the architecture tree, and the corpus copies of
// ARCHITECTURE.md and a subsystem SRD close with their architecture links.
func TestArchitectureLinksAreReciprocated(t *testing.T) {
	reports, system, _ := architectureProgram(t)
	pages, _, _, sidebar, err := HierarchyPages(reports, nil)
	if err != nil {
		t.Fatal(err)
	}
	story := string(pages[Dir+"/"+HierarchyDir+"/st001-score-trajectories.md"])
	if !strings.Contains(story, "## Components") || !strings.Contains(story, "[grader](../architecture/srd010-po-1/grader.md)") {
		t.Errorf("the story's page does not link grader's architecture page:\n%s", story)
	}
	nav := string(sidebar)
	for _, want := range []string{
		"- [Architecture](/architecture/index.md)",
		"(/architecture/srd010-po-1.md",
		"(/architecture/srd010-po-1/grader.md",
	} {
		if !strings.Contains(nav, want) {
			t.Errorf("the navigator lacks %q:\n%s", want, nav)
		}
	}
	corpus, err := CorpusPages(system, nil)
	if err != nil {
		t.Fatal(err)
	}
	arch := string(corpus[Dir+"/"+CorpusDir+"/ARCHITECTURE.md"])
	if !strings.Contains(arch, "[The architecture](../architecture/index.md)") {
		t.Errorf("the ARCHITECTURE copy does not close with the architecture index:\n%s", arch)
	}
	srd := string(corpus[Dir+"/"+CorpusDir+"/specs/software-requirements/srd010-po-1.md"])
	if !strings.Contains(srd, "[Its subsystem](../../../architecture/srd010-po-1.md)") {
		t.Errorf("the subsystem SRD's copy does not close with its page:\n%s", srd)
	}
}

// An issue's component is its component:: label, else the one component
// the registry places in its project; a project holding several, or none,
// gives none (GH-2928).
func TestIssueComponent(t *testing.T) {
	subs := program.Subsystems{
		{ID: "srd010-po-1", Components: []program.Component{{Name: "grader", Repository: "https://gitlab.example.com/g/po-1"}}},
		{ID: "srd011-po-2", Components: []program.Component{
			{Name: "scorecard", Repository: "https://gitlab.example.com/g/po-2"},
			{Name: "publisher", Repository: "https://gitlab.example.com/g/po-2"},
		}},
	}
	for _, tc := range []struct {
		name, project, want string
		labels              []string
	}{
		{"the label wins", "g/po-1", "scorecard", []string{"plane::data", "component::scorecard"}},
		{"the project's one component", "g/po-1", "grader", []string{"plane::data"}},
		{"a project of several components", "g/po-2", "", nil},
		{"a project of none", "g/other", "", nil},
	} {
		if got := issueComponent(tracker.Issue{Labels: tc.labels}, tc.project, subs); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

// The facts carry each issue's component and the components the system's
// ARCHITECTURE declares that no subsystem SRD registers, with their states
// (GH-2928).
func TestFactsCarryIssueAndSystemComponents(t *testing.T) {
	_, _, f := architectureProgram(t)
	if len(f.System) != 1 || f.System[0].ID != "control_layer" || f.System[0].State != "intended" {
		t.Errorf("system components: %+v", f.System)
	}
	found := false
	for _, it := range f.Items {
		if it.Kind == timeline.KindIssue && it.Title == "Grade a trajectory" {
			found = true
			if it.Component != "grader" {
				t.Errorf("the po-1 issue's component is %q, want grader, the one component g/po-1 holds", it.Component)
			}
		}
	}
	if !found {
		t.Fatalf("no issue item in the facts: %+v", f.Items)
	}
}

// workFacts is a plan of a feature with one story under it, the story
// citing grader and its issues landing in grader and scorecard, an issue
// implementing no story landing in grader, and control_layer a system
// component an epic cites.
func workFacts() timeline.Facts {
	return timeline.Facts{
		Architecture: []timeline.SubsystemFact{
			{ID: "srd010-po-1", Components: []timeline.ComponentFact{{ID: "grader", State: "in_transition"}}},
			{ID: "srd011-po-2", Components: []timeline.ComponentFact{{ID: "scorecard", State: "intended"}, {ID: "publisher", State: "intended"}}},
		},
		System: []timeline.ComponentFact{{ID: "control_layer", State: "intended"}},
		Items: []timeline.Item{
			{ID: "ep001-control", Kind: "epic", Title: "Experiment control", Status: "active", Architecture: []string{"control_layer"}},
			{ID: "ft001-grader", Kind: "feature", Title: "Grader", Status: "active"},
			{ID: "st001-score", Kind: "story", Title: "Score trajectories", Parent: "ft001-grader", Status: "active", Points: 5, PointsDone: 2, Architecture: []string{"grader"}},
			{ID: "g/po-1#2", Kind: timeline.KindIssue, Title: "Grade a run", Parent: "st001-score", Status: "done", URL: "https://gitlab.example.com/g/po-1/-/issues/2", Component: "grader"},
			{ID: "g/po-2#3", Kind: timeline.KindIssue, Title: "Show the score", Parent: "st001-score", Status: "active", URL: "https://gitlab.example.com/g/po-2/-/issues/3", Component: "scorecard"},
			{ID: "g/po-1#4", Kind: timeline.KindIssue, Title: "Tidy the grader", Status: "future", URL: "https://gitlab.example.com/g/po-1/-/issues/4", Component: "Grader"},
		},
	}
}

// A component's page lists the work landing in it: each story whose
// issues carry it, with those issues beneath linked on the forge, then the
// issues implementing no story; a component with none says so (GH-2928).
func TestComponentPageListsWork(t *testing.T) {
	f := workFacts()
	grader := componentPage(f.Architecture[0], f.Architecture[0].Components[0], f)
	for _, want := range []string{
		"## Work",
		`- <span class="kind kind-story">Story</span> [Score trajectories](../../hierarchy/st001-score.md): active, 2 of 5 points` + "\n  - [g/po-1#2 Grade a run](https://gitlab.example.com/g/po-1/-/issues/2): done",
		"- Issues implementing no story the plan holds\n  - [g/po-1#4 Tidy the grader](https://gitlab.example.com/g/po-1/-/issues/4): future",
	} {
		if !strings.Contains(grader, want) {
			t.Errorf("grader's page lacks %q:\n%s", want, grader)
		}
	}
	if strings.Contains(grader, "Show the score") {
		t.Errorf("grader's page lists scorecard's issue:\n%s", grader)
	}
	publisher := componentPage(f.Architecture[1], f.Architecture[1].Components[1], f)
	if !strings.Contains(publisher, "No issue lands in it yet.") || !strings.Contains(publisher, "No planning document cites it yet.") {
		t.Errorf("publisher's page does not say it has no work and no citation:\n%s", publisher)
	}
}

// A component no subsystem SRD registers has a page of its own under
// architecture/system, which the index, the citation links, and its
// citing documents reach (GH-2928).
func TestSystemComponentPage(t *testing.T) {
	f := workFacts()
	if p := ArchPages(f.Architecture, f.System)[program.ComponentKey("control_layer")]; p != "architecture/system/control_layer.md" {
		t.Errorf("control_layer maps to %q", p)
	}
	page := systemComponentPage(f.System[0], f, func(p string) bool { return p == "ARCHITECTURE.md" })
	for _, want := range []string{
		"[Architecture](../../architecture/index.md) › System",
		"# control_layer", "component · intended · declared by the system's ARCHITECTURE",
		"[ARCHITECTURE](../../corpus/ARCHITECTURE.md): the system's, declaring it",
		`<span class="kind kind-epic">Epic</span> [Experiment control](../../hierarchy/ep001-control.md): active`,
		"No issue lands in it yet.",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("control_layer's page lacks %q:\n%s", want, page)
		}
	}
	index := architectureIndex(f, func(string) bool { return false })
	if !strings.Contains(index, "## System components") || !strings.Contains(index, "- [control_layer](system/control_layer.md): intended") {
		t.Errorf("the index does not list control_layer:\n%s", index)
	}
	nav := sidebarPage(map[string]*Node{}, nil, nil, nil, nil, f.Architecture, f.System, false)
	if !strings.Contains(nav, "  - System\n    - [control_layer](/architecture/system/control_layer.md \"control_layer\")") {
		t.Errorf("the navigator does not list control_layer:\n%s", nav)
	}
}

// A hierarchy page links the components its issues land in beside those
// it cites, marked as inferred and each named once; a feature's page
// reaches the issues of the stories under it (GH-2928).
func TestHierarchyPageLinksInferredComponents(t *testing.T) {
	f := workFacts()
	nodes, tracked := hierarchyNodes(f)
	arch := ArchPages(f.Architecture, f.System)
	story := nodePage(nodes["st001-score"], nodes, tracked, func(string) bool { return false }, nil, nil, arch)
	want := "## Components\n\n- [grader](../architecture/srd010-po-1/grader.md)\n- [scorecard](../architecture/srd011-po-2/scorecard.md) (from its issues)\n\n"
	if !strings.Contains(story, want) {
		t.Errorf("the story's components are not %q:\n%s", want, story)
	}
	feature := nodePage(nodes["ft001-grader"], nodes, tracked, func(string) bool { return false }, nil, nil, arch)
	want = "## Components\n\n- [grader](../architecture/srd010-po-1/grader.md) (from its issues)\n- [scorecard](../architecture/srd011-po-2/scorecard.md) (from its issues)\n\n"
	if !strings.Contains(feature, want) {
		t.Errorf("the feature's components are not %q:\n%s", want, feature)
	}
	epic := nodePage(nodes["ep001-control"], nodes, tracked, func(string) bool { return false }, nil, nil, arch)
	if !strings.Contains(epic, "- [control_layer](../architecture/system/control_layer.md)\n") {
		t.Errorf("the epic does not link control_layer's system page:\n%s", epic)
	}
}
