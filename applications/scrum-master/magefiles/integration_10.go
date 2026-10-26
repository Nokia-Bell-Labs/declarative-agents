// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/goals"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/plan"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.HierarchyGate, Integration{}.SprintPlanning)
}

// copyTree copies the files under src into dst, keeping their paths.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		writeFileTo(filepath.Join(dst, rel), string(b))
		return nil
	})
}

// HierarchyGate authors a theme, initiatives, an epic, a feature, and stories in a copy of the corpus and proves the gate and the parent's audit pass them, the spec tool reads them as SRDs, and the gate fails each malformed repository (test-rel04.0-agile-hierarchy).
func (Integration) HierarchyGate() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	c := &check{}
	tmp, err := os.MkdirTemp("", "scrum-master-hierarchy-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := runStreaming(root, "cp", "-R", "docs", tmp); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(root, "testdata", "hierarchy", "valid"), tmp); err != nil {
		return err
	}
	docs, err := hierarchy.Load(tmp)
	if err != nil {
		return err
	}
	kinds := map[hierarchy.Kind]int{}
	for _, d := range docs {
		kinds[d.Kind]++
	}
	c.expect(kinds[hierarchy.Theme] == 1 && kinds[hierarchy.Initiative] == 2 && kinds[hierarchy.Epic] == 1 && kinds[hierarchy.Feature] == 1 && kinds[hierarchy.Story] == 4,
		"hierarchy read as %v", kinds)
	f := hierarchy.Validate(docs)
	c.expect(len(f) == 0, "valid hierarchy: %v", f)
	if len(f) == 0 {
		c.expect(runCorpusAudit(tmp, runStreaming) == nil, "the parent's corpus audit fails beside a valid hierarchy")
	}
	progress, external := hierarchy.RollUp(docs, map[string]bool{"st001-score-trajectories": true})
	for _, p := range progress {
		if p.ID == "ep001-agent-evaluation" {
			c.expect(p.Done == 5 && p.Total == 16, "epic progress %d of %d, want 5 of 16", p.Done, p.Total)
		}
	}
	c.expect(slices.Equal(external, []string{"st004-shared-rubric"}), "external stories %v", external)

	cases, err := os.ReadDir(filepath.Join(root, "testdata", "hierarchy", "malformed"))
	if err != nil {
		return err
	}
	for _, m := range cases {
		dir := filepath.Join(root, "testdata", "hierarchy", "malformed", m.Name())
		docs, err := hierarchy.Load(dir)
		c.expect(err == nil && len(hierarchy.Validate(docs)) > 0, "%s: the gate found nothing (err %v)", m.Name(), err)
	}
	return c.err("hierarchyGate")
}

// SprintPlanning files po-1's Sprint Goal with tasks, lists the team's ranked Product Backlog with velocity, writes the chosen stories onto the Sprint Goal with a planned event naming them, and changes nothing on a second push (test-rel04.0-agile-hierarchy).
func (Integration) SprintPlanning() error {
	r, err := newRigEnv()
	if err != nil {
		return err
	}
	system, err := r.project("system")
	if err != nil {
		return err
	}
	po1, err := r.project("product-owners/po-1")
	if err != nil {
		return err
	}
	po2, err := r.project("product-owners/po-2")
	if err != nil {
		return err
	}
	if err := r.seedSystem(system, po1, po2); err != nil {
		return err
	}
	c := &check{}
	base := []string{"--system", system, "--release", rigRelease, "--ref", seedBranch}
	var p goals.Plan
	if err := r.wordsJSON(&p, append([]string{"goals", "plan"}, base...)...); err != nil {
		return err
	}
	for i := range p.Objectives {
		p.Objectives[i].Repository = po1
	}
	planFile := filepath.Join(r.run.Dir, "sprint-planning-plan.json")
	writeJSONTo(planFile, p)
	var pushed []goals.Pushed
	if err := r.wordsJSON(&pushed, append([]string{"goals", "push", "--plan", planFile}, base...)...); err != nil {
		return err
	}
	// One result per outcome (#152): two outcomes, one Sprint Goal.
	c.expect(len(pushed) == 2 && pushed[0].IID != 0 && pushed[0].IID == pushed[1].IID,
		"two outcomes' three items for po-1 are not one Sprint Goal: %+v", pushed)
	if len(pushed) == 0 {
		return c.err("sprintPlanning")
	}
	goalIID := pushed[0].IID

	clone, err := r.clone(po1, "po-1-sprint-planning")
	if err != nil {
		return err
	}
	if err := copyTree(filepath.Join(r.root, "testdata", "hierarchy", "valid", "docs", "specs", "stories"), filepath.Join(clone, "docs", "specs", "stories")); err != nil {
		return err
	}
	writeFileTo(filepath.Join(clone, "docs", "sprint-ends", "00.9.md"), "# Sprint end 00.9\n\n## Stories done\n\n- st001-score-trajectories\n")
	// Every repository of a program carries a road-map (design.md D7); the
	// rig's team repository is seeded with none.
	roadMap, err := os.ReadFile(filepath.Join(r.root, "testdata", "system", "road-map.md"))
	if err != nil {
		return err
	}
	writeFileTo(filepath.Join(clone, "docs", "road-map.md"), string(roadMap))
	var cand plan.Candidates
	if err := r.wordsJSON(&cand, "plan", "candidates", "--dir", clone); err != nil {
		return err
	}
	b := cand.Backlog
	c.expect(b != nil && b.Measured && b.Velocity == 5, "backlog velocity: %+v", b)
	if b != nil {
		c.expect(len(b.Stories) == 3 && b.Stories[0].ID == "st002-store-scores", "backlog stories %+v", b.Stories)
		c.expect(slices.Equal(b.Recommended, []string{"st002-store-scores"}), "recommended %v", b.Recommended)
	}

	chosen := "st002-store-scores,st003-scorecard-page"
	push := []string{"plan", "backlog", "--project", po1, "--release", rigRelease, "--stories", chosen, "--dir", clone}
	if _, err := r.runWords(append(push, "--dry-run")...); err != nil {
		return err
	}
	var first plan.BacklogResult
	if err := r.wordsJSON(&first, push...); err != nil {
		return err
	}
	c.expect(first.Action == "updated" && first.Planned, "first backlog push %+v", first)
	issues, err := r.issues(po1)
	if err != nil {
		return err
	}
	for _, i := range issues {
		if i.IID != goalIID {
			continue
		}
		c.expect(strings.Contains(i.Description, "## Sprint Backlog\n\n- st002-store-scores Store scores (3 points)\n- st003-scorecard-page Scorecard page (8 points)\n"),
			"Sprint Backlog not written:\n%s", i.Description)
	}
	notes, err := r.notes(po1, goalIID)
	if err != nil {
		return err
	}
	withStories := 0
	for _, n := range notes {
		if e, ok := events.Parse(n.Body); ok && e.Name == "planned" && len(e.Values("story")) == 2 {
			withStories++
		}
	}
	c.expect(withStories == 1, "%d planned events name the stories, want 1", withStories)
	var again plan.BacklogResult
	if err := r.wordsJSON(&again, push...); err != nil {
		return err
	}
	c.expect(again.Action == "unchanged" && !again.Planned, "second backlog push %+v", again)
	return c.err("sprintPlanning")
}
