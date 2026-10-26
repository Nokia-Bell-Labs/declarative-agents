// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package tracker

import "strings"

// Router serves each project from the tracking system its program
// configures for it, and every other project from the default (srd042):
// a system named for a project serves that project, and one named for a
// group serves every project under it that no closer name claims. A
// router holds whole tracking systems, so a project's issues and its code
// always come from one system and project#iid names one issue.
type Router struct {
	Default Tracker
	// Systems maps a project or group path to the system serving it.
	Systems map[string]Tracker
}

// For is the system serving path, a project or a group: the system named
// for the longest configured path that is path or an ancestor group of it,
// else the default.
func (r *Router) For(path string) Tracker {
	best, served := "", r.Default
	for key, t := range r.Systems {
		if (path == key || strings.HasPrefix(path, key+"/")) && len(key) > len(best) {
			best, served = key, t
		}
	}
	return served
}

// For is the system serving path when t routes, else t itself. A caller
// asking whether the system offers an optional interface, such as
// AdmittingCreator, asks the system For returns, which a router would
// otherwise hide.
func For(t Tracker, path string) Tracker {
	if r, ok := t.(*Router); ok {
		return r.For(path)
	}
	return t
}

// LastRead is the default system's last read, when it records one.
func (r *Router) LastRead() (Read, bool) {
	if rd, ok := r.Default.(Reader); ok {
		return rd.LastRead()
	}
	return Read{}, false
}

func (r *Router) GetFile(project, path, ref string) ([]byte, error) {
	return r.For(project).GetFile(project, path, ref)
}

func (r *Router) ListDir(project, dir, ref string) ([]string, error) {
	return r.For(project).ListDir(project, dir, ref)
}

func (r *Router) TagExists(project, tag string) (bool, error) {
	return r.For(project).TagExists(project, tag)
}

func (r *Router) GetIssue(project string, iid int) (Issue, error) {
	return r.For(project).GetIssue(project, iid)
}

func (r *Router) ListIssues(project string, q IssueQuery) ([]Issue, error) {
	return r.For(project).ListIssues(project, q)
}

func (r *Router) CreateIssue(project string, in NewIssue) (Issue, error) {
	return r.For(project).CreateIssue(project, in)
}

func (r *Router) UpdateIssue(project string, iid int, u IssueUpdate) (Issue, error) {
	return r.For(project).UpdateIssue(project, iid, u)
}

func (r *Router) ListNotes(project string, iid int) ([]Note, error) {
	return r.For(project).ListNotes(project, iid)
}

func (r *Router) CreateNote(project string, iid int, body string) error {
	return r.For(project).CreateNote(project, iid, body)
}

func (r *Router) FindMilestone(project, title string) (Milestone, error) {
	return r.For(project).FindMilestone(project, title)
}

func (r *Router) ActiveMilestones(project string) ([]Milestone, error) {
	return r.For(project).ActiveMilestones(project)
}

func (r *Router) EnsureMilestone(group string, m Milestone) (string, error) {
	return r.For(group).EnsureMilestone(group, m)
}

func (r *Router) Merges(project, author, since string) ([]Merge, error) {
	return r.For(project).Merges(project, author, since)
}

func (r *Router) IssueAwards(project string, iid int) ([]Award, error) {
	return r.For(project).IssueAwards(project, iid)
}

func (r *Router) GroupLabels(group string) ([]Label, error) { return r.For(group).GroupLabels(group) }

func (r *Router) CreateGroupLabel(group string, l Label) error {
	return r.For(group).CreateGroupLabel(group, l)
}

func (r *Router) UpdateGroupLabel(group string, l Label) error {
	return r.For(group).UpdateGroupLabel(group, l)
}

// CurrentUser is the person in the default system; UserIn names them in
// the system serving a project, where the account may differ.
func (r *Router) CurrentUser() (User, error) { return r.Default.CurrentUser() }

func (r *Router) AssignIssue(project string, iid int, user User) error {
	return r.For(project).AssignIssue(project, iid, user)
}

func (r *Router) DefaultBranch(project string) (string, error) {
	return r.For(project).DefaultBranch(project)
}

func (r *Router) BranchExists(project, branch string) (bool, error) {
	return r.For(project).BranchExists(project, branch)
}

func (r *Router) CreateBranch(project, branch, ref string) error {
	return r.For(project).CreateBranch(project, branch, ref)
}

// UserIn is the person running scrum-master as the system serving project knows
// them: their beads name, or their GitHub or GitLab login.
func UserIn(t Tracker, project string) (User, error) { return For(t, project).CurrentUser() }
