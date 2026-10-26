// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// createsPerWorker is how many issues each worker files at once.
const createsPerWorker = 3

// gitlabWorkload runs the workers at once against the rig's origin and
// asserts what srd042 and eng04 promise under real contention (#57).
func gitlabWorkload(env *gitlabEnv) error {
	c := &check{}
	workers := make([]*gitlabWorker, gitlabWorkers)
	for i := range workers {
		w, err := env.worker(i)
		if err != nil {
			return err
		}
		workers[i] = w
	}
	body := filepath.Join(env.run.Dir, "body.md")
	writeFileTo(body, "Work for the GitLab rig.\n\n## Requirements\n\n1. Do it.\n\n## Acceptance Criteria\n\n- It is done (test: rig; environment: local)\n")

	// Disjoint creates: every worker files its own issues at the same time.
	po1 := env.projectPath("product-owners/po-1")
	reported := map[string]int{}
	var mu sync.Mutex
	parallel(workers, func(w *gitlabWorker) {
		for k := 0; k < createsPerWorker; k++ {
			title := fmt.Sprintf("%s create %d", w.user, k)
			var res program.FileResult
			err := w.wordsJSON(&res, "issue", "file", "--project", po1, "--title", title, "--body-file", body, "--plane", "data", "--kind", "build")
			mu.Lock()
			c.expect(err == nil && res.IID > 0, "%s: file %q: %+v %v", w.user, title, res, err)
			reported[title] = res.IID
			mu.Unlock()
		}
	})
	created, err := readIssues(workers[0], po1)
	if err != nil {
		return err
	}
	byTitle, seen := map[string]int{}, map[int]bool{}
	for _, i := range created {
		c.expect(!seen[i.IID], "%s#%d is numbered twice", po1, i.IID)
		seen[i.IID] = true
		byTitle[i.Title] = i.IID
	}
	c.expect(len(created) == gitlabWorkers*createsPerWorker, "%s holds %d issues, want %d", po1, len(created), gitlabWorkers*createsPerWorker)
	for title, iid := range reported {
		c.expect(byTitle[title] == iid, "%q was reported as #%d and lives as #%d", title, iid, byTitle[title])
	}

	// Contended pops: every worker races for the same issues, one per
	// project, so the merges after them never race on one main.
	operator := workers[0]
	type contended struct {
		key, project string
		iid          int
		winner       *gitlabWorker
		pop          program.PopResult
	}
	var targets []*contended
	for _, key := range []string{"system", "reports", "product-owners/po-2"} {
		project := env.projectPath(key)
		var res program.FileResult
		if err := operator.wordsJSON(&res, "issue", "file", "--project", project, "--title", "Contended in "+path.Base(key), "--body-file", body, "--plane", "management"); err != nil {
			return err
		}
		targets = append(targets, &contended{key: key, project: project, iid: res.IID})
	}
	parallel(workers, func(w *gitlabWorker) {
		for n := range targets {
			t := targets[(n+w.index())%len(targets)]
			var res program.PopResult
			err := w.wordsJSON(&res, "issue", "pop", "--project", t.project, "--issue", fmt.Sprint(t.iid), "--dir", w.checkout(path.Base(t.key)))
			mu.Lock()
			if err == nil && res.Assigned {
				c.expect(t.winner == nil, "%s#%d was taken by both %s and %s", t.project, t.iid, userOf(t.winner), w.user)
				t.winner, t.pop = w, res
			}
			mu.Unlock()
		}
	})
	for _, t := range targets {
		c.expect(t.winner != nil, "%s#%d: nobody took it", t.project, t.iid)
		if t.winner == nil {
			continue
		}
		issues, err := readIssues(operator, t.project)
		if err != nil {
			return err
		}
		for _, i := range issues {
			if i.IID == t.iid {
				c.expect(len(i.Assignees) == 1 && i.Assignees[0] == t.winner.user, "%s#%d assignees %v, want only %s, whose pop took it", t.project, t.iid, i.Assignees, t.winner.user)
			}
		}
	}

	// An offline pop: one worker pops while its tracker is unreachable,
	// another takes the issue online, and reconcile reports the loss.
	offline, online := workers[gitlabWorkers-1], workers[1]
	var x program.FileResult
	if err := operator.wordsJSON(&x, "issue", "file", "--project", po1, "--title", "Taken while offline", "--body-file", body, "--plane", "management"); err != nil {
		return err
	}
	// The worker last synced after the issue was filed; offline, it knows
	// only what it last pulled.
	if _, err := readIssues(offline, po1); err != nil {
		return err
	}
	if err := env.git(offline.trackerClone(), "remote", "set-url", "origin", filepath.Join(env.run.Dir, "unreachable.git")); err != nil {
		return err
	}
	var stale program.PopResult
	err = offline.wordsJSON(&stale, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(x.IID), "--dir", offline.checkout("po-1"))
	c.expect(err == nil && stale.Provisional, "offline pop: %+v %v, want provisional", stale, err)
	var taken program.PopResult
	err = online.wordsJSON(&taken, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(x.IID), "--dir", online.checkout("po-1"))
	c.expect(err == nil && taken.Assigned && !taken.Provisional, "online pop: %+v %v", taken, err)
	if err := env.git(offline.trackerClone(), "remote", "set-url", "origin", env.tracker); err != nil {
		return err
	}
	var rec struct {
		Reconciled []beads.Reconciled `json:"reconciled"`
	}
	err = offline.wordsJSON(&rec, "tracker", "reconcile")
	id := strings.TrimPrefix(x.WebURL, "beads:")
	lost := false
	for _, r := range rec.Reconciled {
		lost = lost || (r.ID == id && strings.HasPrefix(r.Outcome, "lost"))
	}
	c.expect(err == nil && lost, "reconcile: %+v %v, want %s reported lost", rec, err, id)

	// Closes by merge: each winner delivers its issue with another worker's
	// approval, the three merges at once.
	var wg sync.WaitGroup
	for n, t := range targets {
		if t.winner == nil {
			continue
		}
		wg.Add(1)
		go func(n int, t *contended) {
			defer wg.Done()
			err := deliver(env, t.winner, workers[(t.winner.index()+1)%gitlabWorkers], t.key, t.project, t.iid, t.pop)
			mu.Lock()
			c.expect(err == nil, "%s#%d: deliver: %v", t.project, t.iid, err)
			mu.Unlock()
		}(n, t)
	}
	wg.Wait()
	for _, t := range targets {
		issues, _ := readIssues(operator, t.project)
		for _, i := range issues {
			if i.IID == t.iid {
				c.expect(i.State == "closed", "%s#%d is %s after its merge", t.project, t.iid, i.State)
			}
		}
	}

	// Convergence: after a sync, every clone reads the same tracker.
	var first string
	for _, w := range workers {
		state, err := trackerState(env, w)
		if err != nil {
			return err
		}
		if first == "" {
			first = state
			continue
		}
		c.expect(state == first, "%s reads a different tracker than %s:\n%s\nvs\n%s", w.user, workers[0].user, state, first)
	}
	return c.err("gitlab workload")
}

