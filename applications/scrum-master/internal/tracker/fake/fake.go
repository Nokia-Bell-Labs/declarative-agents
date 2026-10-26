// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package fake is an in-memory Tracker for unit tests.
package fake

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Fake holds projects' files, tags, issues, notes, and milestones in memory.
type Fake struct {
	Files      map[string][]byte // "project@ref:path"
	Tags       map[string]bool   // "project@tag"
	Issues     map[string][]*tracker.Issue
	Notes      map[string][]tracker.Note // "project#iid"
	Milestones map[string]tracker.Milestone
	Awards     map[string][]tracker.Award // "project#iid"
	Labels     map[string][]tracker.Label // group -> labels
	Me         tracker.User
	Branches   map[string]string           // "project@branch" -> ref it was created from
	Default    string                      // default branch of every project
	MRs        map[string][]*tracker.Merge // project -> merged deliveries
	Commits    map[string]string           // "project@ref:path" -> the commit that last changed it
	// Writes counts every mutating call, so a dry run can prove it wrote nothing.
	Writes int
}

var _ tracker.Tracker = (*Fake)(nil)

// New returns an empty fake.
func New() *Fake {
	return &Fake{Files: map[string][]byte{}, Tags: map[string]bool{}, Issues: map[string][]*tracker.Issue{},
		Notes: map[string][]tracker.Note{}, Milestones: map[string]tracker.Milestone{}, Awards: map[string][]tracker.Award{}, Labels: map[string][]tracker.Label{},
		Me: tracker.User{ID: 1, Username: "me"}, Branches: map[string]string{}, Default: "main",
		MRs: map[string][]*tracker.Merge{}}
}

// AddFile stores content at project, ref, path.
func (f *Fake) AddFile(project, ref, path string, content []byte) {
	f.Files[project+"@"+ref+":"+path] = content
}

// AddTag records a tag.
func (f *Fake) AddTag(project, tag string) { f.Tags[project+"@"+tag] = true }

func (f *Fake) GetFile(project, path, ref string) ([]byte, error) {
	b, ok := f.Files[project+"@"+ref+":"+path]
	if !ok {
		return nil, tracker.ErrNotFound
	}
	return b, nil
}

func (f *Fake) ListDir(project, dir, ref string) ([]string, error) {
	prefix := project + "@" + ref + ":" + strings.TrimSuffix(dir, "/") + "/"
	var out []string
	for k := range f.Files {
		if rest, ok := strings.CutPrefix(k, prefix); ok && !strings.Contains(rest, "/") {
			out = append(out, strings.TrimPrefix(k, project+"@"+ref+":"))
		}
	}
	slices.Sort(out)
	return out, nil
}

// LastCommit names a file's commit: the one SetCommit recorded, else one
// derived from where it is, so each file cites a commit.
func (f *Fake) LastCommit(project, path, ref string) (string, error) {
	if _, ok := f.Files[project+"@"+ref+":"+path]; !ok {
		return "", tracker.ErrNotFound
	}
	if c, ok := f.Commits[project+"@"+ref+":"+path]; ok {
		return c, nil
	}
	return fmt.Sprintf("%x", len(project+path)*7919+len(ref)), nil
}

// SetCommit records the commit that last changed a file.
func (f *Fake) SetCommit(project, ref, path, commit string) {
	if f.Commits == nil {
		f.Commits = map[string]string{}
	}
	f.Commits[project+"@"+ref+":"+path] = commit
}

func (f *Fake) TagExists(project, tag string) (bool, error) { return f.Tags[project+"@"+tag], nil }

func (f *Fake) find(project string, iid int) (*tracker.Issue, error) {
	for _, i := range f.Issues[project] {
		if i.IID == iid {
			return i, nil
		}
	}
	return nil, tracker.ErrNotFound
}

func (f *Fake) GetIssue(project string, iid int) (tracker.Issue, error) {
	i, err := f.find(project, iid)
	if err != nil {
		return tracker.Issue{}, err
	}
	return *i, nil
}

func (f *Fake) ListIssues(project string, q tracker.IssueQuery) ([]tracker.Issue, error) {
	var out []tracker.Issue
	for _, i := range f.Issues[project] {
		if q.State != "" && q.State != "all" && i.State != q.State {
			continue
		}
		if !hasAll(i.Labels, q.Labels) {
			continue
		}
		if q.Search != "" && !strings.Contains(i.Title+" "+i.Description, q.Search) {
			continue
		}
		if q.UpdatedAfter != "" && i.UpdatedAt != "" && i.UpdatedAt < q.UpdatedAfter {
			continue
		}
		out = append(out, *i)
	}
	return out, nil
}

func hasAll(have, want []string) bool {
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}

