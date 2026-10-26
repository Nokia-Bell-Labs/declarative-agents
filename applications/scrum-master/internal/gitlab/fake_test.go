// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package gitlab

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// The fake finishes its merge check at once, so a retried merge need not
// wait.
func init() { mergeWait = 0 }

// lab is a GitLab in memory, enough of its REST API v4 for the tracker:
// projects' issues, notes, award emoji and merge requests with approvals
// and notes, and groups' milestones and labels. A merge request merged here
// is merged in its project's git origin, when the test names one.
type lab struct {
	mu       sync.Mutex
	projects map[string]*project
	groups   map[string]*group
	users    map[string]int
	origins  map[string]string
}

type project struct {
	issues     map[int]map[string]any
	notes      map[int][]map[string]any
	awards     map[int][]map[string]any
	milestones []map[string]any
	mrs        map[int]map[string]any
	mrNotes    map[int][]map[string]any
	approvals  map[int][]string
	nextIssue  int
	nextMR     int
}

type group struct {
	milestones []map[string]any
	labels     []map[string]any
}

var nextMilestone = 100

func newLab() *lab {
	return &lab{projects: map[string]*project{}, groups: map[string]*group{}, users: map[string]int{}, origins: map[string]string{}}
}

func (l *lab) project(p string) *project {
	if l.projects[p] == nil {
		l.projects[p] = &project{issues: map[int]map[string]any{}, notes: map[int][]map[string]any{}, awards: map[int][]map[string]any{},
			mrs: map[int]map[string]any{}, mrNotes: map[int][]map[string]any{}, approvals: map[int][]string{}, nextIssue: 1, nextMR: 1}
	}
	return l.projects[p]
}

func (l *lab) group(g string) *group {
	if l.groups[g] == nil {
		l.groups[g] = &group{}
	}
	return l.groups[g]
}

func (l *lab) as(login string) API {
	if _, ok := l.users[login]; !ok {
		l.users[login] = len(l.users) + 1
	}
	return labClient{l, login}
}

type labClient struct {
	l     *lab
	login string
}

type apiError struct {
	code int
	msg  string
}

func (e apiError) Error() string { return fmt.Sprintf("glab: %s (HTTP %d)", e.msg, e.code) }

func (c labClient) user() map[string]any {
	return map[string]any{"id": c.l.users[c.login], "username": c.login}
}

func (c labClient) Run(args ...string) ([]byte, error) {
	c.l.mu.Lock()
	defer c.l.mu.Unlock()
	if len(args) == 6 && args[0] == "mr" && args[1] == "update" && args[3] == "--ready" && args[4] == "--repo" {
		n, _ := strconv.Atoi(args[2])
		m := c.l.project(args[5]).mrs[n]
		if m == nil {
			return nil, apiError{404, "Not Found"}
		}
		m["draft"] = false
		m["title"] = strings.TrimPrefix(m["title"].(string), "Draft: ")
		// GitLab checks a merge request that left draft before it lets
		// one merge, and refuses a merge meanwhile.
		m["checks"] = 1
		return nil, nil
	}
	return nil, fmt.Errorf("fake glab: no command %v", args)
}

