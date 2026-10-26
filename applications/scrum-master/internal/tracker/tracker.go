// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package tracker is scrum-master's tracker boundary. Every package that reads or
// writes issues goes through the Tracker interface: production uses the
// beads tracker (internal/beads), and tests use it or the fake in
// internal/tracker/fake.
package tracker

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when a project, issue, file, tag, or milestone does
// not exist.
var ErrNotFound = errors.New("not found")

// Issue is the part of a tracker issue scrum-master reads.
type Issue struct {
	Project     string   `json:"project"`
	IID         int      `json:"iid"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Labels      []string `json:"labels"`
	State       string   `json:"state"`
	WebURL      string   `json:"web_url"`
	DueDate     string   `json:"due_date,omitempty"`
	Milestone   string   `json:"milestone,omitempty"`
	Assignees   []string `json:"assignees,omitempty"`
	Author      string   `json:"author,omitempty"`
	CreatedAt   string   `json:"created_at,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	// ClosedAt is when the issue last closed, empty while it is open; the
	// timeline reads actual ends from it (GH-82).
	ClosedAt string `json:"closed_at,omitempty"`
}

// HasLabel reports whether the issue carries the label.
func (i Issue) HasLabel(label string) bool {
	for _, l := range i.Labels {
		if l == label {
			return true
		}
	}
	return false
}

// NewIssue is what CreateIssue files. It never names an assignee: only pop
// assigns, and only the person who runs it (srd041 R9.1).
type NewIssue struct {
	Title       string
	Description string
	Labels      []string
	DueDate     string // YYYY-MM-DD, or empty
	Milestone   string // milestone title, or empty
}

// IssueUpdate changes an issue; nil fields are left alone.
type IssueUpdate struct {
	// State is "close" or "reopen", or nil to leave the state alone.
	State *string
	// CloseReason, with State "close", records why the issue closed where
	// the tracking system keeps a reason (beads' close_reason; GH-20).
	CloseReason *string
	Description *string
	DueDate     *string
	// Milestone sets the issue's milestone, by title.
	Milestone    *string
	AddLabels    []string
	RemoveLabels []string
}

// IssueQuery filters ListIssues.
type IssueQuery struct {
	State  string // opened, closed, or all
	Labels []string
	Search string
	// UpdatedAfter is a YYYY-MM-DD date, or empty.
	UpdatedAfter string
}

// Note is one comment on an issue.
type Note struct {
	ID     int    `json:"id"`
	Body   string `json:"body"`
	Author string `json:"author"`
	System bool   `json:"system"`
}

// Milestone is a project or group milestone.
type Milestone struct {
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	StartDate   string `json:"start_date,omitempty"`
	DueDate     string `json:"due_date,omitempty"`
}

// Whether EnsureMilestone created, updated, or left a milestone as it was.
const (
	MilestoneCreated   = "created"
	MilestoneUpdated   = "updated"
	MilestoneUnchanged = "unchanged"
)

// Award is an award emoji on an issue and the person who gave it.
type Award struct {
	Name string `json:"name"`
	User string `json:"user"`
}

