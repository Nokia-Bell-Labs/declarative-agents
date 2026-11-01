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

// architectureProgram is the status program with a registry of two teams:
// po-1 owning grader, whose repository holds grader's component SRD, and
// po-2 owning scorecard, whose repository holds none. The system's
// ARCHITECTURE declares both components, the system's story cites grader,
// and the timeline facts are written into the reports checkout.
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
		"docs/ARCHITECTURE.md":                            "---\nid: architecture\ntitle: Architecture\n---\n\n<!-- field: components -->\n## Components\n\n<!-- component: grader -->\n### grader\n\nScores runs.\n\n<!-- component: scorecard -->\n### scorecard\n\nPublishes scorecards.\n",
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
