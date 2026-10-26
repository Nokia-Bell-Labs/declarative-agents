// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package github

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// hub is a GitHub in memory, enough of its REST API for the tracker: each
// repository's issues, comments, reactions, labels, milestones, pull
// requests and reviews. A pull request merged here is merged in its
// repository's git origin, when the test names one, so the local checkout
// reads the merge as it would after a real one.
type hub struct {
	mu      sync.Mutex
	repos   map[string]*repo
	orgs    map[string][]string
	origins map[string]string // repository -> bare git origin
}

type repo struct {
	next       int
	issues     map[int]map[string]any
	comments   map[int][]map[string]any
	reactions  map[int][]map[string]any
	labels     []map[string]any
	milestones []map[string]any
	pulls      map[int]map[string]any
	reviews    map[int][]map[string]any
}

func newHub() *hub {
	return &hub{repos: map[string]*repo{}, orgs: map[string][]string{}, origins: map[string]string{}}
}

func (h *hub) repo(name string) *repo {
	r := h.repos[name]
	if r == nil {
		r = &repo{next: 1, issues: map[int]map[string]any{}, comments: map[int][]map[string]any{}, reactions: map[int][]map[string]any{},
			pulls: map[int]map[string]any{}, reviews: map[int][]map[string]any{}}
		h.repos[name] = r
		owner, _, _ := strings.Cut(name, "/")
		h.orgs[owner] = append(h.orgs[owner], name)
	}
	return r
}

// as is the hub as one GitHub login sees it.
func (h *hub) as(login string) API { return client{h, login} }

type client struct {
	h     *hub
	login string
}

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return fmt.Sprintf("gh: %s (HTTP %d)", e.msg, e.code) }

func notFound() error { return httpError{404, "Not Found"} }

func (c client) Run(args ...string) ([]byte, error) {
	c.h.mu.Lock()
	defer c.h.mu.Unlock()
	if len(args) >= 5 && args[0] == "pr" && args[1] == "ready" && args[3] == "--repo" {
		n, _ := strconv.Atoi(args[2])
		p := c.h.repo(args[4]).pulls[n]
		if p == nil {
			return nil, notFound()
		}
		p["draft"] = false
		return nil, nil
	}
	return nil, fmt.Errorf("fake gh: no command %v", args)
}

