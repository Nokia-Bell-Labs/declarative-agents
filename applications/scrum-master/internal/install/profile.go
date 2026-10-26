// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package install

import (
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master"
)

// EmbeddedSkills is the full roster the binary ships: the embedded SKILL.md
// skills and the profile-rendered ones, one source of truth per name.
func EmbeddedSkills() ([]Skill, error) {
	embedded, err := Skills(scrummaster.Plugin, scrummaster.SkillsDir)
	if err != nil {
		return nil, err
	}
	rendered, err := ProfileSkills(scrummaster.Plugin, scrummaster.AgentsDir)
	if err != nil {
		return nil, err
	}
	return MergeSkills(embedded, rendered)
}

// A profile-rendered skill (GH-135): a skill whose workflow is a
// declarative-agents profile under plugin/agents/<name>/. The profile's
// machine is the source of truth — its states are the skill's steps — and
// the SKILL.md an agent receives is rendered from it, so an agent that does
// not run the runtime follows the same steps the runtime executes.

// profileDoc is the part of a profile.yaml rendering reads.
type profileDoc struct {
	Name             string            `yaml:"name"`
	Machine          string            `yaml:"machine"`
	Libraries        map[string]string `yaml:"libraries"`
	ToolDeclarations []string          `yaml:"tool_declarations"`
}

// declarationsDoc is the part of a tool declarations file rendering reads.
type declarationsDoc struct {
	Tools []struct {
		Name   string   `yaml:"name"`
		Type   string   `yaml:"type"`
		Binary string   `yaml:"binary"`
		Args   []string `yaml:"args"`
	} `yaml:"tools"`
}

// wordAliases names the profile that runs a model profile's word where the
// profile is not the word's name with dashes (GH-48).
var wordAliases = map[string]string{"issue_pop": "pop", "issue_merge": "merge"}

// modelFreeSteps is, for a model profile, how a coding agent runs each of
// its exec words without a model (GH-48): the model-free profile named for
// the word, or the word's own command line when it is not a scrum-master
// word.
func modelFreeSteps(fsys fs.FS, dir string, prof profileDoc, names map[string]bool) ([]string, error) {
	var out []string
	for _, f := range prof.ToolDeclarations {
		if path.IsAbs(f) || strings.HasPrefix(f, "..") {
			continue // agent-core's own tools, which no word is
		}
		b, err := fs.ReadFile(fsys, path.Join(dir, prof.Name, f))
		if err != nil {
			return nil, err
		}
		var d declarationsDoc
		if err := yaml.Unmarshal(b, &d); err != nil {
			return nil, fmt.Errorf("%s: %w", f, err)
		}
		for _, t := range d.Tools {
			if t.Type != "exec" {
				continue
			}
			profile, ok := wordAliases[t.Name]
			if !ok {
				profile = strings.ReplaceAll(t.Name, "_", "-")
			}
			switch {
			case names[profile]:
				out = append(out, fmt.Sprintf("- %s: `%s --request request.json`", t.Name, RunCommand(profile)))
			case t.Binary != "scrum-master":
				out = append(out, fmt.Sprintf("- %s: `%s`", t.Name, strings.Join(append([]string{t.Binary}, t.Args...), " ")))
			default:
				return nil, fmt.Errorf("%s's word %s has no model-free profile %s", prof.Name, t.Name, profile)
			}
		}
	}
	return out, nil
}

// machineDoc is the part of a machine definition rendering reads.
type machineDoc struct {
	Name           string         `yaml:"name"`
	Purpose        string         `yaml:"purpose"`
	InitialState   string         `yaml:"initial_state"`
	States         []machineState `yaml:"states"`
	TerminalStates []string       `yaml:"terminal_states"`
}

type machineState struct {
	Name    string `yaml:"name"`
	Meaning string `yaml:"meaning"`
}

