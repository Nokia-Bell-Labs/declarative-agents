// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"cmp"
	"errors"
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
// order, the open issues implementing them or naming the epic directly that
// can be worked now, and those an open blocker holds back.
type ListResult struct {
	Project string   `json:"project"`
	Stories []string `json:"stories"`
	Issues  []Listed `json:"issues"`
	Blocked []Listed `json:"blocked"`
}

// ListImplementing lists project's open issues whose Implements line names
// story, or, with epic, the epic itself or any story under it in the
// hierarchy at dir (srd007 R6.5, srd036): in the stories' rank order, then by
// IID, so implement works a story's issues in the order they were meant, with
// the issues naming the epic directly after them. An issue a Blocked-by line
// holds behind an open issue is listed apart.
func ListImplementing(t tracker.Tracker, project, dir, story, epic string) (ListResult, error) {
	r := ListResult{Project: project, Stories: []string{}, Issues: []Listed{}, Blocked: []Listed{}}
	switch {
	case epic != "":
		docs, err := hierarchy.Load(dir)
		if err != nil {
			return r, err
		}
		r.Stories = storiesUnder(docs, epic)
		held := slices.ContainsFunc(docs, func(d *hierarchy.Doc) bool { return d.ID == epic })
		if len(r.Stories) == 0 && !held {
			return r, fmt.Errorf("%s holds no epic %s and no story has it among its ancestors", dir, epic)
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
		if target := implementsOf(i); !slices.Contains(r.Stories, target) && (epic == "" || target != epic) {
			continue
		}
		it, err := listing(t, project, i)
		if err != nil {
			return r, err
		}
		listed = append(listed, it)
	}
	// An issue naming the epic directly has no story's rank: it follows them.
	rank := func(target string) int {
		if n := slices.Index(r.Stories, target); n >= 0 {
			return n
		}
		return len(r.Stories)
	}
	slices.SortStableFunc(listed, func(a, b Listed) int {
		if ra, rb := rank(a.Story), rank(b.Story); ra != rb {
			return ra - rb
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

// ListAssigned lists the caller's open issues in projects, by project then
// IID, whoever assigned them and whether or not they have started (srd013). A
// tracking issue is left out: its assignees are following it, and it holds no work.
func ListAssigned(t tracker.Tracker, projects []string) (ListResult, error) {
	r := ListResult{Stories: []string{}, Issues: []Listed{}, Blocked: []Listed{}}
	if len(projects) == 1 {
		r.Project = projects[0]
	}
	seen := map[string]bool{}
	for _, project := range projects {
		if seen[project] {
			continue
		}
		seen[project] = true
		me, err := tracker.UserIn(t, project)
		if err != nil {
			return r, err
		}
		open, err := t.ListIssues(project, tracker.IssueQuery{State: "opened"})
		if err != nil {
			return r, fmt.Errorf("%s: %w", project, err)
		}
		slices.SortStableFunc(open, func(a, b tracker.Issue) int { return a.IID - b.IID })
		for _, i := range open {
			if !slices.Contains(i.Assignees, me.Username) || i.HasLabel("type::tracking") {
				continue
			}
			it, err := listing(t, project, i)
			if err != nil {
				return r, err
			}
			if len(it.BlockedBy) > 0 {
				r.Blocked = append(r.Blocked, it)
			} else {
				r.Issues = append(r.Issues, it)
			}
		}
	}
	return r, nil
}

// programProjects is the system repository and every repository its
// subsystem SRDs declare.
func programProjects(t tracker.Tracker, system, ref string) ([]string, error) {
	subs, err := LoadSubsystems(t, system, ref)
	if err != nil && !errors.Is(err, tracker.ErrNotFound) {
		return nil, err
	}
	return append([]string{ProjectPath(system)}, subs.Repositories()...), nil
}

// implementsOf is the document id i's Implements line names, if any.
func implementsOf(i tracker.Issue) string {
	var target string
	for _, l := range links.ParseLinks(i.Description) {
		if l.Kind == links.Implements {
			target = docID(l.Target)
		}
	}
	return target
}

// listing is i as a list shows it, with the open issues its Blocked-by lines
// name.
func listing(t tracker.Tracker, project string, i tracker.Issue) (Listed, error) {
	it := Listed{Ref: fmt.Sprintf("%s#%d", project, i.IID), IID: i.IID, Title: i.Title, State: i.State, Assignees: i.Assignees, Story: implementsOf(i)}
	for _, l := range links.ParseLinks(i.Description) {
		if l.Kind != links.BlockedBy {
			continue
		}
		open, err := openIssue(t, project, l.Target)
		if err != nil {
			return it, err
		}
		if open {
			it.BlockedBy = append(it.BlockedBy, l.Target)
		}
	}
	return it, nil
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
