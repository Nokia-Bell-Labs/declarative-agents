// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/runtime"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.WorkflowProfiles)
}

// WorkflowProfiles runs the workflows a person ran as tpm commands before
// GH-37 through their profiles, with the repository's agent and neither tpm
// nor scrum-master on PATH: comment, admit, approve (as the reviewer),
// close after a merge that closed nothing, recur, status, labels,
// tracker-sync, tracker-migrate, and tracker-export each end in Done with
// their effect in the tracker or, for the export, the file holding every
// issue; pop opens a chosen branch, and merge closes two other issues as it
// delivers (GH-47); issue-file files a recurring issue, and backlog-list
// and issue-lint run as a model skill's steps would without a model (GH-48);
// specify's and tasks' model steps are sent the plan-vs-architecture guide
// before the request (GH-59)
// (test: workflow profile runs; environment: local).
func (Integration) WorkflowProfiles() error {
	r, err := newRigEnv()
	if err != nil {
		return err
	}
	po1, err := r.project("product-owners/po-1")
	if err != nil {
		return err
	}
	if err := r.milestone(rigMilestone, rigDue); err != nil {
		return err
	}
	if _, err := r.runWords("issue", "labels", "--group", r.group); err != nil {
		return err
	}
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	clone, err := r.clone(po1, "po-1")
	if err != nil {
		return err
	}
	file := func(title string) (program.FileResult, error) {
		path := filepath.Join(r.run.Dir, "wf-"+program.Slug(title)+".md")
		writeFileTo(path, "## Requirements\n\n1. Add the note.\n\n## Acceptance Criteria\n\n- The note exists (test: note_exists; environment: local)\n")
		var res program.FileResult
		return res, r.wordsJSON(&res, "issue", "file", "--project", po1, "--title", title, "--body-file", path,
			"--plane", "data", "--kind", "build", "--component", "po-1", "--milestone", rigMilestone)
	}
	c := &check{}
	run := func(profile, user, dir string, iid int, params map[string]any) string {
		out, err := r.runWorkflow(rel, profile, user, dir, po1, fmt.Sprint(iid), params)
		c.expect(err == nil && strings.Contains(out, "final machine state: Done"), "the %s run must end in Done: %v: %s", profile, err, tail17(out))
		return out
	}

	work, err := file("Workflow profiles")
	if err != nil {
		return err
	}
	body := filepath.Join(r.run.Dir, "wf-comment.md")
	writeFileTo(body, "Picked up for the workflow profiles.\n")
	run("comment", rigUser, clone, work.IID, map[string]any{"body_file": body})
	notes, _ := r.notes(po1, work.IID)
	c.expect(containsNote(notes, "Picked up for the workflow profiles."), "the comment is not on #%d", work.IID)
	run("admit", rigUser, clone, work.IID, map[string]any{"milestone": rigMilestone})

	var popped program.PopResult
	if err := r.wordsJSON(&popped, "issue", "pop", "--project", po1, "--issue", fmt.Sprint(work.IID), "--dir", clone); err != nil {
		return err
	}
	writeFileTo(filepath.Join(popped.Worktree, "NOTE.md"), "a note\n")
	if err := r.commitPush(popped.Worktree, "Add the note", "NOTE.md"); err != nil {
		return err
	}
	reviewerClone, err := r.clone(po1, "reviewer-workflow")
	if err != nil {
		return err
	}
	run("approve", rigReviewer, reviewerClone, work.IID, nil)
	notes, _ = r.notes(po1, work.IID)
	c.expect(containsNote(notes, rigReviewer), "no approval by %s on #%d", rigReviewer, work.IID)

	// merge-branch merges and closes nothing, which the close workflow
	// finishes on the merge commit.
	if _, err := r.runWords("issue", "merge-branch", "--project", po1, "--issue", fmt.Sprint(work.IID), "--branch", popped.Branch,
		"--dir", popped.Worktree, "--audit", rigAudit); err != nil {
		return err
	}
	run("close", rigUser, clone, work.IID, nil)
	c.expect(r.closed(po1, work.IID), "#%d is open after the close workflow", work.IID)

	recurring, err := r.tr.CreateIssue(po1, tracker.NewIssue{Title: "Recurring: workflow audit", Labels: []string{"type::recurring"}, DueDate: rigDue,
		Description: "Audit the workflows.\n\n## Previous Runs\n\n- none\n"})
	if err != nil {
		return err
	}
	run("recur", rigUser, clone, recurring.IID, nil)
	c.expect(r.closed(po1, recurring.IID), "the recurring issue #%d is open after its recur", recurring.IID)

	// GH-47: pop on a chosen branch, then one merge closing two other
	// issues, each given as a request parameter.
	chosen, err := file("Workflow chosen branch")
	if err != nil {
		return err
	}
	run("pop", rigUser, clone, chosen.IID, map[string]any{"branch": "chosen-branch"})
	worktree := ""
	if list, err := gitOut(clone, "worktree", "list", "--porcelain"); err == nil {
		dir := ""
		for _, line := range strings.Split(list, "\n") {
			if d, ok := strings.CutPrefix(line, "worktree "); ok {
				dir = d
			}
			if line == "branch refs/heads/chosen-branch" {
				worktree = dir
			}
		}
	}
	c.expect(worktree != "", "pop with branch chosen-branch left no worktree on it")
	if worktree != "" {
		writeFileTo(filepath.Join(worktree, "CHOSEN.md"), "chosen\n")
		if err := r.commitPush(worktree, "Add the chosen note", "CHOSEN.md"); err != nil {
			return err
		}
		first, err := file("Workflow closed by the merge, first")
		if err != nil {
			return err
		}
		second, err := file("Workflow closed by the merge, second")
		if err != nil {
			return err
		}
		run("merge", rigUser, worktree, chosen.IID, map[string]any{"branch": "chosen-branch",
			"closes": []string{fmt.Sprintf("%s#%d", po1, first.IID), fmt.Sprintf("%s#%d", po1, second.IID)}})
		for _, iid := range []int{chosen.IID, first.IID, second.IID} {
			c.expect(r.closed(po1, iid), "%s#%d is open after the merge that closes it", po1, iid)
		}
	}

	// GH-48: the model-free profiles a model skill's steps name.
	scrumBody := filepath.Join(r.run.Dir, "wf-recurring.md")
	writeFileTo(scrumBody, "Hold the Daily Scrum.\n\n## Recurrence\n\nEvery weekday.\n\n## Previous Runs\n\n- none\n")
	before, _ := r.issues(po1)
	run("issue-file", rigUser, clone, 0, map[string]any{"project": po1, "title": "Recurring: daily scrum", "body_file": scrumBody,
		"type": "recurring", "plane": "management", "milestone": rigMilestone, "due": rigDue})
	after, _ := r.issues(po1)
	recurringIID := 0
	for _, i := range after {
		if i.Title == "Recurring: daily scrum" && slices.Contains(i.Labels, "type::recurring") && i.DueDate == rigDue {
			recurringIID = i.IID
		}
	}
	c.expect(recurringIID != 0 && len(after) == len(before)+1, "issue-file filed no recurring issue with its due date: %d issues before, %d after", len(before), len(after))
	run("backlog-list", rigUser, clone, 0, nil)
	run("issue-lint", rigUser, clone, 0, map[string]any{"project": po1, "issue": recurringIID})

	// GH-59: specify and tasks read the plan-vs-architecture guide, then the
	// request, before their model step: a stand-in model server records the
	// chat each model step sends, which must carry both, and refuses it, so
	// the run stops there.
	for _, profile := range []string{"specify", "tasks"} {
		var chat []byte
		model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			chat, _ = io.ReadAll(req.Body)
			http.Error(w, "the rig's stand-in model answers nothing", http.StatusServiceUnavailable)
		}))
		guided, _ := r.runWorkflow(rel, profile, rigUser, clone, po1, "", map[string]any{"request": "Add the rounding scorer"},
			"OLLAMA_URL="+model.URL)
		model.Close()
		c.expect(strings.Contains(string(chat), "Read the plan-vs-architecture guide below before acting") && strings.Contains(string(chat), "Add the rounding scorer"),
			"the %s model step was not sent the guide and the request: %.300s: %s", profile, chat, tail17(guided))
	}

	status := run("status", rigUser, clone, 0, nil)
	c.expect(strings.Contains(status, po1), "the status run names no %s: %s", po1, tail17(status))
	run("labels", rigUser, clone, 0, map[string]any{"group": r.group})
	run("tracker-sync", rigUser, clone, 0, nil)
	run("tracker-migrate", rigUser, clone, 0, nil)
	export := filepath.Join(r.run.Dir, "wf-export.jsonl")
	run("tracker-export", rigUser, clone, 0, map[string]any{"out": export})
	exported, _ := os.ReadFile(export)
	c.expect(strings.Contains(string(exported), "Workflow profiles") && strings.Contains(string(exported), "Recurring: workflow audit"),
		"the export names not every issue: %s", exported)
	return c.err("workflowProfiles")
}

