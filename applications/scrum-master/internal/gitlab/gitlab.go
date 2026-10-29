// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package gitlab is the glab tracking system: a program's issues, labels,
// and milestones in GitLab, and each branch reviewed and merged as a merge
// request, all through the glab CLI and the person's existing login, so
// scrum-master stores no token (srd042). GitLab has native due dates, start dates,
// group milestones, group labels, and award emoji. Files, tags, branches,
// and merged history come from the local checkout, as every tracking
// system reads them.
package gitlab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// API is how the tracker reaches GitLab: Do calls the REST API v4 through
// glab api, with body as the JSON input when it is not nil, and Run runs
// any other glab command.
type API interface {
	Do(method, path string, body any) ([]byte, error)
	Run(args ...string) ([]byte, error)
}

// CLI is API over the glab on PATH.
type CLI struct{}

func (CLI) Do(method, path string, body any) ([]byte, error) {
	args := []string{"api", "--method", method, path}
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		f, err := os.CreateTemp("", "scrum-master-glab-*.json")
		if err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())
		if _, err := f.Write(b); err != nil {
			f.Close()
			return nil, err
		}
		f.Close()
		args = append(args, "--input", f.Name())
	}
	return glab(args...)
}

func (CLI) Run(args ...string) ([]byte, error) { return glab(args...) }

func glab(args ...string) ([]byte, error) {
	cmd := exec.Command("glab", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		head := strings.Join(args[:min(len(args), 4)], " ")
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("glab %s: %s", head, msg)
		}
		return out, fmt.Errorf("glab %s: %w", head, err)
	}
	return out, nil
}

// Tracker is the glab tracking system for the command's checkout.
type Tracker struct {
	*checkout.Code
	API API
	me  *tracker.User
}

var (
	_ tracker.Tracker  = (*Tracker)(nil)
	_ tracker.Reviewer = (*Tracker)(nil)
)

// New serves the checkout at dir through api.
func New(dir string, api API) *Tracker {
	return &Tracker{Code: &checkout.Code{Dir: dir}, API: api}
}

func id(p string) string { return url.PathEscape(p) }

func (t *Tracker) get(p string, v any) error {
	b, err := t.API.Do("GET", p, nil)
	if err != nil {
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "Not Found") {
			return fmt.Errorf("%s: %w", p, tracker.ErrNotFound)
		}
		return err
	}
	return json.Unmarshal(b, v)
}

func (t *Tracker) all(p string, each func(json.RawMessage) error) error {
	sep := "?"
	if strings.Contains(p, "?") {
		sep = "&"
	}
	for page := 1; ; page++ {
		var items []json.RawMessage
		if err := t.get(fmt.Sprintf("%s%sper_page=100&page=%d", p, sep, page), &items); err != nil {
			return err
		}
		for _, it := range items {
			if err := each(it); err != nil {
				return err
			}
		}
		if len(items) < 100 {
			return nil
		}
	}
}

// GroupProjects lists the project paths under a group, subgroups
// included, so reconcile can sweep the program's group (GH-2822).
func (t *Tracker) GroupProjects(group string) ([]string, error) {
	var out []string
	err := t.all("groups/"+id(group)+"/projects?include_subgroups=true", func(m json.RawMessage) error {
		var p struct {
			Path string `json:"path_with_namespace"`
		}
		if err := json.Unmarshal(m, &p); err != nil {
			return err
		}
		out = append(out, p.Path)
		return nil
	})
	return out, err
}

type glUser struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}

type glIssue struct {
	IID         int      `json:"iid"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	State       string   `json:"state"`
	WebURL      string   `json:"web_url"`
	DueDate     string   `json:"due_date"`
	Labels      []string `json:"labels"`
	Assignees   []glUser `json:"assignees"`
	Author      glUser   `json:"author"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	ClosedAt    string   `json:"closed_at"`
	Milestone   *struct {
		Title string `json:"title"`
	} `json:"milestone"`
}

func (g glIssue) view(project string) tracker.Issue {
	i := tracker.Issue{Project: project, IID: g.IID, Title: g.Title, Description: g.Description, Labels: g.Labels, State: "opened",
		WebURL: g.WebURL, DueDate: g.DueDate, Author: g.Author.Username, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt}
	if g.State == "closed" {
		i.State, i.ClosedAt = "closed", g.ClosedAt
	}
	for _, a := range g.Assignees {
		i.Assignees = append(i.Assignees, a.Username)
	}
	if g.Milestone != nil {
		i.Milestone = g.Milestone.Title
	}
	return i
}