// ProfileSkills reads every declarative-agents profile of an agents
// directory in fsys, <dir>/<name>/profile.yaml with its machine, and renders
// each as the Skill its agents receive. A profile whose name is not its
// directory, or whose machine lacks a purpose or states, is refused.
func ProfileSkills(fsys fs.FS, dir string) ([]Skill, error) {
	paths, err := fs.Glob(fsys, dir+"/*/profile.yaml")
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	env, err := ProfileEnv(fsys, dir)
	if err != nil {
		return nil, err
	}
	names := map[string]bool{}
	for _, p := range paths {
		names[path.Base(path.Dir(p))] = true
	}
	var out []Skill
	for _, p := range paths {
		pb, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, err
		}
		var prof profileDoc
		if err := yaml.Unmarshal(pb, &prof); err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if base := path.Base(path.Dir(p)); prof.Name != base {
			return nil, fmt.Errorf("%s: name %q is not its directory %q", p, prof.Name, base)
		}
		mb, err := fs.ReadFile(fsys, path.Join(path.Dir(p), prof.Machine))
		if err != nil {
			return nil, fmt.Errorf("%s: machine: %w", p, err)
		}
		var m machineDoc
		if err := yaml.Unmarshal(mb, &m); err != nil {
			return nil, fmt.Errorf("%s: machine: %w", p, err)
		}
		var steps []string
		if prof.Libraries["providers"] != "" {
			if steps, err = modelFreeSteps(fsys, dir, prof, names); err != nil {
				return nil, fmt.Errorf("%s: %w", p, err)
			}
		}
		s, err := renderProfileSkill(prof.Name, m, env[prof.Name], prof.Libraries["providers"] != "", steps)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, s)
	}
	return out, nil
}

// renderProfileSkill is the Skill a machine renders to: the purpose's first
// paragraph is the description, the rest the introduction, then how to run
// the vendored profile with the agent and the environment it reads (GH-29),
// and the states between the initial and the terminal ones are the
// numbered steps, in the machine's order, each its meaning. The terminal
// states close the skill, so a reader sees how a run ends, the refusal
// state among them. A machine with a model keeps the steps for an agent to
// follow when no model is configured.
func renderProfileSkill(name string, m machineDoc, env []string, model bool, steps []string) (Skill, error) {
	paragraphs := strings.Split(strings.TrimSpace(m.Purpose), "\n\n")
	if paragraphs[0] == "" {
		return Skill{}, fmt.Errorf("machine %q has no purpose to render from", m.Name)
	}
	if len(m.States) == 0 {
		return Skill{}, fmt.Errorf("machine %q has no states to render from", m.Name)
	}
	description := strings.Join(strings.Fields(paragraphs[0]), " ")
	terminal := map[string]bool{}
	for _, t := range m.TerminalStates {
		terminal[t] = true
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n", name)
	for _, p := range paragraphs[1:] {
		b.WriteString("\n" + p + "\n")
	}
	b.WriteString(runSection(name, env, model, steps))
	fmt.Fprintf(&b, "\nThe steps are the machine states of %s/%s/profile.yaml, in order;\nthe runtime executes the same machine (design.md D14).\n\n", AgentsVendored, name)
	n := 0
	for _, st := range m.States {
		if st.Name == m.InitialState || terminal[st.Name] {
			continue
		}
		n++
		fmt.Fprintf(&b, "%d. **%s** — %s\n", n, st.Name, indent(st.Meaning))
	}
	if n == 0 {
		return Skill{}, fmt.Errorf("machine %q has no steps between its initial and terminal states", m.Name)
	}
	b.WriteString("\nA run ends in a terminal state:\n\n")
	for _, st := range m.States {
		if terminal[st.Name] {
			fmt.Fprintf(&b, "- **%s** — %s\n", st.Name, indent(st.Meaning))
		}
	}
	head, err := yaml.Marshal(struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}{name, description})
	if err != nil {
		return Skill{}, err
	}
	return Skill{Name: name, Description: description, Head: strings.TrimSuffix(string(head), "\n"), Body: b.String()}, nil
}

