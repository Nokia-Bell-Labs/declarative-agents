// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Classification is the plane and kind scrum-master derives from the paths a
// request names (srd035 R2.2).
type Classification struct {
	Plane  string `json:"plane"`
	Kind   string `json:"kind,omitempty"`
	Reason string `json:"reason"`
}

// planningDirs hold the hierarchy documents, the objective documents read
// as initiatives (srd045, srd034 R1.1), and the sprint-end summaries, which
// close a period (srd038).
var planningDirs = []string{"docs/specs/themes/", "docs/specs/initiatives/", "docs/specs/epics/", "docs/specs/features/", "docs/specs/stories/", "docs/specs/objectives/", "docs/sprint-ends/"}

// planning reports whether a path is a planning document: the road-map or a
// theme, initiative, epic, feature, story, or objective document in any
// repository, and in the system repository an SRD, which is a system or
// subsystem SRD, and docs/ARCHITECTURE.md and docs/VISION.md, which declare
// the subsystems a plan moves (srd001 R1.1, GH-90). A directory, with or
// without its trailing slash, is planning when the documents in it are.
func planning(p string, inSystem bool) bool {
	p = path.Clean(strings.TrimPrefix(p, "./"))
	dir := p + "/"
	switch {
	case p == "docs/road-map.md":
		return true
	case slices.ContainsFunc(planningDirs, func(d string) bool { return strings.HasPrefix(dir, d) }):
		return true
	case inSystem && strings.HasPrefix(dir, SRDDir+"/"):
		return true
	case inSystem && (p == "docs/ARCHITECTURE.md" || p == "docs/VISION.md"):
		return true
	}
	return false
}

// Classify derives the plane and kind from the paths a request names. The
// strongest path decides: a planning document makes a plane::management
// issue, then a control/ path a plane::control issue, then any other docs/
// path a kind::spec data issue (srd001 R33); with none of these, the issue is
// project work of the stated kind, and a missing kind is an error naming it.
// inSystem says the request is for the system repository.
func Classify(paths []string, statedKind string, inSystem bool) (Classification, error) {
	var docs, control, plan []string
	for _, raw := range paths {
		p := path.Clean(strings.TrimPrefix(raw, "./"))
		switch {
		case planning(p, inSystem):
			plan = append(plan, p)
		case p == "control" || strings.HasPrefix(p, "control/"):
			control = append(control, p)
		case p == "docs" || strings.HasPrefix(p, "docs/"):
			docs = append(docs, p)
		}
	}
	switch {
	case len(plan) > 0:
		reason := deciding(plan, "is a planning document", "are planning documents")
		if len(plan) < len(paths) {
			// Say how many of the paths given these are, so a path that
			// is no planning document is not read as counted (GH-100).
			reason += fmt.Sprintf(", %d of the %d paths given", len(plan), len(paths))
		}
		return Classification{Plane: "management", Reason: reason}, nil
	case len(control) > 0:
		return Classification{Plane: "control", Reason: deciding(control, "is a policy file", "are policy files")}, nil
	case len(docs) > 0 && statedKind != "":
		return Classification{Plane: "data", Kind: statedKind, Reason: deciding(docs, "is a requirements or design document", "are requirements or design documents") + ", and the stated kind is the work"}, nil
	case len(docs) > 0:
		return Classification{Plane: "data", Kind: "spec", Reason: deciding(docs, "is a requirements or design document", "are requirements or design documents")}, nil
	case statedKind != "":
		return Classification{Plane: "data", Kind: statedKind, Reason: "project work"}, nil
	}
	return Classification{}, fmt.Errorf("project work needs a kind (one of build, experiment, research, review, consume, handoff, release, defect, request, question)")
}

// Labelling is what file puts on a new issue: the axis values given
// explicitly, and the rest derived from the paths and the subsystem SRDs.
type Labelling struct {
	Plane, Kind, Component string
	Derived                *Classification
}

// Derive fills what the caller did not give: plane and kind from the paths
// (Classify), and the component from the subsystem SRD in the system
// repository that declares project. Explicit values always win.
func Derive(t tracker.Tracker, project, system, ref string, paths []string, given Labelling) (Labelling, error) {
	out := given
	if out.Plane == "" && len(paths) > 0 {
		c, err := Classify(paths, given.Kind, system != "" && ProjectPath(project) == ProjectPath(system))
		if err != nil {
			return out, err
		}
		out.Plane, out.Derived = c.Plane, &c
		if out.Kind == "" {
			out.Kind = c.Kind
		} else if out.Plane != "data" {
			// A stated kind is work: it makes a data issue whatever plane
			// the paths alone would give, as scrum-master's own sprint-end issues
			// are plane::data, kind::release on a planning document.
			out.Plane = "data"
			c.Plane, c.Kind, c.Reason = "data", out.Kind, "a stated kind makes a data issue; "+c.Reason
		}
	}
	if out.Plane == "" {
		out.Plane = "data"
	}
	if out.Component == "" && system != "" {
		subs, err := LoadSubsystems(t, system, ref)
		if err != nil {
			return out, fmt.Errorf("read subsystem SRDs in %s: %w", system, err)
		}
		if c, _, ok := subs.ComponentFor(project); ok {
			out.Component = c.Name
		}
	}
	return out, nil
}

// deciding names the paths that decided a plane: the one path, else the
// first and how many others, so the reason accounts for all of them (#188).
func deciding(paths []string, one, many string) string {
	if len(paths) == 1 {
		return paths[0] + " " + one
	}
	return fmt.Sprintf("%s and %d other %s %s", paths[0], len(paths)-1, map[bool]string{true: "path", false: "paths"}[len(paths) == 2], many)
}