// deliver commits in the winner's worktree, pushes the branch, has the
// reviewer approve it, and merges it with scrum-master issue merge.
func deliver(env *gitlabEnv, winner, reviewer *gitlabWorker, key, project string, iid int, pop program.PopResult) error {
	writeFileTo(filepath.Join(pop.Worktree, "rig", fmt.Sprintf("%d.md", iid)), "Delivered by "+winner.user+".\n")
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", fmt.Sprintf("Deliver #%d", iid)}, {"push", "-q", "-u", "origin", pop.Branch}} {
		if err := env.git(pop.Worktree, append([]string{"-c", "user.name=" + winner.user, "-c", "user.email=" + winner.user + "@example.invalid"}, args...)...); err != nil {
			return err
		}
	}
	dir := reviewer.checkout(path.Base(key))
	if err := env.git(dir, "fetch", "-q", "origin"); err != nil {
		return err
	}
	if _, err := reviewer.run("issue", "approve", "--project", project, "--issue", fmt.Sprint(iid), "--branch", pop.Branch, "--dir", dir); err != nil {
		return err
	}
	_, err := winner.run("issue", "merge", "--project", project, "--issue", fmt.Sprint(iid), "--branch", pop.Branch, "--dir", pop.Worktree)
	return err
}

// readIssues reads every issue of project through the worker's own clone,
// which pulls first, while the worker runs nothing else.
func readIssues(w *gitlabWorker, project string) ([]tracker.Issue, error) {
	lock, _ := w.env.busy.LoadOrStore(w.user, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	defer lock.(*sync.Mutex).Unlock()
	tr := beads.NewTracker(w.trackerClone(), w.user, w.checkout("work"))
	return tr.ListIssues(project, tracker.IssueQuery{State: "all"})
}

// trackerState is every project's issues as one worker's clone reads them,
// in a stable order.
func trackerState(env *gitlabEnv, w *gitlabWorker) (string, error) {
	var lines []string
	for _, key := range env.projects {
		issues, err := readIssues(w, env.projectPath(key))
		if err != nil {
			return "", err
		}
		for _, i := range issues {
			b, _ := json.Marshal(struct {
				Project, Title, State string
				IID                   int
				Assignees             []string
			}{env.projectPath(key), i.Title, i.State, i.IID, i.Assignees})
			lines = append(lines, string(b))
		}
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n"), nil
}

// parallel runs f for every worker at once and waits for all of them.
func parallel(workers []*gitlabWorker, f func(*gitlabWorker)) {
	var wg sync.WaitGroup
	for _, w := range workers {
		wg.Add(1)
		go func(w *gitlabWorker) {
			defer wg.Done()
			f(w)
		}(w)
	}
	wg.Wait()
}

func userOf(w *gitlabWorker) string {
	if w == nil {
		return "nobody"
	}
	return w.user
}

// index is the worker's number, which spreads the workers' orders.
func (w *gitlabWorker) index() int {
	var i int
	fmt.Sscanf(w.user, "worker%d", &i)
	return i
}