func (f *Fake) CreateIssue(project string, in tracker.NewIssue) (tracker.Issue, error) {
	f.Writes++
	i := &tracker.Issue{Project: project, IID: len(f.Issues[project]) + 1, Title: in.Title, Description: in.Description,
		Labels: slices.Clone(in.Labels), State: "opened", DueDate: in.DueDate, Milestone: in.Milestone, Author: f.Me.Username}
	i.WebURL = fmt.Sprintf("https://gitlab.example.com/%s/-/issues/%d", project, i.IID)
	f.Issues[project] = append(f.Issues[project], i)
	return *i, nil
}

func (f *Fake) UpdateIssue(project string, iid int, u tracker.IssueUpdate) (tracker.Issue, error) {
	f.Writes++
	i, err := f.find(project, iid)
	if err != nil {
		return tracker.Issue{}, err
	}
	if u.Description != nil {
		i.Description = *u.Description
	}
	if u.State != nil {
		i.State = map[string]string{"close": "closed", "reopen": "opened"}[*u.State]
		i.ClosedAt = ""
		if i.State == "closed" {
			i.ClosedAt = time.Now().UTC().Format(time.RFC3339)
		}
	}
	if u.DueDate != nil {
		i.DueDate = *u.DueDate
	}
	if u.Milestone != nil {
		i.Milestone = *u.Milestone
	}
	for _, l := range u.AddLabels {
		if !slices.Contains(i.Labels, l) {
			i.Labels = append(i.Labels, l)
		}
	}
	i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return slices.Contains(u.RemoveLabels, l) })
	return *i, nil
}

func (f *Fake) ListNotes(project string, iid int) ([]tracker.Note, error) {
	return f.Notes[fmt.Sprintf("%s#%d", project, iid)], nil
}

func (f *Fake) CreateNote(project string, iid int, body string) error {
	f.Writes++
	key := fmt.Sprintf("%s#%d", project, iid)
	f.Notes[key] = append(f.Notes[key], tracker.Note{ID: len(f.Notes[key]) + 1, Body: body, Author: f.Me.Username})
	return nil
}

func (f *Fake) GroupLabels(group string) ([]tracker.Label, error) {
	return slices.Clone(f.Labels[group]), nil
}

func (f *Fake) CreateGroupLabel(group string, l tracker.Label) error {
	f.Writes++
	for _, x := range f.Labels[group] {
		if x.Name == l.Name {
			return fmt.Errorf("label %q already exists", l.Name)
		}
	}
	f.Labels[group] = append(f.Labels[group], l)
	return nil
}

func (f *Fake) UpdateGroupLabel(group string, l tracker.Label) error {
	f.Writes++
	for i, x := range f.Labels[group] {
		if x.Name == l.Name {
			f.Labels[group][i] = l
			return nil
		}
	}
	return tracker.ErrNotFound
}

func (f *Fake) CurrentUser() (tracker.User, error) { return f.Me, nil }

func (f *Fake) AssignIssue(project string, iid int, user tracker.User) error {
	f.Writes++
	i, err := f.find(project, iid)
	if err != nil {
		return err
	}
	i.Assignees = []string{user.Username}
	return nil
}

func (f *Fake) DefaultBranch(string) (string, error) { return f.Default, nil }

func (f *Fake) BranchExists(project, branch string) (bool, error) {
	_, ok := f.Branches[project+"@"+branch]
	return ok, nil
}

func (f *Fake) CreateBranch(project, branch, ref string) error {
	f.Writes++
	if _, ok := f.Branches[project+"@"+branch]; ok {
		return fmt.Errorf("branch %s exists", branch)
	}
	f.Branches[project+"@"+branch] = ref
	return nil
}

func (f *Fake) Merges(project, author, since string) ([]tracker.Merge, error) {
	var out []tracker.Merge
	for _, m := range f.MRs[project] {
		if m.State == "merged" && (author == "" || m.Author == author) && m.MergedAt >= since[:min(len(since), 10)] {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (f *Fake) IssueAwards(project string, iid int) ([]tracker.Award, error) {
	return f.Awards[fmt.Sprintf("%s#%d", project, iid)], nil
}

// ActiveMilestones returns every milestone the fake holds, by title.
func (f *Fake) ActiveMilestones(project string) ([]tracker.Milestone, error) {
	var out []tracker.Milestone
	for _, title := range slices.Sorted(maps.Keys(f.Milestones)) {
		out = append(out, f.Milestones[title])
	}
	return out, nil
}

// EnsureMilestone keeps one milestone per title, as the client does for a
// group, and counts a write only when it creates or changes one.
func (f *Fake) EnsureMilestone(group string, m tracker.Milestone) (string, error) {
	cur, ok := f.Milestones[m.Title]
	switch {
	case !ok:
		f.Milestones[m.Title] = m
		f.Writes++
		return tracker.MilestoneCreated, nil
	case cur == m:
		return tracker.MilestoneUnchanged, nil
	}
	f.Milestones[m.Title] = m
	f.Writes++
	return tracker.MilestoneUpdated, nil
}

func (f *Fake) FindMilestone(project, title string) (tracker.Milestone, error) {
	m, ok := f.Milestones[title]
	if !ok {
		return tracker.Milestone{}, tracker.ErrNotFound
	}
	return m, nil
}
