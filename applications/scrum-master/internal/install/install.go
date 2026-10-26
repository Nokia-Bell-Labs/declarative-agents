// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package install writes the embedded Agent Skills where each agent reads
// skills and checks that what is installed matches the binary (srd037).
package install

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill is one embedded SKILL.md: its Agent Skills front matter, kept as
// written in Head, and its body.
type Skill struct {
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Head        string `yaml:"-" json:"-"`
	Body        string `yaml:"-" json:"-"`
}

// Agents are the agents install writes for, in a stable order.
var Agents = []string{"cursor", "claude", "copilot"}

// target is where an agent reads Agent Skills, <skills>/<name>/SKILL.md,
// and where an earlier release wrote command files, which install removes
// (srd037 R2.2, R3.3).
type target struct {
	skills       string
	legacy       string
	legacySuffix string
}

var targets = map[string]target{
	"cursor":  {skills: ".cursor/skills", legacy: ".cursor/commands", legacySuffix: ".md"},
	"claude":  {skills: ".claude/skills", legacy: ".claude/commands", legacySuffix: ".md"},
	"copilot": {skills: ".github/skills", legacy: ".github/prompts", legacySuffix: ".prompt.md"},
}

// Skills reads every skill of an Agent Plugins skills directory in fsys:
// each <dir>/<name>/SKILL.md, one level deep. A skill whose front matter
// names another directory is refused (Agent Skills).
func Skills(fsys fs.FS, dir string) ([]Skill, error) {
	paths, err := fs.Glob(fsys, dir+"/*/SKILL.md")
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	var out []Skill
	for _, p := range paths {
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, err
		}
		s, err := parse(string(b))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		if base := filepath.Base(filepath.Dir(p)); s.Name != base {
			return nil, fmt.Errorf("%s: name %q is not its directory %q", p, s.Name, base)
		}
		out = append(out, s)
	}
	return out, nil
}

func parse(src string) (Skill, error) {
	rest, ok := strings.CutPrefix(src, "---\n")
	if !ok {
		return Skill{}, errors.New("no front matter")
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return Skill{}, errors.New("unterminated front matter")
	}
	var s Skill
	if err := yaml.Unmarshal([]byte(head), &s); err != nil {
		return Skill{}, err
	}
	if s.Description == "" {
		return Skill{}, errors.New("front matter has no description")
	}
	s.Head, s.Body = head, strings.TrimLeft(body, "\n")
	return s, nil
}

// stampPattern reads a skill's stamp, the current one or the tpm stamp an
// earlier release wrote (GH-36).
var stampPattern = regexp.MustCompile(`(?m)^<!-- (?:skill|tpm) install version=(\S+) sha256=([0-9a-f]{64}) -->\n`)

