// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package install

import (
	"fmt"
	"io/fs"
	"path"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The installed roster (GH-2749): every profile ships and is vendored, so
// any of them runs by its path, but a coding agent is given only the skills
// a person asks for by name. Each installed skill's description sits in
// the agent's context on every turn, and fifty near-alike names route a
// request to a step rather than its workflow.

// workflows are the profiles installed as skills of their own: the
// workflows a person starts.
var workflows = []string{
	"init", "onboard", "upgrade", "check",
	"pop", "merge", "implement",
	"draft", "design", "specify", "plan", "tasks",
	"sprint-end", "release", "peer-read",
	"status-report", "status-rollup", "site",
}

// group is one installed skill that gathers single-word profiles a person
// runs now and then, each by its own command line.
type group struct {
	Name, Description, Intro string
	Profiles                 []string
}

var groups = []group{
	{"issue", "File, list, lint, comment on, close, admit, approve, and recur the program's issues, and keep its labels, each with the model-free profile that runs one scrum-master word.",
		"Use these for one issue operation; to work an issue from pop to merge, use the pop and merge skills, and implement for the whole loop.",
		[]string{"issue-file", "issue-list", "issue-lint", "comment", "close", "recur", "admit", "approve", "labels"}},
	{"backlog", "List and check the planning hierarchy, print a document's template or the components ARCHITECTURE declares, route a guide, and render the architecture status, each with the model-free profile that runs one scrum-master word.",
		"Use these to read or check the backlog; to write a document, use the draft skill, and design for the architecture.",
		[]string{"backlog-list", "backlog-check", "backlog-template", "backlog-components", "backlog-guide", "architecture-status"}},
	{"reports", "File the week's status and rollup issues, lay down a reports repository, and collect, aggregate, validate, or read back weekly reports, each with the model-free profile that runs one scrum-master word.",
		"Use these for one reporting operation; to write your weekly report, use status-report, and status-rollup for a team's or the program's rollup.",
		[]string{"status-issues", "reports-init", "reports-collect", "aggregate", "reports-validate", "reports-previous", "reports-path", "status"}},
	{"tracker", "Export, migrate, or sync the program's tracker, each with the model-free profile that runs one scrum-master word.",
		"Use these when moving a program between tracking systems or keeping a bd tracker in step.",
		[]string{"tracker-export", "tracker-migrate", "tracker-sync"}},
}

// Roster is the skills install writes from the profile-rendered ones: the
// workflows, each as rendered, and one skill per group listing its
// profiles. Every shipped profile is reachable from the roster, as a
// workflow, a group's member, or a step a workflow runs; one that is not
// is refused, so no profile ships that no skill names.
func Roster(fsys fs.FS, dir string, rendered []Skill) ([]Skill, error) {
	byName := map[string]Skill{}
	for _, s := range rendered {
		byName[s.Name] = s
	}
	steps, err := stepsOf(fsys, dir, byName)
	if err != nil {
		return nil, err
	}
	reached := map[string]bool{}
	var reach func(string)
	reach = func(name string) {
		if reached[name] {
			return
		}
		reached[name] = true
		for _, s := range steps[name] {
			reach(s)
		}
	}
	var out []Skill
	for _, w := range workflows {
		s, ok := byName[w]
		if !ok {
			return nil, fmt.Errorf("the roster's workflow %s has no profile", w)
		}
		out = append(out, s)
		reach(w)
	}
	env, err := ProfileEnv(fsys, dir)
	if err != nil {
		return nil, err
	}
	for _, g := range groups {
		s, err := renderGroup(g, byName, env)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		for _, p := range g.Profiles {
			reach(p)
		}
	}
	var stranded []string
	for name := range byName {
		if !reached[name] {
			stranded = append(stranded, name)
		}
	}
	if len(stranded) > 0 {
		sort.Strings(stranded)
		return nil, fmt.Errorf("profiles %s are reachable from no installed skill: add each to a workflow, a group, or a workflow's steps", strings.Join(stranded, ", "))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// stepsOf maps each profile to the profiles of the scrum-master words its
// declarations run, as modelFreeSteps names them.
func stepsOf(fsys fs.FS, dir string, names map[string]Skill) (map[string][]string, error) {
	out := map[string][]string{}
	for name := range names {
		pb, err := fs.ReadFile(fsys, path.Join(dir, name, "profile.yaml"))
		if err != nil {
			return nil, err
		}
		var prof profileDoc
		if err := yaml.Unmarshal(pb, &prof); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for _, f := range prof.ToolDeclarations {
			if path.IsAbs(f) || strings.HasPrefix(f, "..") {
				continue
			}
			b, err := fs.ReadFile(fsys, path.Join(dir, name, f))
			if err != nil {
				return nil, err
			}
			var d declarationsDoc
			if err := yaml.Unmarshal(b, &d); err != nil {
				return nil, fmt.Errorf("%s: %w", f, err)
			}
			for _, t := range d.Tools {
				if t.Type != "exec" || t.Binary != "scrum-master" {
					continue
				}
				p, ok := wordAliases[t.Name]
				if !ok {
					p = strings.ReplaceAll(t.Name, "_", "-")
				}
				if _, ok := names[p]; ok && p != name && !slices.Contains(out[name], p) {
					out[name] = append(out[name], p)
				}
			}
		}
	}
	return out, nil
}

// renderGroup is a group's skill: its description, then each member's
// purpose, as its own skill would open, and the command line that runs it.
func renderGroup(g group, byName map[string]Skill, env map[string][]string) (Skill, error) {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n\nEach runs a model-free profile the repository vendored, with the agent, from the\nrepository's root; a word's parameters go in the request (request.json holding\n{\"parameters\": {...}}), and the profile's own skill text is its machine,\n%s/<name>/machine.yaml.\n", g.Name, g.Intro, AgentsVendored)
	for _, p := range g.Profiles {
		s, ok := byName[p]
		if !ok {
			return Skill{}, fmt.Errorf("the %s group's %s has no profile", g.Name, p)
		}
		fmt.Fprintf(&b, "\n## %s\n\n%s\n\n    %s --request request.json\n", p, s.Description, RunCommand(p))
		if vars := env[p]; len(vars) > 0 {
			fmt.Fprintf(&b, "\nIt reads %s from the environment.\n", strings.Join(vars, ", "))
		}
	}
	head, err := yaml.Marshal(struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}{g.Name, g.Description})
	if err != nil {
		return Skill{}, err
	}
	return Skill{Name: g.Name, Description: g.Description, Head: strings.TrimSuffix(string(head), "\n"), Body: b.String()}, nil
}
