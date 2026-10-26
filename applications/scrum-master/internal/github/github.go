// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package github is the gh tracking system: a program's issues, labels,
// and milestones in GitHub, and each branch reviewed and merged as a pull
// request, all through the gh CLI and the person's existing login, so scrum-master
// stores no token (srd042). Files, tags, branches, and merged history come
// from the local checkout, as every tracking system reads them.
package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// API is how the tracker reaches GitHub: Do calls the REST API through gh
// api, with body as the JSON input when it is not nil, and Run runs any
// other gh command.
type API interface {
	Do(method, path string, body any) ([]byte, error)
	Run(args ...string) ([]byte, error)
}

// CLI is API over the gh on PATH.
type CLI struct{}

func (CLI) Do(method, path string, body any) ([]byte, error) {
	args := []string{"api", "--method", method, path}
	var in []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		in = b
		args = append(args, "--input", "-")
	}
	return gh(in, args...)
}

func (CLI) Run(args ...string) ([]byte, error) { return gh(nil, args...) }

func gh(stdin []byte, args ...string) ([]byte, error) {
	cmd := exec.Command("gh", args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return out, fmt.Errorf("gh %s: %s", strings.Join(args[:min(len(args), 4)], " "), msg)
		}
		return out, fmt.Errorf("gh %s: %w", strings.Join(args[:min(len(args), 4)], " "), err)
	}
	return out, nil
}

// Tracker is the gh tracking system for the command's checkout.
type Tracker struct {
	*checkout.Code
	API API
	// Paths are the projects and owners control/trackers.md routes here;
	// a group's labels and milestones go to each repository among them.
	Paths []string
	me    *tracker.User
}

var (
	_ tracker.Tracker  = (*Tracker)(nil)
	_ tracker.Reviewer = (*Tracker)(nil)
)

// New serves the checkout at dir through api.
func New(dir string, api API, paths []string) *Tracker {
	return &Tracker{Code: &checkout.Code{Dir: dir}, API: api, Paths: paths}
}

// GitHub has no due date on an issue and no start date on a milestone, so
// each is kept as a line of the description the adapter reads back.
var (
	dueLine   = regexp.MustCompile(`(?m)^Due: (\d{4}-\d{2}-\d{2})\n?`)
	startLine = regexp.MustCompile(`(?m)^Start: (\d{4}-\d{2}-\d{2})\n?`)
)

func withLine(body string, re *regexp.Regexp, key, value string) string {
	body = re.ReplaceAllString(body, "")
	if value == "" {
		return body
	}
	return strings.TrimRight(body, "\n") + "\n\n" + key + ": " + value + "\n"
}

func lineValue(body string, re *regexp.Regexp) string {
	if m := re.FindStringSubmatch(body); m != nil {
		return m[1]
	}
	return ""
}