func (t *Tracker) GetIssue(project string, iid int) (tracker.Issue, error) {
	var g glIssue
	if err := t.get(fmt.Sprintf("projects/%s/issues/%d", id(project), iid), &g); err != nil {
		return tracker.Issue{}, err
	}
	return g.view(project), nil
}

func (t *Tracker) ListIssues(project string, q tracker.IssueQuery) ([]tracker.Issue, error) {
	v := url.Values{"state": {"all"}}
	if q.State == "opened" || q.State == "closed" {
		v.Set("state", q.State)
	}
	if len(q.Labels) > 0 {
		v.Set("labels", strings.Join(q.Labels, ","))
	}
	if q.Search != "" {
		v.Set("search", q.Search)
	}
	if q.UpdatedAfter != "" {
		v.Set("updated_after", q.UpdatedAfter+"T00:00:00Z")
	}
	var out []tracker.Issue
	err := t.all(fmt.Sprintf("projects/%s/issues?%s", id(project), v.Encode()), func(raw json.RawMessage) error {
		var g glIssue
		if err := json.Unmarshal(raw, &g); err != nil {
			return err
		}
		out = append(out, g.view(project))
		return nil
	})
	slices.SortFunc(out, func(a, b tracker.Issue) int { return a.IID - b.IID })
	return out, err
}

// ancestors are project's groups, nearest first: g/sub/repo has g/sub and g.
func ancestors(project string) []string {
	var out []string
	for p := path.Dir(project); p != "." && p != "/"; p = path.Dir(p) {
		out = append(out, p)
	}
	return out
}

type glMilestone struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	StartDate   string `json:"start_date"`
	DueDate     string `json:"due_date"`
	State       string `json:"state"`
}

func (m glMilestone) view() tracker.Milestone {
	return tracker.Milestone{Title: m.Title, Description: m.Description, StartDate: m.StartDate, DueDate: m.DueDate}
}

// milestones lists the milestones of the project and its ancestor groups,
// in that order, in state (active, closed, or all).
func (t *Tracker) milestones(project, state string) ([]glMilestone, error) {
	q := ""
	if state != "all" {
		q = "?state=" + state
	}
	var out []glMilestone
	scopes := append([]string{"projects/" + id(project)}, func() []string {
		var gs []string
		for _, g := range ancestors(project) {
			gs = append(gs, "groups/"+id(g))
		}
		return gs
	}()...)
	for _, scope := range scopes {
		err := t.all(scope+"/milestones"+q, func(raw json.RawMessage) error {
			var m glMilestone
			if err := json.Unmarshal(raw, &m); err != nil {
				return err
			}
			out = append(out, m)
			return nil
		})
		if err != nil && !errors.Is(err, tracker.ErrNotFound) {
			return nil, err
		}
	}
	return out, nil
}

func (t *Tracker) FindMilestone(project, title string) (tracker.Milestone, error) {
	ms, err := t.milestones(project, "all")
	if err != nil {
		return tracker.Milestone{}, err
	}
	for _, m := range ms {
		if m.Title == title {
			return m.view(), nil
		}
	}
	return tracker.Milestone{}, fmt.Errorf("milestone %s for %s: %w", title, project, tracker.ErrNotFound)
}

func (t *Tracker) ActiveMilestones(project string) ([]tracker.Milestone, error) {
	ms, err := t.milestones(project, "active")
	if err != nil {
		return nil, err
	}
	var out []tracker.Milestone
	for _, m := range ms {
		out = append(out, m.view())
	}
	slices.SortStableFunc(out, func(a, b tracker.Milestone) int {
		return strings.Compare(a.DueDate+"\x00"+a.Title, b.DueDate+"\x00"+b.Title)
	})
	return out, nil
}

func (t *Tracker) milestoneID(project, title string) (int, error) {
	ms, err := t.milestones(project, "all")
	if err != nil {
		return 0, err
	}
	for _, m := range ms {
		if m.Title == title {
			return m.ID, nil
		}
	}
	return 0, fmt.Errorf("milestone %s for %s: %w", title, project, tracker.ErrNotFound)
}

