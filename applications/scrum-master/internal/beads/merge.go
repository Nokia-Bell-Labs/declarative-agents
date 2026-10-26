// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"fmt"
	"slices"
	"strings"
)

// mergeIssues is the eng04 merge of two clones' tracker files over their
// merge base. theirs is the tracker repository's side, so its writes were
// pushed first. Issues only one side changed merge trivially; an issue both
// changed merges field-wise by mergeIssue.
func mergeIssues(rows []Row, base, ours, theirs []Issue, now string) []Issue {
	index := func(issues []Issue) map[string]Issue {
		m := make(map[string]Issue, len(issues))
		for _, i := range issues {
			m[i.ID] = i
		}
		return m
	}
	b, o, t := index(base), index(ours), index(theirs)
	var ids []string
	for id := range o {
		ids = append(ids, id)
	}
	for id := range t {
		if _, ok := o[id]; !ok {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)

	var merged []Issue
	for _, id := range ids {
		oi, inOurs := o[id]
		ti, inTheirs := t[id]
		switch {
		case !inOurs:
			merged = append(merged, ti)
		case !inTheirs:
			merged = append(merged, oi)
		default:
			merged = append(merged, mergeIssue(rows, b[id], oi, ti, now))
		}
	}
	renumber(merged, func(id string) bool {
		_, inTheirs := t[id]
		_, inBase := b[id]
		return !inTheirs && !inBase
	})
	return merged
}

// renumber gives every issue ours created that collides with another
// issue's IID in the same repository the next free IID there: two clones
// that create between pulls both pick the next number, and the earlier push
// keeps it.
func renumber(issues []Issue, oursNew func(id string) bool) {
	type key struct {
		repo string
		iid  int
	}
	taken := map[key]bool{}
	top := map[string]int{}
	for _, i := range issues {
		if i.IID == 0 {
			continue
		}
		if !oursNew(i.ID) {
			taken[key{i.Repository, i.IID}] = true
		}
		top[i.Repository] = max(top[i.Repository], i.IID)
	}
	for n := range issues {
		i := &issues[n]
		if i.IID == 0 || !oursNew(i.ID) {
			continue
		}
		if taken[key{i.Repository, i.IID}] {
			top[i.Repository]++
			i.IID = top[i.Repository]
		}
		taken[key{i.Repository, i.IID}] = true
	}
}

// mergeIssue merges one issue both sides changed. Plain fields: last writer
// wins. Assignee: the earlier push keeps it. State: ours replays onto theirs
// through the srd041 table, and a replay the table refuses is dropped with
// the refusal recorded as a comment. Labels merge as sets; comments are
// append-only and union.
func mergeIssue(rows []Row, b, o, t Issue, now string) Issue {
	oursLater := o.UpdatedAt > t.UpdatedAt
	m := t
	m.Title = lww(b.Title, o.Title, t.Title, oursLater)
	m.Description = lww(b.Description, o.Description, t.Description, oursLater)
	m.Priority = lww(b.Priority, o.Priority, t.Priority, oursLater)
	m.IssueType = lww(b.IssueType, o.IssueType, t.IssueType, oursLater)
	m.Repository = lww(b.Repository, o.Repository, t.Repository, oursLater)
	m.Release = lww(b.Release, o.Release, t.Release, oursLater)
	m.DueDate = lww(b.DueDate, o.DueDate, t.DueDate, oursLater)
	m.UpdatedAt = max(o.UpdatedAt, t.UpdatedAt)

	if o.Assignee != b.Assignee && t.Assignee == b.Assignee {
		m.Assignee = o.Assignee
	}

	oursMoved := stateOf(o) != stateOf(b)
	theirsMoved := stateOf(t) != stateOf(b)
	if oursMoved && !theirsMoved {
		setState(&m, o)
	}
	if oursMoved && theirsMoved && o.State != t.State {
		if _, ok := row(rows, m, o.StateActor, o.State); ok {
			setState(&m, o)
		} else {
			m.Comments = append(m.Comments, Comment{
				ID: newID(), IssueID: m.ID, Author: "scrum-master", CreatedAt: now,
				Text: fmt.Sprintf("Refused on merge: %s to %s by %s is not in the srd041 table; the tracker keeps %s.",
					t.State, o.State, o.StateActor, t.State),
			})
		}
	}

	m.Labels = mergeSet(b.Labels, o.Labels, t.Labels)
	m.Dependencies = mergeDependencies(b.Dependencies, o.Dependencies, t.Dependencies)
	m.Comments = mergeComments(o.Comments, m.Comments)
	return m
}

func lww[T comparable](b, o, t T, oursLater bool) T {
	switch {
	case o == b, o == t:
		return t
	case t == b, oursLater:
		return o
	}
	return t
}

type state struct{ state, actor, status, closedAt, reason string }

func stateOf(i Issue) state {
	return state{i.State, i.StateActor, i.Status, i.ClosedAt, i.CloseReason}
}

func setState(m *Issue, from Issue) {
	m.State, m.StateActor, m.Status = from.State, from.StateActor, from.Status
	m.ClosedAt, m.CloseReason = from.ClosedAt, from.CloseReason
}

// mergeSet keeps what theirs holds, adds what ours added, and drops what
// ours removed.
func mergeSet(b, o, t []string) []string {
	out := slices.Clone(t)
	for _, l := range o {
		if !slices.Contains(b, l) && !slices.Contains(out, l) {
			out = append(out, l)
		}
	}
	return slices.DeleteFunc(out, func(l string) bool {
		return slices.Contains(b, l) && !slices.Contains(o, l)
	})
}

func mergeComments(o, t []Comment) []Comment {
	out := slices.Clone(t)
	for _, c := range o {
		if !slices.ContainsFunc(out, func(x Comment) bool { return x.ID == c.ID }) {
			out = append(out, c)
		}
	}
	slices.SortStableFunc(out, func(a, b Comment) int {
		if a.CreatedAt != b.CreatedAt {
			return strings.Compare(a.CreatedAt, b.CreatedAt)
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out
}