type ghIssue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	UpdatedAt string `json:"updated_at"`
	CreatedAt string `json:"created_at"`
	ClosedAt  string `json:"closed_at"`
	User      struct {
		Login string `json:"login"`
	} `json:"user"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
	Assignees []struct {
		Login string `json:"login"`
	} `json:"assignees"`
	Milestone *struct {
		Title string `json:"title"`
	} `json:"milestone"`
	PullRequest *struct{} `json:"pull_request"`
}

func (g ghIssue) view(project string) tracker.Issue {
	i := tracker.Issue{Project: project, IID: g.Number, Title: g.Title, Description: g.Body, WebURL: g.HTMLURL,
		DueDate: lineValue(g.Body, dueLine), Author: g.User.Login, CreatedAt: g.CreatedAt, UpdatedAt: g.UpdatedAt, State: "opened"}
	if g.State == "closed" {
		i.State, i.ClosedAt = "closed", g.ClosedAt
	}
	for _, l := range g.Labels {
		i.Labels = append(i.Labels, l.Name)
	}
	for _, a := range g.Assignees {
		i.Assignees = append(i.Assignees, a.Login)
	}
	if g.Milestone != nil {
		i.Milestone = g.Milestone.Title
	}
	return i
}

func (t *Tracker) get(path string, v any) error {
	b, err := t.API.Do("GET", path, nil)
	if err != nil {
		if strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "404") {
			return fmt.Errorf("%s: %w", path, tracker.ErrNotFound)
		}
		return err
	}
	return json.Unmarshal(b, v)
}

// all reads every page of a list endpoint.
func (t *Tracker) all(path string, each func(json.RawMessage) error) error {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	for page := 1; ; page++ {
		var items []json.RawMessage
		if err := t.get(fmt.Sprintf("%s%sper_page=100&page=%d", path, sep, page), &items); err != nil {
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

func (t *Tracker) GetIssue(project string, iid int) (tracker.Issue, error) {
	var g ghIssue
	if err := t.get(fmt.Sprintf("repos/%s/issues/%d", project, iid), &g); err != nil {
		return tracker.Issue{}, err
	}
	if g.PullRequest != nil {
		return tracker.Issue{}, fmt.Errorf("%s#%d is a pull request: %w", project, iid, tracker.ErrNotFound)
	}
	return g.view(project), nil
}

func (t *Tracker) ListIssues(project string, q tracker.IssueQuery) ([]tracker.Issue, error) {
	state := map[string]string{"opened": "open", "closed": "closed"}[q.State]
	if state == "" {
		state = "all"
	}
	v := url.Values{"state": {state}}
	if len(q.Labels) > 0 {
		v.Set("labels", strings.Join(q.Labels, ","))
	}
	if q.UpdatedAfter != "" {
		v.Set("since", q.UpdatedAfter+"T00:00:00Z")
	}
	var out []tracker.Issue
	search := strings.ToLower(q.Search)
	err := t.all(fmt.Sprintf("repos/%s/issues?%s", project, v.Encode()), func(raw json.RawMessage) error {
		var g ghIssue
		if err := json.Unmarshal(raw, &g); err != nil {
			return err
		}
		if g.PullRequest != nil || (search != "" && !strings.Contains(strings.ToLower(g.Title+"\n"+g.Body), search)) {
			return nil
		}
		out = append(out, g.view(project))
		return nil
	})
	slices.SortFunc(out, func(a, b tracker.Issue) int { return a.IID - b.IID })
	return out, err
}

func (t *Tracker) milestoneNumber(project, title string) (int, error) {
	n := 0
	err := t.all(fmt.Sprintf("repos/%s/milestones?state=all", project), func(raw json.RawMessage) error {
		var m ghMilestone
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		if m.Title == title {
			n = m.Number
		}
		return nil
	})
	if err == nil && n == 0 {
		err = fmt.Errorf("milestone %s in %s: %w", title, project, tracker.ErrNotFound)
	}
	return n, err
}

func (t *Tracker) CreateIssue(project string, in tracker.NewIssue) (tracker.Issue, error) {
	body := map[string]any{"title": in.Title, "body": withLine(in.Description, dueLine, "Due", in.DueDate), "labels": in.Labels}
	if in.Labels == nil {
		body["labels"] = []string{}
	}
	if in.Milestone != "" {
		n, err := t.milestoneNumber(project, in.Milestone)
		if err != nil {
			return tracker.Issue{}, err
		}
		body["milestone"] = n
	}
	b, err := t.API.Do("POST", fmt.Sprintf("repos/%s/issues", project), body)
	if err != nil {
		return tracker.Issue{}, err
	}
	var g ghIssue
	if err := json.Unmarshal(b, &g); err != nil {
		return tracker.Issue{}, err
	}
	return g.view(project), nil
}

func (t *Tracker) UpdateIssue(project string, iid int, u tracker.IssueUpdate) (tracker.Issue, error) {
	cur, err := t.GetIssue(project, iid)
	if err != nil {
		return cur, err
	}
	patch := map[string]any{}
	if u.State != nil {
		switch *u.State {
		case "close":
			patch["state"] = "closed"
		case "reopen":
			patch["state"] = "open"
		default:
			return cur, fmt.Errorf("state %q is neither close nor reopen", *u.State)
		}
	}
	body := cur.Description
	if u.Description != nil {
		body = withLine(*u.Description, dueLine, "Due", cur.DueDate)
	}
	if u.DueDate != nil {
		body = withLine(body, dueLine, "Due", *u.DueDate)
	}
	if body != cur.Description {
		patch["body"] = body
	}
	if u.Milestone != nil {
		if *u.Milestone == "" {
			patch["milestone"] = nil
		} else {
			n, err := t.milestoneNumber(project, *u.Milestone)
			if err != nil {
				return cur, err
			}
			patch["milestone"] = n
		}
	}
	if len(u.AddLabels)+len(u.RemoveLabels) > 0 {
		labels := slices.DeleteFunc(slices.Clone(cur.Labels), func(l string) bool { return slices.Contains(u.RemoveLabels, l) })
		for _, l := range u.AddLabels {
			if !slices.Contains(labels, l) {
				labels = append(labels, l)
			}
		}
		patch["labels"] = append([]string{}, labels...)
	}
	if len(patch) > 0 {
		if _, err := t.API.Do("PATCH", fmt.Sprintf("repos/%s/issues/%d", project, iid), patch); err != nil {
			return cur, err
		}
	}
	return t.GetIssue(project, iid)
}

func (t *Tracker) ListNotes(project string, iid int) ([]tracker.Note, error) {
	var out []tracker.Note
	err := t.all(fmt.Sprintf("repos/%s/issues/%d/comments", project, iid), func(raw json.RawMessage) error {
		var c struct {
			ID   int    `json:"id"`
			Body string `json:"body"`
			User struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		if err := json.Unmarshal(raw, &c); err != nil {
			return err
		}
		out = append(out, tracker.Note{ID: c.ID, Body: c.Body, Author: c.User.Login})
		return nil
	})
	return out, err
}

func (t *Tracker) CreateNote(project string, iid int, body string) error {
	_, err := t.API.Do("POST", fmt.Sprintf("repos/%s/issues/%d/comments", project, iid), map[string]string{"body": body})
	return err
}

type ghMilestone struct {
	Number      int    `json:"number"`
	Title       string `json:"title"`
	Description string `json:"description"`
	DueOn       string `json:"due_on"`
	State       string `json:"state"`
}

func (m ghMilestone) view() tracker.Milestone {
	due := m.DueOn
	if len(due) >= 10 {
		due = due[:10]
	}
	return tracker.Milestone{Title: m.Title, Description: strings.TrimSpace(startLine.ReplaceAllString(m.Description, "")),
		StartDate: lineValue(m.Description, startLine), DueDate: due}
}

func (t *Tracker) milestones(project, state string) ([]ghMilestone, error) {
	var out []ghMilestone
	err := t.all(fmt.Sprintf("repos/%s/milestones?state=%s", project, state), func(raw json.RawMessage) error {
		var m ghMilestone
		if err := json.Unmarshal(raw, &m); err != nil {
			return err
		}
		out = append(out, m)
		return nil
	})
	return out, err
}

// FindMilestone looks in the project's repository: GitHub has no group
// milestones, so a group's is written into each of its repositories.
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
	ms, err := t.milestones(project, "open")
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

// repositories are the repositories a group's labels and milestones go to:
// the group itself when it is a repository, else the routed repositories
// under it, else every repository its owner holds.
func (t *Tracker) repositories(group string) ([]string, error) {
	if strings.Count(group, "/") == 1 {
		return []string{group}, nil
	}
	var out []string
	for _, p := range t.Paths {
		if strings.Count(p, "/") == 1 && strings.HasPrefix(p, group+"/") {
			out = append(out, p)
		}
	}
	if len(out) > 0 {
		return out, nil
	}
	err := t.all("orgs/"+group+"/repos", func(raw json.RawMessage) error {
		var r struct {
			FullName string `json:"full_name"`
			Archived bool   `json:"archived"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		if !r.Archived {
			out = append(out, r.FullName)
		}
		return nil
	})
	return out, err
}