func (c labClient) Do(method, raw string, body any) ([]byte, error) {
	c.l.mu.Lock()
	defer c.l.mu.Unlock()
	var in map[string]any
	if body != nil {
		b, _ := json.Marshal(body)
		json.Unmarshal(b, &in)
	}
	p, query, _ := strings.Cut(raw, "?")
	q, _ := url.ParseQuery(query)
	if page := q.Get("page"); page != "" && page != "1" {
		return []byte("[]"), nil
	}
	seg := strings.Split(p, "/")
	for i := range seg {
		seg[i], _ = url.PathUnescape(seg[i])
	}
	out, err := c.route(method, seg, q, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (c labClient) route(method string, seg []string, q url.Values, in map[string]any) (any, error) {
	if len(seg) == 1 && seg[0] == "user" {
		return c.user(), nil
	}
	if len(seg) == 1 && seg[0] == "users" {
		if uid, ok := c.l.users[q.Get("username")]; ok {
			return []map[string]any{{"id": uid, "username": q.Get("username")}}, nil
		}
		return []map[string]any{}, nil
	}
	if len(seg) >= 3 && seg[0] == "groups" {
		g := c.l.group(seg[1])
		switch {
		case seg[2] == "milestones" && len(seg) == 3 && method == "GET":
			var out []map[string]any
			for _, m := range g.milestones {
				if t := q.Get("title"); t != "" && m["title"] != t {
					continue
				}
				if st := q.Get("state"); st != "" && m["state"] != st {
					continue
				}
				out = append(out, m)
			}
			return orEmpty(out), nil
		case seg[2] == "milestones" && len(seg) == 3 && method == "POST":
			nextMilestone++
			m := map[string]any{"id": nextMilestone, "title": in["title"], "description": in["description"],
				"start_date": in["start_date"], "due_date": in["due_date"], "state": "active"}
			g.milestones = append(g.milestones, m)
			return m, nil
		case seg[2] == "milestones" && len(seg) == 4 && method == "PUT":
			n, _ := strconv.Atoi(seg[3])
			for _, m := range g.milestones {
				if m["id"].(int) == n {
					for k, v := range in {
						m[k] = v
					}
					return m, nil
				}
			}
		case seg[2] == "labels" && len(seg) == 3 && method == "GET":
			return orEmpty(g.labels), nil
		case seg[2] == "labels" && len(seg) == 3 && method == "POST":
			g.labels = append(g.labels, in)
			return in, nil
		case seg[2] == "labels" && len(seg) == 4 && method == "PUT":
			for _, l := range g.labels {
				if l["name"] == seg[3] {
					for k, v := range in {
						l[k] = v
					}
					return l, nil
				}
			}
		}
		return nil, apiError{404, "Not Found"}
	}
	if len(seg) < 3 || seg[0] != "projects" {
		return nil, apiError{404, "Not Found"}
	}
	name := seg[1]
	pr := c.l.project(name)
	rest := seg[2:]
	num := func(i int) int { n, _ := strconv.Atoi(rest[i]); return n }
	switch {
	case rest[0] == "issues" && len(rest) == 1 && method == "GET":
		var out []map[string]any
		for _, n := range sortedKeys(pr.issues) {
			i := pr.issues[n]
			if st := q.Get("state"); st != "all" && i["state"] != st {
				continue
			}
			if ls := q.Get("labels"); ls != "" && !hasAll(i["labels"], strings.Split(ls, ",")) {
				continue
			}
			if s := strings.ToLower(q.Get("search")); s != "" && !strings.Contains(strings.ToLower(fmt.Sprint(i["title"], i["description"])), s) {
				continue
			}
			out = append(out, i)
		}
		return orEmpty(out), nil
	case rest[0] == "issues" && len(rest) == 1 && method == "POST":
		n := pr.nextIssue
		pr.nextIssue++
		i := map[string]any{"iid": n, "title": in["title"], "description": in["description"], "state": "opened", "author": c.user(),
			"web_url": fmt.Sprintf("https://gitlab.com/%s/-/issues/%d", name, n), "labels": splitLabels(in["labels"]), "assignees": []any{},
			"due_date": in["due_date"]}
		if mid, ok := in["milestone_id"]; ok {
			i["milestone"] = c.l.milestoneByID(name, int(mid.(float64)))
		}
		pr.issues[n] = i
		return i, nil
	case rest[0] == "issues" && len(rest) == 2:
		i := pr.issues[num(1)]
		if i == nil {
			return nil, apiError{404, "Not Found"}
		}
		if method == "PUT" {
			for k, v := range in {
				switch k {
				case "state_event":
					i["state"] = map[string]string{"close": "closed", "reopen": "opened"}[v.(string)]
					// GitLab stamps closed_at as an issue closes (GH-82).
					i["closed_at"] = map[string]any{"close": "2026-11-06T10:00:00Z", "reopen": nil}[v.(string)]
				case "add_labels":
					ls := splitLabels(i["labels"])
					for _, l := range splitLabels(v) {
						if !slices.Contains(ls, l) {
							ls = append(ls, l)
						}
					}
					i["labels"] = ls
				case "remove_labels":
					drop := splitLabels(v)
					i["labels"] = slices.DeleteFunc(splitLabels(i["labels"]), func(l string) bool { return slices.Contains(drop, l) })
				case "assignee_ids":
					var as []any
					for _, uid := range v.([]any) {
						for login, id := range c.l.users {
							if float64(id) == uid.(float64) {
								as = append(as, map[string]any{"id": id, "username": login})
							}
						}
					}
					i["assignees"] = as
				case "milestone_id":
					if v.(float64) == 0 {
						delete(i, "milestone")
					} else {
						i["milestone"] = c.l.milestoneByID(name, int(v.(float64)))
					}
				default:
					i[k] = v
				}
			}
		}
		return i, nil
	case rest[0] == "issues" && len(rest) == 3 && rest[2] == "notes":
		n := num(1)
		if method == "POST" {
			note := map[string]any{"id": len(pr.notes[n]) + 1, "body": in["body"], "author": c.user(), "system": false}
			pr.notes[n] = append(pr.notes[n], note)
			return note, nil
		}
		return orEmpty(pr.notes[n]), nil
	case rest[0] == "issues" && len(rest) == 3 && rest[2] == "award_emoji":
		return orEmpty(pr.awards[num(1)]), nil
	case rest[0] == "milestones" && method == "GET":
		return orEmpty(pr.milestones), nil
	case rest[0] == "merge_requests" && len(rest) == 1 && method == "GET":
		var out []map[string]any
		for _, n := range sortedKeys(pr.mrs) {
			if m := pr.mrs[n]; m["state"] == "opened" && m["source_branch"] == q.Get("source_branch") {
				out = append(out, m)
			}
		}
		return orEmpty(out), nil
	case rest[0] == "merge_requests" && len(rest) == 1 && method == "POST":
		n := pr.nextMR
		pr.nextMR++
		m := map[string]any{"iid": n, "state": "opened", "title": in["title"], "draft": strings.HasPrefix(fmt.Sprint(in["title"]), "Draft: "),
			"source_branch": in["source_branch"], "target_branch": in["target_branch"], "author": c.login, "checks": 0,
			"web_url": fmt.Sprintf("https://gitlab.com/%s/-/merge_requests/%d", name, n)}
		pr.mrs[n] = m
		return m, nil
	case rest[0] == "merge_requests" && len(rest) == 2 && method == "GET":
		m := pr.mrs[num(1)]
		if m == nil {
			return nil, apiError{404, "Not Found"}
		}
		// Reading it while it checks ends the check.
		status := "mergeable"
		if m["checks"].(int) > 0 {
			status = "checking"
			m["checks"] = m["checks"].(int) - 1
		}
		return map[string]any{"iid": m["iid"], "detailed_merge_status": status}, nil
	case rest[0] == "merge_requests" && len(rest) == 3:
		n := num(1)
		m := pr.mrs[n]
		if m == nil {
			return nil, apiError{404, "Not Found"}
		}
		switch {
		case rest[2] == "notes" && method == "POST":
			note := map[string]any{"body": in["body"], "author": c.user()}
			pr.mrNotes[n] = append(pr.mrNotes[n], note)
			return note, nil
		case rest[2] == "notes":
			return orEmpty(pr.mrNotes[n]), nil
		case rest[2] == "approve" && method == "POST":
			head, err := c.l.head(name, m["source_branch"].(string))
			if err != nil {
				return nil, err
			}
			if in["sha"] != head {
				return nil, apiError{409, "SHA does not match HEAD of source branch"}
			}
			pr.approvals[n] = append(pr.approvals[n], c.login)
			return map[string]any{}, nil
		case rest[2] == "approvals":
			var by []map[string]any
			for _, who := range pr.approvals[n] {
				by = append(by, map[string]any{"user": map[string]any{"username": who}})
			}
			return map[string]any{"approved_by": orEmpty(by)}, nil
		case rest[2] == "merge" && method == "PUT":
			if m["draft"] == true {
				return nil, apiError{405, "Method Not Allowed: draft"}
			}
			if m["checks"].(int) > 0 {
				return nil, apiError{405, "Method Not Allowed"}
			}
			sha, err := c.l.mergeInOrigin(name, m["source_branch"].(string), m["target_branch"].(string), in["sha"].(string), in["merge_commit_message"].(string))
			if err != nil {
				return nil, err
			}
			m["state"], m["merge_commit_sha"] = "merged", sha
			return m, nil
		}
	}
	return nil, apiError{404, "Not Found"}
}

func (l *lab) milestoneByID(projectPath string, mid int) map[string]any {
	for p := path.Dir(projectPath); p != "."; p = path.Dir(p) {
		for _, m := range l.group(p).milestones {
			if m["id"].(int) == mid {
				return m
			}
		}
	}
	return nil
}

func (l *lab) head(name, branch string) (string, error) {
	origin := l.origins[name]
	if origin == "" {
		return "", fmt.Errorf("no origin for %s", name)
	}
	out, err := exec.Command("git", "--git-dir", origin, "rev-parse", "refs/heads/"+branch).Output()
	return strings.TrimSpace(string(out)), err
}

func (l *lab) mergeInOrigin(name, source, target, sha, message string) (string, error) {
	if head, err := l.head(name, source); err != nil {
		return "", err
	} else if head != sha {
		return "", apiError{409, "SHA does not match HEAD of source branch"}
	}
	dir, err := os.MkdirTemp("", "fake-lab-merge-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-c", "user.name=GitLab", "-c", "user.email=noreply@gitlab.com"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	for _, args := range [][]string{{"clone", "-q", l.origins[name], "."}, {"checkout", "-q", target}, {"merge", "-q", "--no-ff", "origin/" + source, "-m", message}, {"push", "-q", "origin", target}} {
		if _, err := git(args...); err != nil {
			return "", err
		}
	}
	return git("rev-parse", "HEAD")
}

func splitLabels(v any) []string {
	switch x := v.(type) {
	case string:
		if x == "" {
			return []string{}
		}
		return strings.Split(x, ",")
	case []string:
		return x
	case []any:
		var out []string
		for _, s := range x {
			out = append(out, s.(string))
		}
		return out
	}
	return []string{}
}

func hasAll(v any, want []string) bool {
	have := splitLabels(v)
	for _, w := range want {
		if !slices.Contains(have, w) {
			return false
		}
	}
	return true
}

func orEmpty(v []map[string]any) []map[string]any {
	if v == nil {
		return []map[string]any{}
	}
	return v
}

func sortedKeys[V any](m map[int]V) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
