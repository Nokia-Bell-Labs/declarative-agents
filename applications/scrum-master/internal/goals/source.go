// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package goals is the scrum-master side of tasks (srd022, srd034 R3): it
// computes each objective's facts and recommendations, and files the
// confirmed objective issues. Nothing here is a model step (design.md D10).
package goals

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/objectives"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

const (
	roadMapPath = "docs/road-map.md"
)

// Release is the road-map release tasks pushes objectives for. A
// consuming program's road-map names the objective items a release proves
// as objectives: [objNNN-short-name#Rn.m] (srd034 R3.1).
type Release struct {
	Version    string   `yaml:"version"`
	Tag        string   `yaml:"tag"`
	Milestone  string   `yaml:"milestone"`
	Objectives []string `yaml:"objectives"`
}

// Sources are the facts tasks reads from the system repository at
// the release tag.
type Sources struct {
	System     string
	Release    Release
	Documents  map[string]*objectives.Document
	Subsystems program.Subsystems
	// Epics are the system repository's agreed epics at the release tag,
	// whose components route each item to the teams owning them (GH-2726).
	Epics []*hierarchy.Doc
}

// Load reads the release from the road-map on ref, then the objective
// documents and subsystem SRDs at the release tag, so every fact is the
// one the Cites lines will pin.
func Load(t tracker.Tracker, system, version, ref string) (Sources, error) {
	s := Sources{System: system, Documents: map[string]*objectives.Document{}}
	f, err := corpus.Get(t, system, roadMapPath, ref)
	if err != nil {
		return s, fmt.Errorf("%s road map at %s: %w", system, ref, err)
	}
	releases, err := readReleases(f)
	if err != nil {
		return s, fmt.Errorf("%s: %w", f.Path, err)
	}
	rm := struct{ Releases []Release }{releases}
	i := slices.IndexFunc(rm.Releases, func(r Release) bool { return r.Version == version })
	if i < 0 {
		return s, fmt.Errorf("%s names no release %s", f.Path, version)
	}
	s.Release = rm.Releases[i]
	if s.Release.Tag == "" {
		return s, fmt.Errorf("release %s has no tag; tasks pins Cites lines to it", version)
	}
	if len(s.Release.Objectives) == 0 {
		return s, fmt.Errorf("release %s names no objective items", version)
	}
	tag := s.Release.Tag
	for _, id := range s.Release.Objectives {
		doc, _, ok := strings.Cut(id, "#")
		if !ok {
			return s, fmt.Errorf("release objective %q is not objNNN-name#Rn.m", id)
		}
		if _, seen := s.Documents[doc]; seen {
			continue
		}
		p := path.Join(objectives.DirFor(doc), doc+".md")
		f, err := corpus.Get(t, system, p, tag)
		if err != nil {
			return s, fmt.Errorf("%s at %s: %w", p, tag, err)
		}
		d, err := objectives.FromFile(f)
		if err != nil {
			return s, fmt.Errorf("%s at %s: %w", f.Path, tag, err)
		}
		p = f.Path
		if f := objectives.Validate(d); len(f) > 0 {
			return s, fmt.Errorf("%s at %s is not valid: %s", p, tag, f[0])
		}
		if d.Status != "agreed" {
			return s, fmt.Errorf("%s at %s has status %s; only an agreed document is pushed (srd034 R1.3)", p, tag, d.Status)
		}
		s.Documents[doc] = d
	}
	if s.Subsystems, err = program.LoadSubsystems(t, system, tag); err != nil {
		return s, err
	}
	s.Epics, err = loadEpics(t, system, tag)
	return s, err
}

// loadEpics reads the system repository's agreed epics at tag; a
// repository with no epics folder has none.
func loadEpics(t tracker.Tracker, system, tag string) ([]*hierarchy.Doc, error) {
	paths, err := t.ListDir(system, epicDir, tag)
	if errors.Is(err, tracker.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []*hierarchy.Doc
	for _, p := range paths {
		if !strings.HasSuffix(p, ".md") || strings.HasSuffix(p, "/README.md") || path.Base(p) == "README.md" {
			continue
		}
		b, err := t.GetFile(system, p, tag)
		if err != nil {
			return nil, err
		}
		d, err := hierarchy.Parse(p, b)
		if err != nil || d.Kind != hierarchy.Epic || d.Status != "agreed" {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}

// readReleases reads the road map's releases: in Markdown, each release
// record with its objective items in deliverables and its tag and milestone
// in release.publication, the declared fields that hold them (design.md
// roadmap program_release_fields).
func readReleases(f corpus.File) ([]Release, error) {
	return ReleasesOf(f.Markdown), nil
}

// ReleasesOf reads a road map's releases in document order: each release
// record's objective items from deliverables, and its tag and milestone
// from release.publication.
func ReleasesOf(d *corpus.Document) []Release {
	var out []Release
	for _, r := range d.Records("release") {
		publication := r.Field("release").Field("publication").Tree()
		out = append(out, Release{
			Version:    r.Key,
			Tag:        corpus.TreeString(publication, "tag"),
			Milestone:  corpus.TreeString(publication, "milestone"),
			Objectives: r.Field("deliverables").List(),
		})
	}
	return out
}

// ComponentFor returns the component a subsystem SRD declares for a
// repository, matched by project path.
func (s Sources) ComponentFor(repository string) (program.Component, program.Subsystem, bool) {
	return s.Subsystems.ComponentFor(repository)
}

func projectPath(repo string) string { return program.ProjectPath(repo) }

// epicDir is where the system repository keeps its epics.
var epicDir = func() string {
	for _, l := range hierarchy.Levels {
		if l.Kind == hierarchy.Epic {
			return l.Dir
		}
	}
	return ""
}()
