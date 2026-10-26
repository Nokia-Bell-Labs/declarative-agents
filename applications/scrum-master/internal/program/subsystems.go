// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
}

// Subsystems are the subsystem SRDs of a system repository at one ref.
type Subsystems []Subsystem

// LoadSubsystems reads every SRD under SRDDir in the system repository at
// ref and keeps those that declare an owner and
// components.
func LoadSubsystems(t tracker.Tracker, system, ref string) (Subsystems, error) {
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
	for _, row := range f.Markdown.Field("implementation_targets").Table() {
		sub.Components = append(sub.Components, Component{Name: row["name"], Repository: row["repository"], Path: row["path"]})
	}
	return sub
}

// ComponentFor returns the component a subsystem SRD declares for a
// repository, matched by project path.
func (s Subsystems) ComponentFor(repository string) (Component, Subsystem, bool) {
	want := ProjectPath(repository)
	for _, sub := range s {
		for _, c := range sub.Components {
			if ProjectPath(c.Repository) == want {
				return c, sub, true
			}
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

// Repositories lists every component repository the subsystems declare.
func (s Subsystems) Repositories() []string {
	var out []string
	for _, sub := range s {
		for _, c := range sub.Components {
			out = append(out, ProjectPath(c.Repository))
		}
	}
	return out
}

// ProjectPath reduces a repository URL or path to GROUP/REPO: an
// https or ssh URL, or the scp form user@host:group/repo that git's
// url.insteadOf often rewrites an origin to (GH-67).
func ProjectPath(repo string) string {
	repo = strings.TrimSuffix(strings.TrimSuffix(repo, "/"), ".git")
	if _, rest, ok := strings.Cut(repo, "://"); ok {
		_, repo, _ = strings.Cut(rest, "/")
	} else if host, path, ok := strings.Cut(repo, ":"); ok && !strings.Contains(host, "/") && path != "" {
		repo = path
	}
	if p, _, ok := strings.Cut(repo, "/-/"); ok {
		repo = p
	}
	return strings.Trim(repo, "/")
}

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
