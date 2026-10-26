// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"cmp"
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Listed is one open issue that implements a story.
type Listed struct {
	Ref       string   `json:"ref"`
	IID       int      `json:"iid"`
	Title     string   `json:"title"`
	State     string   `json:"state"`
	Assignees []string `json:"assignees,omitempty"`
	Story     string   `json:"story"`
	// BlockedBy are the open issues its Blocked-by lines name.
	BlockedBy []string `json:"blocked_by,omitempty"`
}

// ListResult is what scrum-master issue list reports: the stories read, in rank
// order, the open issues implementing them that can be worked now, and
// those an open blocker holds back.
type ListResult struct {
	Project string   `json:"project"`
	Stories []string `json:"stories"`
	Issues  []Listed `json:"issues"`
	Blocked []Listed `json:"blocked"`
}

// ListImplementing lists project's open issues whose Implements line names
// story, or, with epic, any story under that epic in the hierarchy at dir
// (srd007 R6.5, srd036): in the stories' rank order, then by IID, so
// implement works a story's issues in the order they were meant. An issue
// a Blocked-by line holds behind an open issue is listed apart.
func ListImplementing(t tracker.Tracker, project, dir, story, epic string) (ListResult, error) {
	r := ListResult{Project: project, Stories: []string{}, Issues: []Listed{}, Blocked: []Listed{}}
	switch {
	case epic != "":
		docs, err := hierarchy.Load(dir)
		if err != nil {
			return r, err
		}
		r.Stories = storiesUnder(docs, epic)
		if len(r.Stories) == 0 {
			return r, fmt.Errorf("no story in %s has %s among its ancestors", dir, epic)
		}
	case story != "":
		r.Stories = []string{story}
	default:
		return r, fmt.Errorf("name a story or an epic")
	}
	open, err := t.ListIssues(project, tracker.IssueQuery{State: "opened"})
	if err != nil {
		return r, err
	}
	var listed []Listed
	for _, i := range open {
		var target string
		var blockers []string
		for _, l := range links.ParseLinks(i.Description) {
			switch l.Kind {
			case links.Implements:
				target = docID(l.Target)
			case links.BlockedBy:
				blockers = append(blockers, l.Target)
			}
		}
		if !slices.Contains(r.Stories, target) {
			continue
		}
		it := Listed{Ref: fmt.Sprintf("%s#%d", project, i.IID), IID: i.IID, Title: i.Title, State: i.State, Assignees: i.Assignees, Story: target}
		for _, b := range blockers {
			if open, err := openIssue(t, project, b); err != nil {
				return r, err
			} else if open {
				it.BlockedBy = append(it.BlockedBy, b)
			}
		}
		listed = append(listed, it)
	}
	slices.SortStableFunc(listed, func(a, b Listed) int {
		if sa, sb := slices.Index(r.Stories, a.Story), slices.Index(r.Stories, b.Story); sa != sb {
			return sa - sb
		}
		return a.IID - b.IID
	})
	for _, it := range listed {
		if len(it.BlockedBy) > 0 {
			r.Blocked = append(r.Blocked, it)
		} else {
			r.Issues = append(r.Issues, it)
		}
	}
	return r, nil
}

// storiesUnder is every story with epic among its ancestors, by rank then
// id.
func storiesUnder(docs []*hierarchy.Doc, epic string) []string {
	byID := map[string]*hierarchy.Doc{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	under := func(d *hierarchy.Doc) bool {
		for seen := 0; d != nil && seen < len(docs); seen++ {
			p := docID(d.Parent)
			if p == epic {
				return true
			}
			d = byID[p]
		}
		return false
	}
	var stories []*hierarchy.Doc
	for _, d := range docs {
		if d.Kind == hierarchy.Story && under(d) {
			stories = append(stories, d)
		}
	}
	slices.SortStableFunc(stories, func(a, b *hierarchy.Doc) int {
		if c := cmp.Compare(a.RankValue(), b.RankValue()); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	ids := make([]string, len(stories))
	for n, d := range stories {
		ids[n] = d.ID
	}
	return ids
}

// docID is the document id a link or parent names, as an id or a pinned
// URL to the document: .../ep001-x.md#R1 names ep001-x.
func docID(target string) string {
	target, _, _ = strings.Cut(strings.TrimSpace(target), "#")
	return strings.TrimSuffix(path.Base(target), ".md")
}

// openIssue reports whether a Blocked-by target names an open issue: an
// issue reference resolved in the tracker, or #N in project. Any other
// target, such as a story, holds nothing here.
func openIssue(t tracker.Tracker, project, target string) (bool, error) {
	p, n, ok := strings.Cut(target, "#")
	iid, err := strconv.Atoi(n)
	if !ok || err != nil || strings.Contains(p, "://") {
		return false, nil
	}
	if p == "" {
		p = project
	}
	i, err := t.GetIssue(p, iid)
	if err != nil {
		return false, nil
	}
	return i.State != "closed", nil
}