// EnsureMilestone dates the milestone in each repository of the group,
// the pattern a releasing repository outside the program group already
// follows (#124): created when any repository lacked it, else updated when
// any differed.
func (t *Tracker) EnsureMilestone(group string, m tracker.Milestone) (string, error) {
	repos, err := t.repositories(group)
	if err != nil {
		return "", err
	}
	desc := withLine(m.Description, startLine, "Start", m.StartDate)
	want := map[string]any{"title": m.Title, "description": desc}
	if m.DueDate != "" {
		want["due_on"] = m.DueDate + "T00:00:00Z"
	}
	result := tracker.MilestoneUnchanged
	for _, repo := range repos {
		ms, err := t.milestones(repo, "all")
		if err != nil {
			return "", err
		}
		i := slices.IndexFunc(ms, func(x ghMilestone) bool { return x.Title == m.Title })
		switch {
		case i < 0:
			if _, err := t.API.Do("POST", "repos/"+repo+"/milestones", want); err != nil {
				return "", err
			}
			result = tracker.MilestoneCreated
		case ms[i].view() != m:
			if _, err := t.API.Do("PATCH", fmt.Sprintf("repos/%s/milestones/%d", repo, ms[i].Number), want); err != nil {
				return "", err
			}
			if result != tracker.MilestoneCreated {
				result = tracker.MilestoneUpdated
			}
		}
	}
	return result, nil
}

