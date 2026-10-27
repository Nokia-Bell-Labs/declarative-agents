// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// SRDDir holds a repository's SRDs; in the system repository, subsystem
// SRDs among them declare each subsystem's owner and components.
const SRDDir = "docs/specs/software-requirements"

// Component is one component a subsystem SRD declares.
type Component struct {
	Name       string `yaml:"name"`
	Repository string `yaml:"repository"`
	Path       string `yaml:"path"`
}

// Subsystem is one subsystem SRD: its owner and its components
// (design.md subsystem_srd).
type Subsystem struct {
	ID         string      `yaml:"id"`
	Owner      string      `yaml:"owner"`
	Components []Component `yaml:"components"`
	// Scrum is the repository its context's scrum_repository names: the
	// team's planning repository, apart from its code (GH-2738). Empty,
	// the team plans in its first component's repository.
	Scrum string `yaml:"scrum_repository,omitempty"`
}

// ScrumRepository is the team's planning repository as a project path:
// where its features, stories, Sprint Goals, sprint-end summaries, and
// team rollup live. It is the repository the SRD's scrum_repository names,
// else its first component's (GH-2738).
func (sub Subsystem) ScrumRepository() string {
	if sub.Scrum != "" {
		return ProjectPath(sub.Scrum)
	}
	if len(sub.Components) == 0 {
		return ""
	}
	return ProjectPath(sub.Components[0].Repository)
}

// ScrumURL is the scrum repository as the SRD writes it, a URL where it
// gives one.
func (sub Subsystem) ScrumURL() string {
	if sub.Scrum != "" {
		return sub.Scrum
	}
	if len(sub.Components) == 0 {
		return ""
	}
	return sub.Components[0].Repository
}

// PlanningRepositories are the repositories a team's planning documents
// are read from: its scrum repository alone when the SRD names one, else
// every component repository, as before a team could name one.
func (sub Subsystem) PlanningRepositories() []string {
	if sub.Scrum != "" {
		return []string{ProjectPath(sub.Scrum)}
	}
	return sub.Repositories()
}