func digest(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// Render is the SKILL.md an agent receives: the skill's front matter, then
// the stamp naming the scrum-master version and the checksum of everything but the
// stamp line, then the body. Front matter must open the file, so the stamp
// follows it. Every agent receives the same text.
func Render(agent string, s Skill, version string) string {
	head := s.Head
	if head == "" {
		b, _ := yaml.Marshal(map[string]string{"name": s.Name, "description": s.Description})
		head = strings.TrimSuffix(string(b), "\n")
	}
	header := "---\n" + head + "\n---\n"
	unstamped := header + s.Body
	stamp := fmt.Sprintf("<!-- skill install version=%s sha256=%s -->\n", version, digest(unstamped))
	return header + stamp + s.Body
}

// stamped reports whether a file carries a tpm stamp and whether its
// content still matches the checksum the stamp records.
func stamped(content string) (ours, edited bool) {
	m := stampPattern.FindStringSubmatchIndex(content)
	if m == nil {
		return false, false
	}
	recorded := content[m[4]:m[5]]
	without := content[:m[0]] + content[m[1]:]
	return true, digest(without) != recorded
}

// Path is where an agent's copy of a skill lives under dir.
func Path(dir, agent, name string) string {
	return filepath.Join(dir, filepath.FromSlash(targets[agent].skills), name, "SKILL.md")
}

// installedFile is a file carrying scrum-master's stamp in an agent's skills
// directory or in its earlier command layout.
type installedFile struct {
	name, path     string
	legacy, edited bool
}

// stampedFiles lists every file scrum-master stamped for an agent under dir. Files
// without the stamp are never listed, so install leaves them alone.
func stampedFiles(dir, agent string) ([]installedFile, error) {
	t := targets[agent]
	var out []installedFile
	add := func(name, path string, legacy bool) error {
		b, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if ours, edited := stamped(string(b)); ours {
			out = append(out, installedFile{name, path, legacy, edited})
		}
		return nil
	}
	skills, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(t.skills)))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range skills {
		if e.IsDir() {
			if err := add(e.Name(), Path(dir, agent, e.Name()), false); err != nil {
				return nil, err
			}
		}
	}
	legacy, err := os.ReadDir(filepath.Join(dir, filepath.FromSlash(t.legacy)))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range legacy {
		name, ok := strings.CutSuffix(e.Name(), t.legacySuffix)
		if !ok || e.IsDir() || (t.legacySuffix == ".md" && strings.HasSuffix(name, ".prompt")) {
			continue
		}
		if err := add(name, filepath.Join(dir, filepath.FromSlash(t.legacy), e.Name()), true); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Result is what happened to one file.
type Result struct {
	Agent  string `json:"agent"`
	Skill  string `json:"skill"`
	Path   string `json:"path"`
	Action string `json:"action"` // created, updated, unchanged, kept, replaced, removed; exclude file: created, updated
	Reason string `json:"reason,omitempty"`
}

// Detect is the --agent value that installs the skills of the agents the
// repository already uses (GH-49): the ones whose marks DetectAgents finds.
const Detect = "detect"

// agentMarks are what shows a repository uses an agent: its own directory
// for cursor and claude; for copilot, which shares .github with CI, its
// instructions or skills there.
var agentMarks = map[string][]string{
	"cursor":  {".cursor"},
	"claude":  {".claude", "CLAUDE.md"},
	"copilot": {".github/copilot-instructions.md", ".github/skills", ".github/prompts"},
}

// DetectAgents is the agents the repository at dir already uses, in
// Agents' order.
func DetectAgents(dir string) []string {
	var out []string
	for _, a := range Agents {
		for _, mark := range agentMarks[a] {
			if _, err := os.Stat(filepath.Join(dir, mark)); err == nil {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// ResolveAgents is the --agent values for the repository at dir: detect
// alone names the agents it already uses and refuses when it uses none;
// any other values expand as ParseAgents does.
func ResolveAgents(values []string, dir string) ([]string, error) {
	if len(values) == 1 && values[0] == Detect {
		if agents := DetectAgents(dir); len(agents) > 0 {
			return agents, nil
		}
		return nil, errors.New("the repository uses no agent scrum-master knows (no .cursor, .claude or CLAUDE.md, or copilot's instructions or skills under .github); name the agents with --agent: cursor, claude, copilot, or all")
	}
	return ParseAgents(values)
}

// ParseAgents expands the --agent values; all names every agent.
func ParseAgents(values []string) ([]string, error) {
	var out []string
	for _, v := range values {
		if v == "all" {
			return Agents, nil
		}
		if _, ok := targets[v]; !ok {
			return nil, fmt.Errorf("unknown agent %q; use cursor, claude, copilot, or all", v)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, errors.New("name an agent with --agent: cursor, claude, copilot, or all")
	}
	return out, nil
}

// Install writes every skill for every agent under dir (srd037 R2, R3). A
// file scrum-master wrote and nobody edited is rewritten when it differs; an edited
// file, or one scrum-master did not write, is kept unless force; a stamped file whose
// skill is no longer embedded is removed only when unedited. Outside the
// agents' skills directories it writes only its block in the clone's
// .git/info/exclude, which lists exactly the files scrum-master owns (R6). A dry run
// writes nothing.
func Install(dir string, agents []string, skills []Skill, version string, force, dryRun bool) ([]Result, error) {
	exclude, err := excludePath(dir)
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, agent := range agents {
		names := map[string]bool{}
		for _, s := range skills {
			names[s.Name] = true
			path := Path(dir, agent, s.Name)
			want := Render(agent, s, version)
			r := Result{Agent: agent, Skill: s.Name, Path: path}
			cur, err := os.ReadFile(path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				r.Action = "created"
			case err != nil:
				return out, err
			case string(cur) == want:
				r.Action = "unchanged"
			default:
				ours, edited := stamped(string(cur))
				switch {
				case ours && !edited:
					r.Action = "updated"
				case force:
					r.Action = "replaced"
				case ours:
					r.Action, r.Reason = "kept", "edited since scrum-master wrote it; kept"
				default:
					r.Action, r.Reason = "kept", "not written by scrum-master; kept"
				}
			}
			if !dryRun && r.Action != "unchanged" && r.Action != "kept" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					return out, err
				}
				if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
					return out, err
				}
			}
			out = append(out, r)
		}
		stale, err := staleFiles(dir, agent, names)
		if err != nil {
			return out, err
		}
		for _, st := range stale {
			r := Result{Agent: agent, Skill: st.name, Path: st.path}
			if st.edited {
				r.Action, r.Reason = "kept", "no longer embedded but edited; remove it by hand"
				if st.legacy {
					r.Reason = "an earlier release's command file, edited since; remove it by hand"
				}
			} else {
				r.Action = "removed"
				if !dryRun {
					if err := os.Remove(st.path); err != nil {
						return out, err
					}
					if !st.legacy {
						os.Remove(filepath.Dir(st.path))
					}
				}
			}
			out = append(out, r)
		}
	}
	ignore, err := updateIgnore(exclude, owned(dir, out), dryRun)
	if err != nil || ignore.Action == "" {
		return out, err
	}
	return append(out, ignore), nil
}

const (
	ignoreBegin = "# skill install: the skill files the init and onboard profiles install"
	ignoreEnd   = "# end skill install"
)

// legacyIgnore is the block an earlier release wrote, which a run finds and
// rewrites under the current markers (GH-37).
var legacyIgnore = [2]string{"# tpm install: skill commands from the local tpm binary", "# end tpm install"}

// owned is every file scrum-master owns once a run's results apply: the stamped
// files already in any agent's directory, plus what the run wrote, less
// what it removed. A dry run reports the block it would write.
func owned(dir string, results []Result) []string {
	set := map[string]bool{}
	for _, agent := range Agents {
		files, _ := stampedFiles(dir, agent)
		for _, f := range files {
			set[f.path] = true
		}
	}
	for _, r := range results {
		switch r.Action {
		case "created", "updated", "unchanged", "replaced":
			set[r.Path] = true
		case "removed":
			delete(set, r.Path)
		}
	}
	var out []string
	for path := range set {
		if rel, err := filepath.Rel(dir, path); err == nil {
			out = append(out, "/"+filepath.ToSlash(rel))
		}
	}
	sort.Strings(out)
	return out
}

// excludePath is the clone's .git/info/exclude, which git applies and never
// commits. A linked worktree resolves to the common git dir its clone
// shares. A dir that is not a git repository is refused (srd037 R6.1).
func excludePath(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-path", "info/exclude").Output()
	if err != nil {
		return "", fmt.Errorf("%s is not a git repository; scrum-master install writes into a clone", dir)
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	return path, nil
}

// ignoreFile splits the exclude file into the lines before scrum-master's block, the
// block's entries, and the lines after it.
func ignoreFile(path string) (before, entries, after []string, err error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	lines := strings.Split(strings.TrimSuffix(string(b), "\n"), "\n")
	begin, endMarker := slices.Index(lines, ignoreBegin), ignoreEnd
	if begin < 0 {
		begin, endMarker = slices.Index(lines, legacyIgnore[0]), legacyIgnore[1]
	}
	if begin < 0 {
		return lines, nil, nil, nil
	}
	end := slices.Index(lines[begin:], endMarker)
	if end < 0 {
		return lines[:begin], lines[begin+1:], nil, nil
	}
	return lines[:begin], lines[begin+1 : begin+end], lines[begin+end+1:], nil
}

// updateIgnore rewrites scrum-master's block in the exclude file to list exactly the
// files scrum-master owns (srd037 R6). Lines outside the block stay as they are.
func updateIgnore(path string, paths []string, dryRun bool) (Result, error) {
	r := Result{Path: path}
	before, entries, after, err := ignoreFile(path)
	if err != nil {
		return r, err
	}
	_, statErr := os.Stat(path)
	exists := statErr == nil
	if slices.Equal(entries, paths) && (len(paths) > 0 || exists) {
		return r, nil
	}
	lines := slices.Clone(before)
	if len(paths) > 0 {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, ignoreBegin)
		lines = append(lines, paths...)
		lines = append(lines, ignoreEnd)
	}
	lines = append(lines, after...)
	content := strings.Join(lines, "\n")
	if content != "" {
		content += "\n"
	}
	r.Action = "updated"
	if !exists {
		r.Action = "created"
	}
	if dryRun {
		return r, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return r, err
	}
	return r, os.WriteFile(path, []byte(content), 0o644)
}

// staleFiles are the stamped files of an agent whose skill is not embedded,
// and every stamped file of the earlier command layout.
func staleFiles(dir, agent string, embedded map[string]bool) ([]installedFile, error) {
	files, err := stampedFiles(dir, agent)
	if err != nil {
		return nil, err
	}
	var out []installedFile
	for _, f := range files {
		if f.legacy || !embedded[f.name] {
			out = append(out, f)
		}
	}
	return out, nil
}

// Finding is one way the installed skills differ from the binary or are
// not ignored by git.
type Finding struct {
	Agent   string `json:"agent"`
	Path    string `json:"path"`
	Problem string `json:"problem"` // missing, outdated, edited, stale, unignored
}

// Check compares the installed skills with the binary and writes nothing
// (srd037 R4). No findings means the installation is current.
func Check(dir string, agents []string, skills []Skill, version string) ([]Finding, error) {
	exclude, err := excludePath(dir)
	if err != nil {
		return nil, err
	}
	_, entries, _, err := ignoreFile(exclude)
	if err != nil {
		return nil, err
	}
	var out []Finding
	for _, agent := range agents {
		names := map[string]bool{}
		for _, s := range skills {
			names[s.Name] = true
			path := Path(dir, agent, s.Name)
			cur, err := os.ReadFile(path)
			switch {
			case errors.Is(err, fs.ErrNotExist):
				out = append(out, Finding{agent, path, "missing"})
				continue
			case err != nil:
				return out, err
			}
			ours, edited := stamped(string(cur))
			if rel, _ := filepath.Rel(dir, path); ours && !slices.Contains(entries, "/"+filepath.ToSlash(rel)) {
				out = append(out, Finding{agent, path, "unignored"})
			}
			if string(cur) == Render(agent, s, version) {
				continue
			}
			if ours && !edited {
				out = append(out, Finding{agent, path, "outdated"})
			} else {
				out = append(out, Finding{agent, path, "edited"})
			}
		}
		stale, err := staleFiles(dir, agent, names)
		if err != nil {
			return out, err
		}
		for _, st := range stale {
			out = append(out, Finding{agent, st.path, "stale"})
		}
	}
	return out, nil
}

// InstalledAgents are the agents for which dir holds at least one file
// carrying scrum-master's stamp, in either layout.
func InstalledAgents(dir string) []string {
	var out []string
	for _, agent := range Agents {
		if files, _ := stampedFiles(dir, agent); len(files) > 0 {
			out = append(out, agent)
		}
	}
	return out
}
