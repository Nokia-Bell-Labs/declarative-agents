// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"slices"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
)

// Dependency is one of bd's dependency records, kept on the dependent
// issue: IssueID depends on DependsOnID, as a blocker (blocks) or as a
// related issue (related). bd imports them as its own edges.
type Dependency struct {
	IssueID     string `json:"issue_id"`
	DependsOnID string `json:"depends_on_id"`
	Type        string `json:"type"`
	CreatedAt   string `json:"created_at,omitempty"`
	CreatedBy   string `json:"created_by,omitempty"`
}

// The dependency types scrum-master writes, bd's names.
const (
	DependencyBlocks  = "blocks"
	DependencyRelated = "related"
)

// linked is the dependencies an issue's link lines name (srd007 R6.1):
// a Blocked-by line makes the issue depend on its target as a blocker, and
// Depends-on and Relates-to lines as a related issue, since neither makes
// an issue wait (R6.3), recorded on the issue;
// a Blocks line makes its target depend on the issue, recorded on the
// target, so blocks returns the targets' IDs. A line whose target is no
// issue in this tracker, such as a document or a pinned URL, names none.
func (t *Tracker) linked(description string) (own []Dependency, blocks []string, err error) {
	for _, l := range links.ParseLinks(description) {
		typ := ""
		switch l.Kind {
		case links.BlockedBy, links.Blocks:
			typ = DependencyBlocks
		case links.DependsOn, links.RelatesTo:
			typ = DependencyRelated
		default:
			continue
		}
		project, n, ok := strings.Cut(l.Target, "#")
		iid, perr := strconv.Atoi(n)
		if !ok || perr != nil || project == "" || strings.Contains(project, "://") {
			continue
		}
		target, found, err := t.Store.Find(project, iid)
		if err != nil {
			return nil, nil, err
		}
		if !found {
			continue
		}
		if l.Kind == links.Blocks {
			if !slices.Contains(blocks, target.ID) {
				blocks = append(blocks, target.ID)
			}
			continue
		}
		own = addDependency(own, Dependency{DependsOnID: target.ID, Type: typ})
	}
	return own, blocks, nil
}

// addDependency adds d unless the list already holds an edge to the same
// issue of the same type.
func addDependency(deps []Dependency, d Dependency) []Dependency {
	if slices.ContainsFunc(deps, func(x Dependency) bool { return x.DependsOnID == d.DependsOnID && x.Type == d.Type }) {
		return deps
	}
	return append(deps, d)
}

// depend adds each of deps to i, stamped as i's and by the store's user.
func (s *Store) depend(i *Issue, deps []Dependency) bool {
	before := len(i.Dependencies)
	for _, d := range deps {
		d.IssueID, d.CreatedAt, d.CreatedBy = i.ID, s.stamp(), s.User
		i.Dependencies = addDependency(i.Dependencies, d)
	}
	return len(i.Dependencies) > before
}

// blockOn records, on each issue in targets, that it depends on id as a
// blocker: what id's Blocks lines say.
func (t *Tracker) blockOn(targets []string, id string) error {
	for _, target := range targets {
		_, _, _, err := t.Store.mutate(target, "link "+target+" blocked by "+id, func(i *Issue) (bool, error) {
			return t.Store.depend(i, []Dependency{{DependsOnID: id, Type: DependencyBlocks}}), nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// mergeDependencies merges two sides' dependencies as a set of edges, as
// labels merge: an edge either side added stays, and one a side removed
// goes.
func mergeDependencies(b, o, t []Dependency) []Dependency {
	key := func(d Dependency) string { return d.DependsOnID + "\x00" + d.Type }
	keys := func(ds []Dependency) []string {
		var out []string
		for _, d := range ds {
			out = append(out, key(d))
		}
		return out
	}
	byKey := map[string]Dependency{}
	for _, ds := range [][]Dependency{t, o, b} {
		for _, d := range ds {
			if _, ok := byKey[key(d)]; !ok {
				byKey[key(d)] = d
			}
		}
	}
	var out []Dependency
	for _, k := range mergeSet(keys(b), keys(o), keys(t)) {
		out = append(out, byKey[k])
	}
	return out
}