// Repositories are the team's repositories as project paths: its scrum
// repository first where the SRD names one, then each component's.
func (sub Subsystem) Repositories() []string {
	var out []string
	if sub.Scrum != "" {
		out = append(out, ProjectPath(sub.Scrum))
	}
	for _, c := range sub.Components {
		if p := ProjectPath(c.Repository); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// Subsystems are the subsystem SRDs of a system repository at one ref.
type Subsystems []Subsystem

// LoadSubsystems reads every SRD under SRDDir in the system repository at
// ref and keeps those that declare an owner and
// components.
func LoadSubsystems(t tracker.Forge, system, ref string) (Subsystems, error) {
	files, err := t.ListDir(system, SRDDir, ref)
	if err != nil {
		return nil, err
	}
	var out Subsystems
	for _, p := range corpus.Documents(files) {
		content, err := t.GetFile(system, p, ref)
		if err != nil {
			return nil, err
		}
		f, err := corpus.Decode(p, content)
		if err != nil {
			continue
		}
		if sub := readSubsystem(f); sub.Owner != "" && len(sub.Components) > 0 {
			out = append(out, sub)
		}
	}
	return out, nil
}

// SubsystemsIn reads the subsystem SRDs of the checkout at dir from disk,
// as LoadSubsystems reads them through a tracker; none when it has no SRDs.
func SubsystemsIn(dir string) (Subsystems, error) {
	paths, err := filepath.Glob(filepath.Join(dir, SRDDir, "*.md"))
	if err != nil {
		return nil, err
	}
	var out Subsystems
	for _, p := range paths {
		content, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		f, err := corpus.Decode(filepath.ToSlash(filepath.Join(SRDDir, filepath.Base(p))), content)
		if err != nil {
			continue
		}
		if sub := readSubsystem(f); sub.Owner != "" && len(sub.Components) > 0 {
			out = append(out, sub)
		}
	}
	return out, nil
}

// readSubsystem reads an SRD's owner and components: in Markdown, the owner
// item of its context and the rows of its implementation_targets table
// (design.md subsystem_srd).
func readSubsystem(f corpus.File) Subsystem {
	var sub Subsystem
	sub.ID = f.Markdown.ID()
	sub.Owner = corpus.TreeString(f.Markdown.Field("context").Tree(), "owner")
	sub.Scrum = corpus.TreeString(f.Markdown.Field("context").Tree(), "scrum_repository")
	for _, row := range f.Markdown.Field("implementation_targets").Table() {
		sub.Components = append(sub.Components, Component{Name: row["name"], Repository: row["repository"], Path: row["path"]})
	}
	return sub
}

// ComponentFor returns the component a subsystem SRD declares for a
// repository, matched by project path. A team's scrum repository holds no
// component of its own: it answers for its subsystem's first component,
// whose label its issues carry (GH-2738).
func (s Subsystems) ComponentFor(repository string) (Component, Subsystem, bool) {
	want := ProjectPath(repository)
	for _, sub := range s {
		for _, c := range sub.Components {
			if ProjectPath(c.Repository) == want {
				return c, sub, true
			}
		}
	}
	for _, sub := range s {
		if sub.Scrum != "" && ProjectPath(sub.Scrum) == want && len(sub.Components) > 0 {
			return Component{Name: sub.Components[0].Name, Repository: sub.Scrum, Path: "/"}, sub, true
		}
	}
	return Component{}, Subsystem{}, false
}

// Components lists the name of every component the subsystems declare.
func (s Subsystems) Components() []string {
	var out []string
	for _, sub := range s {
		for _, c := range sub.Components {
			out = append(out, c.Name)
		}
	}
	return out
}

// Repositories lists every repository the subsystems declare: each
// team's scrum repository where its SRD names one, and every component
// repository.
func (s Subsystems) Repositories() []string {
	var out []string
	for _, sub := range s {
		if sub.Scrum != "" {
			out = append(out, ProjectPath(sub.Scrum))
		}
		for _, c := range sub.Components {
			out = append(out, ProjectPath(c.Repository))
		}
	}
	return out
}

// ScrumRepositories lists each subsystem's scrum repository, one per team.
func (s Subsystems) ScrumRepositories() []string {
	var out []string
	for _, sub := range s {
		if r := sub.ScrumRepository(); r != "" && !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out
}

// ProjectPath reduces a repository URL or path to GROUP/REPO: an
// https or ssh URL, or the scp form user@host:group/repo that git's
// url.insteadOf often rewrites an origin to (GH-67).
func ProjectPath(repo string) string { return checkout.ProjectPath(repo) }

// CheckProject refuses a filesystem path where a project path is expected
// (GH-68): a checkout's path names no project, and a word that took one
// would file into a project named after the path. flag names the option.
func CheckProject(flag, value string) error {
	if value == "" {
		return nil
	}
	if filepath.IsAbs(value) || value == "." || value == ".." || strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "~") {
		return fmt.Errorf("--%s %s is a filesystem path; name the project, such as group/system, as the tracker knows it", flag, value)
	}
	return nil
}

// Teams are the program's subsystems that are teams: every subsystem SRD
// but the system SRD, the one whose components live in the system
// repository itself, named by its project path or, for an origin that is a
// local path, by its name. The subsystem SRDs are the program's registry of
// its repositories (GH-2725, GH-2726). A system repository with no SRDs
// has no teams.
func Teams(t tracker.Forge, system, ref string) (Subsystems, error) {
	subs, err := LoadSubsystems(t, system, ref)
	if errors.Is(err, tracker.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return subs.Teams(system), nil
}

// Teams are the subsystems but the system's own SRD.
func (s Subsystems) Teams(system string) Subsystems {
	return slices.DeleteFunc(slices.Clone(s), func(sub Subsystem) bool {
		return slices.ContainsFunc(sub.Components, func(c Component) bool { return IsSystem(c.Repository, system) })
	})
}

// IsSystem reports whether repo names the system repository, by project
// path or, for an origin that is a local path, by name.
func IsSystem(repo, system string) bool {
	p := ProjectPath(repo)
	return p == ProjectPath(system) || path.Base(strings.TrimSuffix(p, ".git")) == path.Base(ProjectPath(system))
}

// SubsystemFor is the team subsystem a repository belongs to: the one
// whose SRD lists it as a component's repository.
func (s Subsystems) SubsystemFor(repository string) (Subsystem, bool) {
	_, sub, ok := s.ComponentFor(repository)
	return sub, ok
}

// componentKey compares component names as labels do, whatever their case
// and whether words are joined by a hyphen or an underscore.
func componentKey(name string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(name)), "_", "-")
}

// Lists reports whether the subsystem's SRD lists a component by name.
func (sub Subsystem) Lists(component string) bool {
	_, ok := sub.Component(component)
	return ok
}

// Component is the component the subsystem's SRD lists by name.
func (sub Subsystem) Component(name string) (Component, bool) {
	i := slices.IndexFunc(sub.Components, func(c Component) bool { return componentKey(c.Name) == componentKey(name) })
	if i < 0 {
		return Component{}, false
	}
	return sub.Components[i], true
}

// Owner is the subsystem whose SRD lists a component by name: the team
// whose area it is (GH-2726).
func (s Subsystems) Owner(component string) (Subsystem, bool) {
	for _, sub := range s {
		if sub.Lists(component) {
			return sub, true
		}
	}
	return Subsystem{}, false
}
