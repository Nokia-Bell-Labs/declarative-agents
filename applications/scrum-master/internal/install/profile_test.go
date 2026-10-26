// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package install

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master"
)

// Every skill renders from its profile: the machine's states between the
// initial and the terminal ones are the numbered steps, in the machine's
// order (GH-135, GH-198; srd037 R1.4, srd036 R6.2).
func TestSkillsRenderFromTheirProfiles(t *testing.T) {
	skills, err := ProfileSkills(scrummaster.Plugin, scrummaster.AgentsDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range skills {
		names = append(names, s.Name)
	}
	want := []string{"admit", "aggregate", "approve", "architecture-status", "backlog-check", "backlog-components", "backlog-guide", "backlog-list", "backlog-template", "check", "close", "comment", "design", "doctor", "draft", "goals-plan", "goals-push", "implement", "init", "issue-file", "issue-lint", "issue-list", "labels", "merge", "onboard", "peer-read", "plan-backlog", "plan-candidates", "plan-push", "plan-write", "plan", "pop", "recur", "release-gate", "release-tag", "release", "reports-collect", "reports-init", "reports-path", "reports-previous", "reports-validate", "site", "specify", "sprint-end-facts", "sprint-end-stories", "sprint-end", "status-issues", "status-report", "status-rollup", "status", "tasks", "tracker-export", "tracker-migrate", "tracker-sync", "upgrade"}
	if strings.Join(names, " ") != strings.Join(want, " ") {
		t.Fatalf("profiles rendered %v, want %v", names, want)
	}
	for _, s := range skills {
		if n := len([]rune(s.Description)); n < 1 || n > 1024 || strings.Contains(s.Description, "\n") {
			t.Errorf("%s: description must be one line of 1 to 1024 characters, got %d", s.Name, n)
		}
		mb, err := scrummaster.Plugin.ReadFile(scrummaster.AgentsDir + "/" + s.Name + "/machine.yaml")
		if err != nil {
			t.Fatal(err)
		}
		var m machineDoc
		if err := yaml.Unmarshal(mb, &m); err != nil {
			t.Fatal(err)
		}
		terminal := map[string]bool{}
		for _, name := range m.TerminalStates {
			terminal[name] = true
		}
		var steps []string
		for _, st := range m.States {
			if st.Name != m.InitialState && !terminal[st.Name] {
				steps = append(steps, st.Name)
			}
		}
		for i, name := range steps {
			if want := fmt.Sprintf("%d. **%s** — ", i+1, name); !strings.Contains(s.Body, want) {
				t.Errorf("%s: rendered body misses step %q", s.Name, want)
			}
		}
		if got := regexp.MustCompile(`(?m)^\d+\. \*\*`).FindAllString(s.Body, -1); len(got) != len(steps) {
			t.Errorf("%s: rendered %d steps, machine has %d", s.Name, len(got), len(steps))
		}
		for _, name := range m.TerminalStates {
			if !strings.Contains(s.Body, "**"+name+"**") {
				t.Errorf("%s: rendered body misses terminal state %s", s.Name, name)
			}
		}
		if !strings.Contains(s.Body, "design.md D1") {
			t.Errorf("%s: rendered body misses its design.md citation", s.Name)
		}
	}
}

// The implement skill keeps the workflow's commands in its rendered steps.
func TestImplementRenderKeepsItsCommands(t *testing.T) {
	skills, err := ProfileSkills(scrummaster.Plugin, scrummaster.AgentsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range skills {
		if s.Name != "implement" {
			continue
		}
		for _, cmd := range []string{"scrum-master issue pop", "git push", "scrum-master issue merge", "design.md D10"} {
			if !strings.Contains(s.Body, cmd) {
				t.Errorf("rendered body misses %q", cmd)
			}
		}
		if lines := strings.Count(Render("claude", s, "dev"), "\n"); lines > 70 {
			t.Errorf("rendered SKILL.md has %d lines; keep a profile-rendered skill readable at a glance", lines)
		}
	}
}

func TestMergeSkillsRefusesASkillWithTwoSources(t *testing.T) {
	a := []Skill{{Name: "implement", Description: "embedded"}}
	b := []Skill{{Name: "implement", Description: "rendered"}}
	if _, err := MergeSkills(a, b); err == nil {
		t.Fatal("a name with both an embedded SKILL.md and a profile must be refused")
	}
	merged, err := MergeSkills([]Skill{{Name: "plan", Description: "d"}}, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 2 || merged[0].Name != "implement" || merged[1].Name != "plan" {
		t.Fatalf("merged = %v, want implement then plan", merged)
	}
}

// A model skill names, for each of its words, the model-free profile a
// coding agent runs it with when no model is configured (GH-48).
func TestModelSkillsNameTheirWordsProfiles(t *testing.T) {
	skills, err := ProfileSkills(scrummaster.Plugin, scrummaster.AgentsDir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"specify":    {"backlog-guide", "issue-file", "issue-lint", "backlog-components"},
		"design":     {"backlog-guide", "issue-file", "issue-lint", "backlog-components"},
		"plan":       {"plan-candidates", "plan-write", "plan-push", "plan-backlog"},
		"tasks":      {"backlog-components", "backlog-guide", "goals-plan", "goals-push", "backlog-list", "issue-list", "issue-file"},
		"sprint-end": {"sprint-end-facts", "sprint-end-stories"},
		"draft":      {"backlog-check", "backlog-list", "backlog-template"},
		"implement":  {"pop", "merge"},
	}
	for _, s := range skills {
		for _, profile := range want[s.Name] {
			if !strings.Contains(s.Body, RunCommand(profile)+" --request request.json") {
				t.Errorf("%s's skill does not name the %s profile", s.Name, profile)
			}
		}
	}
}

func TestRenderProfileSkillRefusesAnEmptyMachine(t *testing.T) {
	if _, err := renderProfileSkill("x", machineDoc{Purpose: "", States: []machineState{{Name: "A"}}}, nil, false, nil); err == nil {
		t.Error("a machine without a purpose must be refused")
	}
	if _, err := renderProfileSkill("x", machineDoc{Purpose: "p"}, nil, false, nil); err == nil {
		t.Error("a machine without states must be refused")
	}
	m := machineDoc{Purpose: "p", InitialState: "Idle", States: []machineState{{Name: "Idle"}, {Name: "End"}}, TerminalStates: []string{"End"}}
	if _, err := renderProfileSkill("x", m, nil, false, nil); err == nil {
		t.Error("a machine with no steps between initial and terminal states must be refused")
	}
}

// A rendered skill runs the vendored profile with the agent and names the
// environment it reads; a model-free one says not to run its words by hand,
// a model one keeps its steps for an agent without a model (GH-29).
func TestRenderedSkillsRunTheVendoredProfile(t *testing.T) {
	skills, err := ProfileSkills(scrummaster.Plugin, scrummaster.AgentsDir)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, s := range skills {
		seen[s.Name] = true
		if !strings.Contains(s.Body, "    agent --profile .scrum-master/agents/"+s.Name+"/profile.yaml --core-root .scrum-master/agent-core --directory .\n") {
			t.Errorf("%s: no run line:\n%s", s.Name, s.Body)
		}
		switch s.Name {
		case "pop":
			if !strings.Contains(s.Body, "do not run them by hand") || !strings.Contains(s.Body, "SCRUM_MASTER_ISSUE") {
				t.Errorf("pop's run section:\n%s", s.Body)
			}
		case "plan":
			if !strings.Contains(s.Body, "do the model's part yourself") || !strings.Contains(s.Body, "The steps below say what each word is for") {
				t.Errorf("plan's run section:\n%s", s.Body)
			}
		}
	}
	if !seen["pop"] || !seen["plan"] {
		t.Fatalf("skills = %v", seen)
	}
}

// A comment that names ${VAR} reads nothing: only values count, so the
// doctor reports no VAR for the declarations' own note about it (GH-20).
func TestProfileEnvIgnoresComments(t *testing.T) {
	fsys := fstest.MapFS{
		"agents/x/profile.yaml":      {Data: []byte("name: x\n")},
		"agents/x/declarations.yaml": {Data: []byte("# never a ${VAR} arg\nargs:\n- ${SCRUM_MASTER_PROJECT}\nmodel: ${SCRUM_MASTER_MODEL:-qwen}\n")},
	}
	env, err := ProfileEnv(fsys, "agents")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(env["x"], " "); got != "SCRUM_MASTER_MODEL SCRUM_MASTER_PROJECT" {
		t.Fatalf("variables = %q, want SCRUM_MASTER_MODEL SCRUM_MASTER_PROJECT", got)
	}
}

// No shipped profile reads a variable named VAR.
func TestShippedProfilesReadNoPlaceholder(t *testing.T) {
	env, err := ProfileEnv(scrummaster.Plugin, scrummaster.AgentsDir)
	if err != nil {
		t.Fatal(err)
	}
	for name, vars := range env {
		for _, v := range vars {
			if v == "VAR" {
				t.Errorf("profile %s reads VAR", name)
			}
		}
	}
}
