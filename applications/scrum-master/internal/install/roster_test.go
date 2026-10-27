// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package install

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	scrummaster "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master"
)

// The installed roster is the workflows a person starts and four group
// skills, each group's skill naming each member's run command; every
// profile still renders, and a step a workflow runs is no skill of its own
// (GH-2749).
func TestTheRosterIsWorkflowsAndGroups(t *testing.T) {
	skills, err := EmbeddedSkills()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Skill{}
	for _, s := range skills {
		got[s.Name] = s
	}
	if want := len(workflows) + len(groups); len(got) != want {
		t.Errorf("%d skills installed, want %d", len(got), want)
	}
	for _, w := range workflows {
		if _, ok := got[w]; !ok {
			t.Errorf("workflow %s is not installed", w)
		}
	}
	for _, g := range groups {
		s, ok := got[g.Name]
		if !ok {
			t.Fatalf("group %s is not installed", g.Name)
		}
		for _, p := range g.Profiles {
			if !strings.Contains(s.Body, "## "+p+"\n") || !strings.Contains(s.Body, RunCommand(p)+" --request request.json") {
				t.Errorf("the %s skill does not run %s", g.Name, p)
			}
			if _, own := got[p]; own {
				t.Errorf("%s is both in the %s group and a skill of its own", p, g.Name)
			}
		}
	}
	for _, step := range []string{"plan-write", "plan-candidates", "goals-plan", "sprint-end-facts", "release-gate", "doctor"} {
		if _, own := got[step]; own {
			t.Errorf("the step %s is installed as a skill", step)
		}
	}
	all, err := ProfileSkills(scrummaster.Plugin, scrummaster.AgentsDir)
	if err != nil || len(all) <= len(got) {
		t.Errorf("every profile still renders: %d, %v", len(all), err)
	}
}

// A profile no installed skill reaches, as a workflow, a group's member,
// or a workflow's step, is refused (GH-2749).
func TestARosterRefusesAStrandedProfile(t *testing.T) {
	m := fstest.MapFS{}
	err := fs.WalkDir(scrummaster.Plugin, scrummaster.AgentsDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(scrummaster.Plugin, p)
		m[p] = &fstest.MapFile{Data: b}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	orphan := scrummaster.AgentsDir + "/orphan/"
	m[orphan+"profile.yaml"] = &fstest.MapFile{Data: []byte("name: orphan\nmachine: machine.yaml\n")}
	m[orphan+"machine.yaml"] = &fstest.MapFile{Data: []byte("name: orphan\npurpose: An orphan.\ninitial_state: Start\nstates:\n- name: Start\n  meaning: begins\n- name: Work\n  meaning: works\n- name: Done\n  meaning: ends\nterminal_states: [Done]\n")}
	rendered, err := ProfileSkills(m, scrummaster.AgentsDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Roster(m, scrummaster.AgentsDir, rendered); err == nil || !strings.Contains(err.Error(), "orphan") {
		t.Fatalf("Roster = %v, want the orphan refused", err)
	}
}

// A repository that installed one skill per profile loses, on install of
// the roster, the skills no longer shipped, the steps among them (GH-2749).
func TestInstallingTheRosterRemovesTheStepSkills(t *testing.T) {
	dir := repoDir(t)
	all, err := ProfileSkills(scrummaster.Plugin, scrummaster.AgentsDir)
	if err != nil {
		t.Fatal(err)
	}
	install(t, dir, []string{"claude"}, all, "v0", false)
	roster, err := EmbeddedSkills()
	if err != nil {
		t.Fatal(err)
	}
	got := install(t, dir, []string{"claude"}, roster, "v1", false)
	if got[".claude/skills/plan-write/SKILL.md"] != "removed" || got[".claude/skills/issue-file/SKILL.md"] != "removed" {
		t.Errorf("the step skills were not removed: %v", got)
	}
	if got[".claude/skills/issue/SKILL.md"] != "created" {
		t.Errorf("the issue group was not installed: %v", got[".claude/skills/issue/SKILL.md"])
	}
}