// EnsureMilestone keeps one group milestone by title: GitLab's are native.
func (t *Tracker) EnsureMilestone(group string, m tracker.Milestone) (string, error) {
	var ms []glMilestone
	err := t.all("groups/"+id(group)+"/milestones?title="+url.QueryEscape(m.Title), func(raw json.RawMessage) error {
		var x glMilestone
		if err := json.Unmarshal(raw, &x); err != nil {
			return err
		}
		if x.Title == m.Title {
			ms = append(ms, x)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	want := map[string]string{"title": m.Title, "description": m.Description, "start_date": m.StartDate, "due_date": m.DueDate}
	switch {
	case len(ms) == 0:
		_, err = t.API.Do("POST", "groups/"+id(group)+"/milestones", want)
		return tracker.MilestoneCreated, err
	case ms[0].view() == m:
		return tracker.MilestoneUnchanged, nil
	}
	_, err = t.API.Do("PUT", fmt.Sprintf("groups/%s/milestones/%d", id(group), ms[0].ID), want)
	return tracker.MilestoneUpdated, err
}

func (t *Tracker) CreateIssue(project string, in tracker.NewIssue) (tracker.Issue, error) {
	body := map[string]any{"title": in.Title, "description": in.Description, "labels": strings.Join(in.Labels, ",")}
	if in.DueDate != "" {
		body["due_date"] = in.DueDate
	}
	if in.Milestone != "" {
		mid, err := t.milestoneID(project, in.Milestone)
		if err != nil {
			return tracker.Issue{}, err
		}
		body["milestone_id"] = mid
	}
	b, err := t.API.Do("POST", "projects/"+id(project)+"/issues", body)
	if err != nil {
		return tracker.Issue{}, err
	}
	var g glIssue
	if err := json.Unmarshal(b, &g); err != nil {
		return tracker.Issue{}, err
	}
	return g.view(project), nil
}

func (t *Tracker) UpdateIssue(project string, iid int, u tracker.IssueUpdate) (tracker.Issue, error) {
	put := map[string]any{}
	if u.State != nil {
		switch *u.State {
		case "close", "reopen":
			put["state_event"] = *u.State
		default:
			return tracker.Issue{}, fmt.Errorf("state %q is neither close nor reopen", *u.State)
		}
	}
	if u.Description != nil {
		put["description"] = *u.Description
	}
	if u.DueDate != nil {
		put["due_date"] = *u.DueDate
	}
	if u.Milestone != nil {
		if *u.Milestone == "" {
			put["milestone_id"] = 0
		} else {
			mid, err := t.milestoneID(project, *u.Milestone)
			if err != nil {
				return tracker.Issue{}, err
			}
			put["milestone_id"] = mid
		}
	}
	if len(u.AddLabels) > 0 {
		put["add_labels"] = strings.Join(u.AddLabels, ",")
	}
	if len(u.RemoveLabels) > 0 {
		put["remove_labels"] = strings.Join(u.RemoveLabels, ",")
	}
	if len(put) > 0 {
		if _, err := t.API.Do("PUT", fmt.Sprintf("projects/%s/issues/%d", id(project), iid), put); err != nil {
			return tracker.Issue{}, err
		}
	}
	return t.GetIssue(project, iid)
}

func (t *Tracker) ListNotes(project string, iid int) ([]tracker.Note, error) {
	var out []tracker.Note
	err := t.all(fmt.Sprintf("projects/%s/issues/%d/notes?sort=asc&order_by=created_at", id(project), iid), func(raw json.RawMessage) error {
		var n struct {
			ID     int    `json:"id"`
			Body   string `json:"body"`
			System bool   `json:"system"`
			Author glUser `json:"author"`
		}
		if err := json.Unmarshal(raw, &n); err != nil {
			return err
		}
		out = append(out, tracker.Note{ID: n.ID, Body: n.Body, Author: n.Author.Username, System: n.System})
		return nil
	})
	return out, err
}

func (t *Tracker) CreateNote(project string, iid int, body string) error {
	_, err := t.API.Do("POST", fmt.Sprintf("projects/%s/issues/%d/notes", id(project), iid), map[string]string{"body": body})
	return err
}

func (t *Tracker) IssueAwards(project string, iid int) ([]tracker.Award, error) {
	var out []tracker.Award
	err := t.all(fmt.Sprintf("projects/%s/issues/%d/award_emoji", id(project), iid), func(raw json.RawMessage) error {
		var a struct {
			Name string `json:"name"`
			User glUser `json:"user"`
		}
		if err := json.Unmarshal(raw, &a); err != nil {
			return err
		}
		out = append(out, tracker.Award{Name: a.Name, User: a.User.Username})
		return nil
	})
	return out, err
}

type glLabel struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

func (t *Tracker) GroupLabels(group string) ([]tracker.Label, error) {
	var out []tracker.Label
	err := t.all("groups/"+id(group)+"/labels?only_group_labels=true", func(raw json.RawMessage) error {
		var l glLabel
		if err := json.Unmarshal(raw, &l); err != nil {
			return err
		}
		out = append(out, tracker.Label{Name: l.Name, Color: l.Color, Description: l.Description})
		return nil
	})
	return out, err
}

func (t *Tracker) CreateGroupLabel(group string, l tracker.Label) error {
	_, err := t.API.Do("POST", "groups/"+id(group)+"/labels", glLabel{Name: l.Name, Color: l.Color, Description: l.Description})
	return err
}

func (t *Tracker) UpdateGroupLabel(group string, l tracker.Label) error {
	_, err := t.API.Do("PUT", "groups/"+id(group)+"/labels/"+url.PathEscape(l.Name), map[string]string{"color": l.Color, "description": l.Description})
	return err
}

// CurrentUser is the account glab is logged in as.
func (t *Tracker) CurrentUser() (tracker.User, error) {
	if t.me != nil {
		return *t.me, nil
	}
	var u glUser
	if err := t.get("user", &u); err != nil {
		return tracker.User{}, err
	}
	t.me = &tracker.User{ID: u.ID, Username: u.Username}
	return *t.me, nil
}

// AssignIssue makes user the issue's only assignee; pop's rule that only
// the caller is assigned stays scrum-master's (srd041 R9.1).
func (t *Tracker) AssignIssue(project string, iid int, user tracker.User) error {
	uid := user.ID
	if uid == 0 {
		var us []glUser
		if err := t.get("users?username="+url.QueryEscape(user.Username), &us); err != nil {
			return err
		}
		if len(us) == 0 {
			return fmt.Errorf("no GitLab user %s: %w", user.Username, tracker.ErrNotFound)
		}
		uid = us[0].ID
	}
	_, err := t.API.Do("PUT", fmt.Sprintf("projects/%s/issues/%d", id(project), iid), map[string]any{"assignee_ids": []int{uid}})
	return err
}

// CloseOnMerge closes the issue on the merge that delivered it, citing the
// merge commit, as beads does (srd042 R2.3).
func (t *Tracker) CloseOnMerge(project string, iid int, commit string) (bool, error) {
	i, err := t.GetIssue(project, iid)
	if err != nil || i.State == "closed" {
		return false, err
	}
	if err := t.CreateNote(project, iid, fmt.Sprintf("Closed by the merge %s.", commit)); err != nil {
		return false, err
	}
	closed := "close"
	_, err = t.UpdateIssue(project, iid, tracker.IssueUpdate{State: &closed})
	return err == nil, err
}

// CreateBranch opens the branch with one empty commit of its own, so the
// merge request pop opens has a change from its first moment.
func (t *Tracker) CreateBranch(project, branch, ref string) error {
	return t.OpenBranch(project, branch, ref, "Open "+branch+" for its merge request")
}

type glMR struct {
	IID            int    `json:"iid"`
	WebURL         string `json:"web_url"`
	Draft          bool   `json:"draft"`
	SHA            string `json:"sha"`
	MergeCommitSHA string `json:"merge_commit_sha"`
}

func (t *Tracker) mr(project, branch string) (glMR, error) {
	var ms []glMR
	if err := t.get(fmt.Sprintf("projects/%s/merge_requests?state=opened&source_branch=%s", id(project), url.QueryEscape(branch)), &ms); err != nil {
		return glMR{}, err
	}
	if len(ms) == 0 {
		return glMR{}, fmt.Errorf("no open merge request for %s in %s: %w", branch, project, tracker.ErrNotFound)
	}
	return ms[0], nil
}

func (t *Tracker) OpenReview(project, branch, base, title, body string) (string, error) {
	if m, err := t.mr(project, branch); err == nil {
		return m.WebURL, nil
	} else if !errors.Is(err, tracker.ErrNotFound) {
		return "", err
	}
	b, err := t.API.Do("POST", "projects/"+id(project)+"/merge_requests",
		map[string]any{"source_branch": branch, "target_branch": base, "title": "Draft: " + title, "description": body})
	if err != nil {
		return "", err
	}
	var m glMR
	return m.WebURL, json.Unmarshal(b, &m)
}

// approvalPrefix opens the note an approval leaves on the merge request:
// GitLab's approvals do not name the commit approved, so the note does.
const approvalPrefix = "<!-- scrum-master approve commit="

type glNote struct {
	Body   string `json:"body"`
	Author glUser `json:"author"`
}

func (t *Tracker) mrNotes(project string, iid int) ([]glNote, error) {
	var out []glNote
	err := t.all(fmt.Sprintf("projects/%s/merge_requests/%d/notes?sort=asc&order_by=created_at", id(project), iid), func(raw json.RawMessage) error {
		var n glNote
		if err := json.Unmarshal(raw, &n); err != nil {
			return err
		}
		out = append(out, n)
		return nil
	})
	return out, err
}

// Approve approves the merge request at commit, pinned to that head with
// GitLab's sha check, and leaves a note naming the commit.
func (t *Tracker) Approve(project, branch, commit string) (bool, error) {
	m, err := t.mr(project, branch)
	if err != nil {
		return false, err
	}
	me, err := t.CurrentUser()
	if err != nil {
		return false, err
	}
	notes, err := t.mrNotes(project, m.IID)
	if err != nil {
		return false, err
	}
	marker := approvalPrefix + commit + " -->"
	if slices.ContainsFunc(notes, func(n glNote) bool { return n.Author.Username == me.Username && strings.Contains(n.Body, marker) }) {
		return false, nil
	}
	if _, err := t.API.Do("POST", fmt.Sprintf("projects/%s/merge_requests/%d/approve", id(project), m.IID), map[string]string{"sha": commit}); err != nil {
		return false, err
	}
	_, err = t.API.Do("POST", fmt.Sprintf("projects/%s/merge_requests/%d/notes", id(project), m.IID),
		map[string]string{"body": fmt.Sprintf("Approved %s at %s.\n\n%s", branch, commit, marker)})
	return err == nil, err
}

// Approval names who approved the merge request at tip: a person GitLab
// lists among its approvers whose note names tip.
func (t *Tracker) Approval(project, branch, tip string, excluded []string) (string, string, error) {
	m, err := t.mr(project, branch)
	if err != nil {
		return "", "", err
	}
	var a struct {
		ApprovedBy []struct {
			User glUser `json:"user"`
		} `json:"approved_by"`
	}
	if err := t.get(fmt.Sprintf("projects/%s/merge_requests/%d/approvals", id(project), m.IID), &a); err != nil {
		return "", "", err
	}
	approvers := map[string]bool{}
	for _, x := range a.ApprovedBy {
		approvers[x.User.Username] = true
	}
	notes, err := t.mrNotes(project, m.IID)
	if err != nil {
		return "", "", err
	}
	stale := ""
	for _, n := range notes {
		who := n.Author.Username
		at := strings.Index(n.Body, approvalPrefix)
		if at < 0 || slices.Contains(excluded, who) {
			continue
		}
		commit, _, _ := strings.Cut(n.Body[at+len(approvalPrefix):], " ")
		if commit == tip && approvers[who] {
			return who, "", nil
		}
		stale = commit
	}
	return "", stale, nil
}

// Merge marks the draft ready and merges the merge request at tip with
// message as its merge commit's message.
func (t *Tracker) Merge(project, branch, tip, message string) (string, error) {
	m, err := t.mr(project, branch)
	if err != nil {
		return "", err
	}
	if m.Draft {
		if _, err := t.API.Run("mr", "update", strconv.Itoa(m.IID), "--ready", "--repo", project); err != nil {
			return "", err
		}
	}
	at := fmt.Sprintf("projects/%s/merge_requests/%d", id(project), m.IID)
	for try := 1; ; try++ {
		b, err := t.API.Do("PUT", at+"/merge", map[string]string{"sha": tip, "merge_commit_message": message})
		if err == nil {
			var merged glMR
			if err := json.Unmarshal(b, &merged); err != nil {
				return "", err
			}
			return merged.MergeCommitSHA, nil
		}
		var s struct {
			Status string `json:"detailed_merge_status"`
		}
		if t.get(at, &s) != nil || !slices.Contains(mergeChecking, s.Status) || try == mergeTries {
			if s.Status != "" {
				return "", fmt.Errorf("%w (GitLab reports the merge request %s)", err, s.Status)
			}
			return "", err
		}
		time.Sleep(mergeWait)
	}
}

// mergeChecking are the detailed_merge_status values under which GitLab's
// refusal of a merge is its own lag: one that just left draft refuses with
// 405 until its check ends, and may already read mergeable while the merge
// endpoint still refuses. Merge tries again, mergeWait apart, up to
// mergeTries times.
var (
	mergeChecking = []string{"checking", "unchecked", "preparing", "approvals_syncing", "mergeable"}
	mergeWait     = time.Second
	mergeTries    = 30
)