func (c client) Do(method, raw string, body any) ([]byte, error) {
	c.h.mu.Lock()
	defer c.h.mu.Unlock()
	var in map[string]any
	if body != nil {
		b, _ := json.Marshal(body)
		json.Unmarshal(b, &in)
	}
	path, query, _ := strings.Cut(raw, "?")
	q, _ := url.ParseQuery(query)
	if page := q.Get("page"); page != "" && page != "1" {
		return []byte("[]"), nil
	}
	seg := strings.Split(path, "/")
	out, err := c.route(method, seg, q, in)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

func (c client) user() map[string]any { return map[string]any{"login": c.login} }

func (c client) route(method string, seg []string, q url.Values, in map[string]any) (any, error) {
	switch {
	case method == "GET" && len(seg) == 1 && seg[0] == "user":
		return map[string]any{"id": 1, "login": c.login}, nil
	case method == "GET" && len(seg) == 3 && seg[0] == "orgs" && seg[2] == "repos":
		var out []map[string]any
		for _, r := range c.h.orgs[seg[1]] {
			out = append(out, map[string]any{"full_name": r})
		}
		return out, nil
	case len(seg) < 4 || seg[0] != "repos":
		return nil, notFound()
	}
	name := seg[1] + "/" + seg[2]
	r := c.h.repo(name)
	rest := seg[3:]
	num := func(i int) int { n, _ := strconv.Atoi(rest[i]); return n }
	switch {
	case rest[0] == "issues" && len(rest) == 1 && method == "GET":
		var out []map[string]any
		for _, n := range sortedKeys(r.issues) {
			i := r.issues[n]
			if st := q.Get("state"); st != "all" && i["state"] != st {
				continue
			}
			if ls := q.Get("labels"); ls != "" && !hasLabels(i, strings.Split(ls, ",")) {
				continue
			}
			out = append(out, i)
		}
		return out, nil
	case rest[0] == "issues" && len(rest) == 1 && method == "POST":
		n := r.next
		r.next++
		i := map[string]any{"number": n, "title": in["title"], "body": in["body"], "state": "open", "user": c.user(),
			"html_url": fmt.Sprintf("https://github.com/%s/issues/%d", name, n), "labels": labelObjs(in["labels"]), "assignees": []any{}}
		if m, ok := in["milestone"]; ok && m != nil {
			i["milestone"] = r.milestoneByNumber(int(m.(float64)))
		}
		r.issues[n] = i
		return i, nil
	case rest[0] == "issues" && len(rest) == 2:
		i := r.issues[num(1)]
		if i == nil {
			return nil, notFound()
		}
		if method == "PATCH" {
			for k, v := range in {
				switch k {
				case "labels":
					i["labels"] = labelObjs(v)
				case "assignees":
					var as []any
					for _, a := range v.([]any) {
						as = append(as, map[string]any{"login": a})
					}
					i["assignees"] = as
				case "milestone":
					if v == nil {
						delete(i, "milestone")
					} else {
						i["milestone"] = r.milestoneByNumber(int(v.(float64)))
					}
				case "state":
					// GitHub stamps closed_at as an issue closes (GH-82).
					i[k], i["closed_at"] = v, map[any]any{"closed": "2026-11-06T10:00:00Z"}[v]
				default:
					i[k] = v
				}
			}
		}
		return i, nil
	case rest[0] == "issues" && len(rest) == 3 && rest[2] == "comments":
		n := num(1)
		if method == "POST" {
			cm := map[string]any{"id": len(r.comments[n]) + 1, "body": in["body"], "user": c.user()}
			r.comments[n] = append(r.comments[n], cm)
			return cm, nil
		}
		return orEmpty(r.comments[n]), nil
	case rest[0] == "issues" && len(rest) == 3 && rest[2] == "reactions":
		return orEmpty(r.reactions[num(1)]), nil
	case rest[0] == "milestones" && len(rest) == 1 && method == "GET":
		var out []map[string]any
		for _, m := range r.milestones {
			if st := q.Get("state"); st == "all" || m["state"] == st {
				out = append(out, m)
			}
		}
		return out, nil
	case rest[0] == "milestones" && len(rest) == 1 && method == "POST":
		m := map[string]any{"number": len(r.milestones) + 1, "title": in["title"], "description": in["description"], "due_on": in["due_on"], "state": "open"}
		r.milestones = append(r.milestones, m)
		return m, nil
	case rest[0] == "milestones" && len(rest) == 2 && method == "PATCH":
		m := r.milestoneByNumber(num(1))
		for k, v := range in {
			m[k] = v
		}
		return m, nil
	case rest[0] == "labels" && len(rest) == 1 && method == "GET":
		return orEmpty(r.labels), nil
	case rest[0] == "labels" && len(rest) == 1 && method == "POST":
		if slices.ContainsFunc(r.labels, func(l map[string]any) bool { return l["name"] == in["name"] }) {
			return nil, httpError{422, "already_exists"}
		}
		r.labels = append(r.labels, in)
		return in, nil
	case rest[0] == "labels" && len(rest) == 2 && method == "PATCH":
		name, _ := url.PathUnescape(rest[1])
		for _, l := range r.labels {
			if l["name"] == name {
				for k, v := range in {
					l[k] = v
				}
				return l, nil
			}
		}
		return nil, notFound()
	case rest[0] == "pulls" && len(rest) == 1 && method == "GET":
		var out []map[string]any
		_, head, _ := strings.Cut(q.Get("head"), ":")
		for _, n := range sortedKeys(r.pulls) {
			if p := r.pulls[n]; p["state"] == "open" && p["head_ref"] == head {
				out = append(out, p)
			}
		}
		return out, nil
	case rest[0] == "pulls" && len(rest) == 1 && method == "POST":
		n := r.next
		r.next++
		p := map[string]any{"number": n, "state": "open", "draft": in["draft"], "head_ref": in["head"], "base_ref": in["base"],
			"author": c.login, "html_url": fmt.Sprintf("https://github.com/%s/pull/%d", name, n)}
		r.pulls[n] = p
		return p, nil
	case rest[0] == "pulls" && len(rest) == 3 && rest[2] == "reviews":
		n := num(1)
		p := r.pulls[n]
		if p == nil {
			return nil, notFound()
		}
		if method == "POST" {
			if p["author"] == c.login {
				return nil, httpError{422, "Can not approve your own pull request"}
			}
			rv := map[string]any{"state": "APPROVED", "commit_id": in["commit_id"], "user": c.user()}
			r.reviews[n] = append(r.reviews[n], rv)
			return rv, nil
		}
		return orEmpty(r.reviews[n]), nil
	case rest[0] == "pulls" && len(rest) == 3 && rest[2] == "merge" && method == "PUT":
		p := r.pulls[num(1)]
		if p == nil {
			return nil, notFound()
		}
		if p["draft"] == true {
			return nil, httpError{405, "Pull Request is still a draft"}
		}
		sha, err := c.h.mergeInOrigin(name, p["head_ref"].(string), p["base_ref"].(string), in["sha"].(string), in["commit_title"].(string)+"\n\n"+in["commit_message"].(string))
		if err != nil {
			return nil, err
		}
		p["state"] = "closed"
		return map[string]any{"sha": sha, "merged": true}, nil
	}
	return nil, notFound()
}

// mergeInOrigin merges head into base in the repository's origin as GitHub
// would, refusing a head that moved past sha.
func (h *hub) mergeInOrigin(name, head, base, sha, message string) (string, error) {
	origin := h.origins[name]
	if origin == "" {
		return "fake-merge-" + sha, nil
	}
	dir, err := os.MkdirTemp("", "fake-hub-merge-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	git := func(args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-c", "user.name=GitHub", "-c", "user.email=noreply@github.com"}, args...)...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	if _, err := git("clone", "-q", origin, "."); err != nil {
		return "", err
	}
	if tip, err := git("rev-parse", "origin/"+head); err != nil {
		return "", err
	} else if tip != sha {
		return "", httpError{409, "Head branch was modified. Review and try the merge again."}
	}
	if _, err := git("checkout", "-q", base); err != nil {
		return "", err
	}
	if _, err := git("merge", "-q", "--no-ff", "origin/"+head, "-m", message); err != nil {
		return "", err
	}
	if _, err := git("push", "-q", "origin", base); err != nil {
		return "", err
	}
	return git("rev-parse", "HEAD")
}

func (r *repo) milestoneByNumber(n int) map[string]any {
	for _, m := range r.milestones {
		if int(m["number"].(int)) == n {
			return m
		}
	}
	return nil
}

func labelObjs(v any) []any {
	out := []any{}
	if list, ok := v.([]any); ok {
		for _, l := range list {
			out = append(out, map[string]any{"name": l})
		}
	}
	return out
}

func hasLabels(i map[string]any, want []string) bool {
	var have []string
	for _, l := range i["labels"].([]any) {
		have = append(have, l.(map[string]any)["name"].(string))
	}
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
