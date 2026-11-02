// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package people is who works on which scrum and epic: the system
// repository's control/people.md (config format people). It names people and
// assigns nothing: no issue, status report, or label follows from it (srd045
// R2.5).
package people

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
)

// ControlFile is the people document in the system repository; the technical
// program manager owns it.
const ControlFile = "control/people.md"

// Names is a table cell of comma-separated names, a config document's table
// holding scalars.
type Names []string

// UnmarshalYAML reads the cell.
func (n *Names) UnmarshalYAML(v *yaml.Node) error {
	*n = nil
	for _, name := range strings.Split(v.Value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			*n = append(*n, name)
		}
	}
	return nil
}

// MarshalYAML writes the cell.
func (n Names) MarshalYAML() (any, error) { return strings.Join(n, ", "), nil }

// Person is one person and what they are on: scrums as the timeline facts
// name teams, epics as they name documents (<name>/<id>, the system's by id
// alone).
type Person struct {
	Username string `yaml:"username" json:"username"`
	Scrums   Names  `yaml:"scrums,omitempty" json:"scrums,omitempty"`
	Epics    Names  `yaml:"epics,omitempty" json:"epics,omitempty"`
}

// Config is control/people.md.
type Config struct {
	People []Person `yaml:"people" json:"people"`
}

// Parse reads control/people.md content, refusing a person with no username,
// one no site page can be named for, and a person listed twice.
func Parse(b []byte) (Config, error) {
	var c Config
	if err := corpus.DecodeConfig(b, &c); err != nil {
		return c, err
	}
	seen := map[string]bool{}
	for _, p := range c.People {
		if p.Username == "" || strings.ContainsAny(p.Username, `/\`) {
			return c, fmt.Errorf("%s: %q is not a username", ControlFile, p.Username)
		}
		if seen[p.Username] {
			return c, fmt.Errorf("%s: %s is listed twice", ControlFile, p.Username)
		}
		seen[p.Username] = true
	}
	return c, nil
}

// Load reads the people document from the system checkout dir; a program
// that keeps none has no people recorded.
func Load(dir string) (Config, error) {
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(ControlFile)))
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	return Parse(b)
}

// Document is c as a config document.
func Document(c Config) ([]byte, error) {
	return corpus.FormatConfig("people", "People", c)
}

// Change is one person's allocation or release, in the names given.
type Change struct {
	Username string
	Scrums   []string
	Epics    []string
	Release  bool
}

// Result is what a change did.
type Result struct {
	Path   string `json:"path"`
	Person Person `json:"person"`
	Action string `json:"action"`
	Commit string `json:"commit,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
}

// Apply makes the change in c: people and their names stay sorted, so a
// document reads the same however it was built, and a person left on nothing
// leaves the document.
func Apply(c Config, ch Change) (Config, Person) {
	out := Config{People: slices.Clone(c.People)}
	i := slices.IndexFunc(out.People, func(p Person) bool { return p.Username == ch.Username })
	var p Person
	if i >= 0 {
		p = out.People[i]
		out.People = slices.Delete(out.People, i, i+1)
	}
	p.Username = ch.Username
	p.Scrums, p.Epics = changed(p.Scrums, ch.Scrums, ch.Release), changed(p.Epics, ch.Epics, ch.Release)
	if len(p.Scrums)+len(p.Epics) > 0 {
		out.People = append(out.People, p)
	}
	slices.SortFunc(out.People, func(a, b Person) int { return strings.Compare(a.Username, b.Username) })
	return out, p
}

func changed(have Names, names []string, release bool) Names {
	out := slices.Clone(have)
	if release {
		out = slices.DeleteFunc(out, func(n string) bool { return slices.Contains(names, n) })
	} else {
		out = append(out, names...)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Run applies the change to the people document in the system checkout dir:
// a second identical run changes nothing, a dry run writes nothing, and commit
// records a change on the current branch.
func Run(dir string, ch Change, commit, dryRun bool) (Result, error) {
	r := Result{Path: ControlFile, DryRun: dryRun, Action: "allocated"}
	if ch.Release {
		r.Action = "released"
	}
	c, err := Load(dir)
	if err != nil {
		return r, err
	}
	next, p := Apply(c, ch)
	r.Person = p
	before, err := Document(c)
	if err != nil {
		return r, err
	}
	doc, err := Document(next)
	if err != nil {
		return r, err
	}
	if bytes.Equal(before, doc) {
		r.Action = "unchanged"
		return r, nil
	}
	if dryRun {
		return r, nil
	}
	file := filepath.Join(dir, filepath.FromSlash(ControlFile))
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return r, err
	}
	if err := os.WriteFile(file, doc, 0o644); err != nil {
		return r, err
	}
	if !commit {
		return r, nil
	}
	if out, err := exec.Command("git", "-C", dir, "add", "--", filepath.FromSlash(ControlFile)).CombinedOutput(); err != nil {
		return r, fmt.Errorf("add %s: %v: %s", ControlFile, err, bytes.TrimSpace(out))
	}
	msg := fmt.Sprintf("%s: %s %s (scrum-master people)", ControlFile, r.Action, ch.Username)
	if out, err := exec.Command("git", "-C", dir, "commit", "-q", "-m", msg).CombinedOutput(); err != nil {
		return r, fmt.Errorf("commit %s: %v: %s", ControlFile, err, bytes.TrimSpace(out))
	}
	sha, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
	if err != nil {
		return r, err
	}
	r.Commit = strings.TrimSpace(string(sha))
	return r, nil
}