// Merge is one merged delivery in a project's history, as reports
// read it; on beads a merge commit (srd042 R2.3).
type Merge struct {
	IID          int    `json:"iid"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	State        string `json:"state"`
	WebURL       string `json:"web_url"`
	Author       string `json:"author,omitempty"`
	MergedAt     string `json:"merged_at,omitempty"`
	// Commit is the merge commit, set when the merge is read from a
	// checkout's history rather than a forge's merge requests: such a merge
	// has no request to name, so it is named by its commit (GH-141).
	Commit string `json:"commit,omitempty"`
}

// Ref names the merge in project: its merge request, project!iid, or for a
// merge read from history, its commit, project@sha (GH-141).
func (m Merge) Ref(project string) string {
	if m.Commit != "" {
		return project + "@" + m.Commit
	}
	return fmt.Sprintf("%s!%d", project, m.IID)
}

// User is a tracker account.
type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

// Label is a group label.
type Label struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// Read names the tracker state a command read: the tracker repository
// commit, when it was pulled, and whether the command read the clone's cache
// because the repository was unreachable (srd042 R3.3).
type Read struct {
	Commit  string `json:"commit"`
	At      string `json:"at"`
	Offline bool   `json:"offline,omitempty"`
}

// Reader is a tracker that knows what it last read. The beads tracker is
// one; the fake, which reads nothing remote, is not.
type Reader interface {
	LastRead() (Read, bool)
}

// Triager is a tracker that holds admission as srd041 state transitions
// fired by lint, not as a bare label: Triage admits or flags one issue and
// reports whether its state changed. The fake is not one; there admission
// is the needs-triage label alone.
type Triager interface {
	Triage(project string, iid int, admitted bool) (bool, error)
}

// AdmittingCreator creates an issue and records its admission in the same
// tracker write: admit sees the issue as created and returns lint's reasons
// and the note to leave on a failing issue. A tracker whose writes are
// costly offers it, so filing is one write and never leaves an issue
// without its admission.
type AdmittingCreator interface {
	CreateAdmitted(project string, in NewIssue, admit func(Issue) (reasons []string, note string)) (Issue, error)
}

// Reviewer is a tracking system whose host reviews and merges a branch, as
// a GitHub pull request or a GitLab merge request does (srd013 R5.5). pop
// opens the review, a reviewer approves the head commit there, and merge
// merges it there; a tracking system without one approves by comment and
// merges in the local checkout.
type Reviewer interface {
	// OpenReview opens the review of branch into base, or returns the one
	// already open, and reports its URL.
	OpenReview(project, branch, base, title, body string) (string, error)
	// Approve approves the review of branch at commit as the current user,
	// and reports whether it approved now: an approval of the same commit
	// by the same person is not repeated. The host refuses the author.
	Approve(project, branch, commit string) (bool, error)
	// Approval names who approved the review of branch at tip, other than
	// excluded, and the commit an earlier approval named, if any.
	Approval(project, branch, tip string, excluded []string) (by, stale string, err error)
	// Merge merges the review of branch at tip with message, and returns
	// the merge commit.
	Merge(project, branch, tip, message string) (string, error)
}

// Issues is the issue half of a tracking system: the issues, their notes
// and the ledger events in them, milestones, labels, and the people who
// pop and approve (srd042). Every tracking system supplies it, beads from
// its tracker repository and a hosted one from its host.
type Issues interface {
	GetIssue(project string, iid int) (Issue, error)
	ListIssues(project string, q IssueQuery) ([]Issue, error)
	CreateIssue(project string, in NewIssue) (Issue, error)
	UpdateIssue(project string, iid int, u IssueUpdate) (Issue, error)
	ListNotes(project string, iid int) ([]Note, error)
	CreateNote(project string, iid int, body string) error
	// FindMilestone looks in the project, then in its ancestor groups.
	FindMilestone(project, title string) (Milestone, error)
	// ActiveMilestones lists the active milestones of the project and its
	// ancestor groups.
	ActiveMilestones(project string) ([]Milestone, error)
	// EnsureMilestone makes the group's milestone titled m.Title carry m's
	// description and dates: it creates the milestone when the group has
	// none by that title and updates it otherwise, so planning a release
	// twice leaves one milestone (srd001 R7.1, srd031 R4.2). It returns
	// MilestoneCreated, MilestoneUpdated, or MilestoneUnchanged.
	EnsureMilestone(group string, m Milestone) (string, error)
	// IssueAwards lists the award emoji on an issue (srd015 R1.2).
	IssueAwards(project string, iid int) ([]Award, error)
	// GroupLabels lists the labels defined on the group itself.
	GroupLabels(group string) ([]Label, error)
	CreateGroupLabel(group string, l Label) error
	UpdateGroupLabel(group string, l Label) error
	// CurrentUser is the account the token belongs to.
	CurrentUser() (User, error)
	// AssignIssue makes user the issue's only assignee. Only pop calls it
	// (srd041 R9.1).
	AssignIssue(project string, iid int, user User) error
}

// Forge is the code half of a tracking system: a project's files, tags,
// branches, and merged history. Every tracking system reads these from the
// project's checkout and its origin; how a branch is reviewed and merged is
// program's delivery, local or through the host's review (srd013 R5).
type Forge interface {
	GetFile(project, path, ref string) ([]byte, error)
	// ListDir returns the paths of the files directly under dir at ref.
	ListDir(project, dir, ref string) ([]string, error)
	TagExists(project, tag string) (bool, error)
	// Merges lists the merges into the project since a YYYY-MM-DD day,
	// from its midnight UTC, or since an RFC 3339 instant, by author or,
	// with author empty, by anyone. A merge's MergedAt is its UTC day
	// (srd018 R3.2, R8.1).
	Merges(project, author, since string) ([]Merge, error)
	DefaultBranch(project string) (string, error)
	BranchExists(project, branch string) (bool, error)
	CreateBranch(project, branch, ref string) error
}

// HostFiles is a tracking system whose host serves a repository's files
// on its default branch without a checkout of it, as GitHub's and GitLab's
// APIs do: what a reader collecting from many repositories asks first, so
// it clones none of them (GH-2725). The checkout forge every system shares
// stays the reader of record (srd042 R2.3), and such a reader falls back to
// it when the host cannot answer.
type HostFiles interface {
	// HostTree lists the paths of the files under dir, at any depth.
	HostTree(project, dir string) ([]string, error)
	// HostFile reads one file and names the commit that last changed it.
	HostFile(project, path string) (content []byte, commit string, err error)
}

// FileCommits names the commit that last changed a file at a ref, as a
// checkout's history does: how a reader cites a file it read from a
// checkout rather than from a host (GH-2725).
type FileCommits interface {
	LastCommit(project, path, ref string) (string, error)
}

// Checkouts says whether a project has a checkout of its own to read, as
// opposed to the command's checkout the code half reads for any project it
// finds no sibling for (GH-2725).
type Checkouts interface {
	HasCheckout(project string) bool
}

// Tracker is a whole tracking system, both halves, as one package: bd, gh,
// or glab, chosen per repository (srd042). A caller that needs one half
// takes that half; a new need is a new method on the half it belongs to.
type Tracker interface {
	Issues
	Forge
}