// runWorkflow runs the repository's agent over one workflow profile as user
// from dir, with the rig's tracker, SCRUM_MASTER_PROJECT and SCRUM_MASTER_ISSUE, the
// request's parameters, any further environment entries, and no tpm or
// scrum-master on PATH.
func (r *rigEnv) runWorkflow(rel runtime.Release, profile, user, dir, project, issue string, params map[string]any, env ...string) (string, error) {
	outDir, err := os.MkdirTemp(r.run.Dir, profile+"-agent-*")
	if err != nil {
		return "", err
	}
	args := []string{"--profile", filepath.Join(r.root, "plugin", "agents", profile, "profile.yaml"),
		"--core-root", rel.CoreRoot(), "--directory", dir, "--output", outDir}
	if len(params) > 0 {
		req, _ := json.Marshal(map[string]any{"parameters": params})
		path := filepath.Join(outDir, "request.json")
		if err := os.WriteFile(path, req, 0o644); err != nil {
			return "", err
		}
		args = append(args, "--request", path)
	}
	cmd := exec.Command(rel.Agent, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PATH="+pathWithout("tpm", "scrum-master"),
		"SCRUM_MASTER_TRACKER="+filepath.Join(r.run.Dir, "tracker.git"),
		"SCRUM_MASTER_TRACKER_CLONE="+filepath.Join(r.run.Dir, "tracker-"+user),
		"SCRUM_MASTER_USER="+user,
		"SCRUM_MASTER_WEB_BASE="+r.webBase,
		"SCRUM_MASTER_PROJECT="+project,
		"SCRUM_MASTER_ISSUE="+issue,
		// A profile that vendors, such as reports-init, vendors this
		// repository's own profiles (GH-68).
		install.SourceEnv+"="+rel.Src,
	)
	cmd.Env = append(cmd.Env, env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	return out.String(), err
}

// containsNote reports whether a note's body or author names text.
func containsNote(notes []tracker.Note, text string) bool {
	for _, n := range notes {
		if strings.Contains(n.Body, text) || n.Author == text {
			return true
		}
	}
	return false
}