// IssueAwards are the issue's reactions, a 👍 named thumbsup as the peer
// read reads it (srd015 R1.2).
func (t *Tracker) IssueAwards(project string, iid int) ([]tracker.Award, error) {
	var out []tracker.Award
	err := t.all(fmt.Sprintf("repos/%s/issues/%d/reactions", project, iid), func(raw json.RawMessage) error {
		var r struct {
			Content string `json:"content"`
			User    struct {
				Login string `json:"login"`
			} `json:"user"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		name := map[string]string{"+1": "thumbsup", "-1": "thumbsdown"}[r.Content]
		if name == "" {
			name = r.Content
		}
		out = append(out, tracker.Award{Name: name, User: r.User.Login})
		return nil
	})
	return out, err
}

type ghLabel struct {
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

func (t *Tracker) labels(repo string) ([]ghLabel, error) {
	var out []ghLabel
	err := t.all("repos/"+repo+"/labels", func(raw json.RawMessage) error {
		var l ghLabel
		if err := json.Unmarshal(raw, &l); err != nil {
			return err
		}
		out = append(out, l)
		return nil
	})
	return out, err
}

func toTracker(l ghLabel) tracker.Label {
	return tracker.Label{Name: l.Name, Color: "#" + strings.TrimPrefix(l.Color, "#"), Description: l.Description}
}

// GroupLabels are the labels every repository of the group defines: GitHub
// has no group labels, so a label counts once all of them carry it.
func (t *Tracker) GroupLabels(group string) ([]tracker.Label, error) {
	repos, err := t.repositories(group)
	if err != nil || len(repos) == 0 {
		return nil, err
	}
	count := map[string]int{}
	first := map[string]ghLabel{}
	var order []string
	for _, repo := range repos {
		ls, err := t.labels(repo)
		if err != nil {
			return nil, err
		}
		for _, l := range ls {
			if _, seen := first[l.Name]; !seen {
				first[l.Name] = l
				order = append(order, l.Name)
			}
			count[l.Name]++
		}
	}
	var out []tracker.Label
	for _, n := range order {
		if count[n] == len(repos) {
			out = append(out, toTracker(first[n]))
		}
	}
	return out, nil
}

// CreateGroupLabel puts the label in each repository of the group, and
// UpdateGroupLabel makes each carry it as given.
func (t *Tracker) CreateGroupLabel(group string, l tracker.Label) error { return t.putLabel(group, l) }

func (t *Tracker) UpdateGroupLabel(group string, l tracker.Label) error { return t.putLabel(group, l) }

func (t *Tracker) putLabel(group string, l tracker.Label) error {
	repos, err := t.repositories(group)
	if err != nil {
		return err
	}
	want := map[string]string{"name": l.Name, "color": strings.TrimPrefix(l.Color, "#"), "description": l.Description}
	for _, repo := range repos {
		ls, err := t.labels(repo)
		if err != nil {
			return err
		}
		if slices.ContainsFunc(ls, func(x ghLabel) bool { return x.Name == l.Name }) {
			_, err = t.API.Do("PATCH", "repos/"+repo+"/labels/"+url.PathEscape(l.Name), want)
		} else {
			_, err = t.API.Do("POST", "repos/"+repo+"/labels", want)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// CurrentUser is the account gh is logged in as.
func (t *Tracker) CurrentUser() (tracker.User, error) {
	if t.me != nil {
		return *t.me, nil
	}
	var u struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
	}
	if err := t.get("user", &u); err != nil {
		return tracker.User{}, err
	}
	t.me = &tracker.User{ID: u.ID, Username: u.Login}
	return *t.me, nil
}

// AssignIssue makes user the issue's only assignee; pop's rule that only
// the caller is assigned stays scrum-master's (srd041 R9.1).
func (t *Tracker) AssignIssue(project string, iid int, user tracker.User) error {
	_, err := t.API.Do("PATCH", fmt.Sprintf("repos/%s/issues/%d", project, iid), map[string]any{"assignees": []string{user.Username}})
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

// CreateBranch opens the branch from ref with one empty commit of its own,
// since GitHub opens no pull request for a branch with nothing over its
// base, and pop opens the review as it opens the branch.
func (t *Tracker) CreateBranch(project, branch, ref string) error {
	return t.OpenBranch(project, branch, ref, "Open "+branch+" for its pull request")
}

type ghPull struct {
	Number  int    `json:"number"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
	State   string `json:"state"`
	Head    struct {
		SHA string `json:"sha"`
	} `json:"head"`
}

func (t *Tracker) pull(project, branch string) (ghPull, error) {
	owner, _, _ := strings.Cut(project, "/")
	var ps []ghPull
	if err := t.get(fmt.Sprintf("repos/%s/pulls?state=open&head=%s", project, url.QueryEscape(owner+":"+branch)), &ps); err != nil {
		return ghPull{}, err
	}
	if len(ps) == 0 {
		return ghPull{}, fmt.Errorf("no open pull request for %s in %s: %w", branch, project, tracker.ErrNotFound)
	}
	return ps[0], nil
}

func (t *Tracker) OpenReview(project, branch, base, title, body string) (string, error) {
	if p, err := t.pull(project, branch); err == nil {
		return p.HTMLURL, nil
	} else if !errors.Is(err, tracker.ErrNotFound) {
		return "", err
	}
	b, err := t.API.Do("POST", "repos/"+project+"/pulls", map[string]any{"title": title, "head": branch, "base": base, "body": body, "draft": true})
	if err != nil {
		return "", err
	}
	var p ghPull
	return p.HTMLURL, json.Unmarshal(b, &p)
}

type ghReview struct {
	State    string `json:"state"`
	CommitID string `json:"commit_id"`
	User     struct {
		Login string `json:"login"`
	} `json:"user"`
}

func (t *Tracker) reviews(project string, number int) ([]ghReview, error) {
	var out []ghReview
	err := t.all(fmt.Sprintf("repos/%s/pulls/%d/reviews", project, number), func(raw json.RawMessage) error {
		var r ghReview
		if err := json.Unmarshal(raw, &r); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	})
	return out, err
}

func (t *Tracker) Approve(project, branch, commit string) (bool, error) {
	p, err := t.pull(project, branch)
	if err != nil {
		return false, err
	}
	me, err := t.CurrentUser()
	if err != nil {
		return false, err
	}
	rs, err := t.reviews(project, p.Number)
	if err != nil {
		return false, err
	}
	if slices.ContainsFunc(rs, func(r ghReview) bool {
		return r.State == "APPROVED" && r.CommitID == commit && r.User.Login == me.Username
	}) {
		return false, nil
	}
	_, err = t.API.Do("POST", fmt.Sprintf("repos/%s/pulls/%d/reviews", project, p.Number), map[string]string{"commit_id": commit, "event": "APPROVE"})
	return err == nil, err
}

// Approval reads the pull request's reviews in order: a reviewer's latest
// review is what counts, and an approval counts at the commit it names.
func (t *Tracker) Approval(project, branch, tip string, excluded []string) (string, string, error) {
	p, err := t.pull(project, branch)
	if err != nil {
		return "", "", err
	}
	rs, err := t.reviews(project, p.Number)
	if err != nil {
		return "", "", err
	}
	latest := map[string]ghReview{}
	var order []string
	for _, r := range rs {
		if r.State == "COMMENTED" || r.State == "PENDING" {
			continue
		}
		if _, seen := latest[r.User.Login]; !seen {
			order = append(order, r.User.Login)
		}
		latest[r.User.Login] = r
	}
	stale := ""
	for _, who := range order {
		r := latest[who]
		if slices.Contains(excluded, who) || r.State != "APPROVED" {
			continue
		}
		if r.CommitID == tip {
			return who, "", nil
		}
		stale = r.CommitID
	}
	return "", stale, nil
}

// Merge marks the draft ready and merges the pull request at tip with a
// merge commit carrying message's first line as its title and the rest as
// its body, so merged history reads it as scrum-master's own merges.
func (t *Tracker) Merge(project, branch, tip, message string) (string, error) {
	p, err := t.pull(project, branch)
	if err != nil {
		return "", err
	}
	if p.Draft {
		if _, err := t.API.Run("pr", "ready", strconv.Itoa(p.Number), "--repo", project); err != nil {
			return "", err
		}
	}
	title, body, _ := strings.Cut(message, "\n")
	b, err := t.API.Do("PUT", fmt.Sprintf("repos/%s/pulls/%d/merge", project, p.Number),
		map[string]string{"merge_method": "merge", "sha": tip, "commit_title": title, "commit_message": strings.TrimSpace(body)})
	if err != nil {
		return "", err
	}
	var r struct {
		SHA string `json:"sha"`
	}
	return r.SHA, json.Unmarshal(b, &r)
}
