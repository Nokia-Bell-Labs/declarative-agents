// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

// teamSRD is a subsystem SRD owned by owner whose one component, named
// name, lives in repo.
func teamSRD(id, owner, name, repo string) []byte {
	return []byte(fmt.Sprintf(`---
id: %[1]s
title: "Subsystem: %[1]s"
---

<!-- field: context | form: "tree" -->
## Context

- owner: %[2]s

<!-- req: R1 -->
## R1 - Work

<!-- req: R1.1 -->
### R1.1

Work arrives.

<!-- field: implementation_targets -->
## Implementation targets

| name | repository | path |
| --- | --- | --- |
| %[3]s | https://gitlab.example.com/%[4]s | / |
`, id, owner, name, repo))
}

// teamStory is the fixture's five-point story, pinned by URL to the
// system's epic ep001.
func teamStory(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hierarchy", "valid", "docs", "specs", "stories", "st001-score-trajectories.md"))
	if err != nil {
		t.Fatal(err)
	}
	return []byte(strings.Replace(string(b), "- parent: ft001-grader",
		"- parent: https://gitlab.example.com/g/system/-/blob/corpus/v0.1.0/docs/specs/epics/ep001-agent-evaluation.md", 1))
}

// An epic in the system repository counts the stories pinned to it in
// every team repository the subsystem SRDs name, read through the tracker
// with no checkout of the system's own, the done ones as each team's
// sprint-end summary records; a team repository it cannot read is named,
// never skipped silently (srd045 R3.4; GH-2726).
func TestRollUpCountsEveryTeamTheRegistryNames(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd010-po-1.md", teamSRD("srd010-po-1", "po-1", "grader", "g/po-1"))
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd011-po-2.md", teamSRD("srd011-po-2", "po-2", "scorecard", "g/po-2"))
		p.AddFile("g/system", "main", "docs/specs/software-requirements/srd012-po-9.md", teamSRD("srd012-po-9", "po-9", "gone", "g/po-9"))
		story := teamStory(t)
		p.AddFile("g/po-1", "main", "docs/specs/stories/st001-score-trajectories.md", story)
		p.AddFile("g/po-1", "main", "docs/sprint-ends/01.0.md", []byte("# Sprint end 01.0\n\n## Stories done\n\n- g/po-1#st001-score-trajectories\n"))
		p.AddFile("g/po-2", "main", "docs/specs/stories/st001-score-trajectories.md", story)
		f := p.Tracker()

		epicDir := t.TempDir()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hierarchy", "valid", "docs", "specs", "epics", "ep001-agent-evaluation.md"))
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(filepath.Join(epicDir, "docs", "specs", "epics"), 0o755)
		os.WriteFile(filepath.Join(epicDir, "docs", "specs", "epics", "ep001-agent-evaluation.md"), b, 0o644)
		docs, err := hierarchy.Load(epicDir)
		if err != nil {
			t.Fatal(err)
		}

		sources, unread, ok, err := RegistrySources(f, "g/system", "main", t.TempDir())
		if err != nil || !ok {
			t.Fatalf("RegistrySources: %v, ok %v", err, ok)
		}
		var names []string
		for _, s := range sources {
			names = append(names, s.Name)
		}
		if strings.Join(names, ",") != "po-1,po-2" {
			t.Errorf("sources = %v, want po-1 and po-2", names)
		}
		if len(unread) != 1 || unread[0].Repository != "g/po-9" {
			t.Errorf("unread = %+v, want g/po-9 named", unread)
		}
		points, read := TeamProgress(sources, docs)
		if !read || points["ep001-agent-evaluation"].Total != 10 || points["ep001-agent-evaluation"].Done != 5 {
			t.Fatalf("the epic's team points = %+v, want 5 of 10 from both teams", points["ep001-agent-evaluation"])
		}
		team, done := TeamDocs(sources, docs)
		var ids []string
		for _, d := range team {
			ids = append(ids, d.ID+"<"+d.Parent)
		}
		if strings.Join(ids, ",") != "po-1/st001-score-trajectories<ep001-agent-evaluation,po-2/st001-score-trajectories<ep001-agent-evaluation" ||
			done["po-1/st001-score-trajectories"] != "01.0" {
			t.Errorf("team docs = %v, done %v", ids, done)
		}
	})
}