// ProfileEnv is the environment a profile names: every ${VAR} its directory's
// YAML files read, per profile, sorted. scrum-master doctor reports which are set, so
// a run's credentials are checked before a run (srd043 R4.7).
func ProfileEnv(fsys fs.FS, dir string) (map[string][]string, error) {
	paths, err := fs.Glob(fsys, dir+"/*/profile.yaml")
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, p := range paths {
		name := path.Base(path.Dir(p))
		seen := map[string]bool{}
		err := fs.WalkDir(fsys, path.Dir(p), func(fp string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(fp, ".yaml") {
				return err
			}
			b, err := fs.ReadFile(fsys, fp)
			if err != nil {
				return err
			}
			for _, line := range strings.Split(string(b), "\n") {
				// A comment names a variable without reading it, as the
				// declarations' own "never a ${VAR} arg" note does (GH-20).
				if strings.HasPrefix(strings.TrimSpace(line), "#") {
					continue
				}
				for _, m := range envPattern.FindAllStringSubmatch(line, -1) {
					seen[m[1]] = true
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		vars := make([]string, 0, len(seen))
		for v := range seen {
			vars = append(vars, v)
		}
		sort.Strings(vars)
		out[name] = vars
	}
	return out, nil
}

// envPattern is a ${VAR} or ${VAR:-default} reference in a profile's YAML.
var envPattern = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)(?::-[^}]*)?\}`)

// indent joins a meaning into one list item: the first line bare, each
// further line indented under it.
func indent(meaning string) string {
	lines := strings.Split(strings.TrimSpace(meaning), "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = "   " + lines[i]
	}
	return strings.Join(lines, "\n")
}

// MergeSkills joins the embedded skills and the profile-rendered ones into
// one roster; a name in both is refused, so a skill has one source of truth.
func MergeSkills(embedded, rendered []Skill) ([]Skill, error) {
	names := map[string]bool{}
	for _, s := range embedded {
		names[s.Name] = true
	}
	out := append([]Skill{}, embedded...)
	for _, s := range rendered {
		if names[s.Name] {
			return nil, fmt.Errorf("skill %q is both an embedded SKILL.md and a profile; keep one", s.Name)
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// RunCommand is the command line a skill runs its vendored profile with,
// from the repository's root.
func RunCommand(name string) string {
	return "agent --profile " + AgentsVendored + "/" + name + "/profile.yaml --core-root " + CoreVendored + " --directory ."
}

// runSection tells an agent how to run the workflow: with the agent, over
// the profile the repository vendored, and the environment it reads.
func runSection(name string, env []string, model bool, steps []string) string {
	var b strings.Builder
	b.WriteString("\n## Run\n\n")
	if model {
		b.WriteString("The agent runs this workflow with a model, from the repository's root:\n\n")
	} else {
		b.WriteString("Run the workflow with the agent, from the repository's root; its\nwords are the machine's states, so do not run them by hand:\n\n")
	}
	fmt.Fprintf(&b, "    %s\n", RunCommand(name))
	if len(env) > 0 {
		names := strings.Join(env, ", ")
		if n := len(env); n > 1 {
			names = strings.Join(env[:n-1], ", ") + " and " + env[n-1]
		}
		fmt.Fprintf(&b, "\nIt reads %s from the environment.\n", names)
	}
	if model {
		b.WriteString("\nWithout a model configured for the agent, do the model's part yourself:\nwrite what it would write, and run each of its words through the\nmodel-free profile named for it, the word's parameters in the request\n(request.json holding {\"parameters\": {...}}):\n\n")
		b.WriteString(strings.Join(steps, "\n") + "\n")
		b.WriteString("\nThe steps below say what each word is for.\n")
	}
	return b.String()
}
