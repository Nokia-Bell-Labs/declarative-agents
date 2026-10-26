// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/gitlab"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Glab runs the opt-in GitLab rig for the glab tracking system: with glab
// logged in, it works in one private project, rig, under the group
// GLAB_RIG_GROUP names, creating the group (as a subgroup of its parent)
// and the project on first use and reusing them after. With
// SCRUM_MASTER_TRACKER=glab it builds scrum-master and proves scrum-master issue labels writes the
// schema as group labels, scrum-master issue file files an admitted issue on a
// dated group milestone, scrum-master issue pop opens the branch with a draft merge
// request, and under the default policy scrum-master issue merge merges the author's
// own merge request and closes the issue. A second person's approval, and
// the refusals of a policy that requires one, are proven offline
// (internal/gitlab), since the rig has one identity. It never joins mage
// regression (srd042, #132).
func (Integration) Glab() error {
	group := strings.Trim(strings.TrimSpace(os.Getenv("GLAB_RIG_GROUP")), "/") //nolint:forbidigo // The opt-in rig reads its tokens and endpoints from the environment (eng01).
	if group == "" {
		return fmt.Errorf("set GLAB_RIG_GROUP to the GitLab group the rig works in")
	}
	api := gitlab.CLI{}
	get := func(p string, v any) error {
		b, err := api.Do("GET", p, nil)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, v)
	}
	var me struct {
		Username string `json:"username"`
	}
	if err := get("user", &me); err != nil {
		return fmt.Errorf("glab is not logged in: %w", err)
	}
	fmt.Printf("glab rig in %s\n", group)
	var g struct {
		ID int `json:"id"`
	}
	if err := get("groups/"+url.PathEscape(group), &g); err != nil {
		var parent struct {
			ID int `json:"id"`
		}
		if err := get("groups/"+url.PathEscape(path.Dir(group)), &parent); err != nil {
			return fmt.Errorf("neither %s nor its parent group exists: %w", group, err)
		}
		b, err := api.Do("POST", "groups", map[string]any{"name": path.Base(group), "path": path.Base(group), "parent_id": parent.ID, "visibility": "private",
			"description": "scrum-master's GitLab rig: disposable issues and merge requests"})
		if err != nil {
			return err
		}
		if err := json.Unmarshal(b, &g); err != nil {
			return err
		}
	}
	project := group + "/rig"
	var p struct {
		ID int `json:"id"`
	}
	if err := get("projects/"+url.PathEscape(project), &p); err != nil {
		if _, err := api.Do("POST", "projects", map[string]any{"name": "rig", "path": "rig", "namespace_id": g.ID, "visibility": "private",
			"initialize_with_readme": true, "default_branch": "main"}); err != nil {
			return err
		}
	}
	root, err := os.MkdirTemp("", "scrum-master-glab-rig-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if root, err = filepath.EvalSymlinks(root); err != nil {
		return err
	}
	bin := filepath.Join(root, "scrum-master")
	if err := runStreaming(".", "go", "build", "-o", bin, "./magefiles/words"); err != nil {
		return err
	}
	clone := filepath.Join(root, "rig")
	if out, err := exec.Command("glab", "repo", "clone", project, clone).CombinedOutput(); err != nil {
		return fmt.Errorf("clone %s: %v: %s", project, err, out)
	}
	for _, args := range [][]string{
		{"config", "credential.helper", ""},
		{"config", "--add", "credential.helper", "!glab auth git-credential"},
	} {
		if err := gitIn(clone, args...); err != nil {
			return err
		}
	}
	words := func(dir string, args ...string) ([]byte, error) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "SCRUM_MASTER_TRACKER=glab")
		var errb bytes.Buffer
		cmd.Stderr = &errb
		out, err := cmd.Output()
		if err != nil {
			return out, fmt.Errorf("scrum-master %s: %w: %s", strings.Join(args, " "), err, errb.String())
		}
		return out, nil
	}
	c := &check{}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	gl := gitlab.New(clone, api)

	if _, err := words(clone, "issue", "labels", "--group", group); err != nil {
		return err
	}
	ls, err := gl.GroupLabels(group)
	if err != nil {
		return err
	}
	c.expect(len(ls) >= len(program.DefaultLabels()), "group labels in %s: %d, want the %d of the schema", group, len(ls), len(program.DefaultLabels()))

	milestone := "rig-" + stamp
	today := time.Now().UTC()
	if _, err := gl.EnsureMilestone(group, tracker.Milestone{Title: milestone, Description: "glab rig run " + stamp,
		StartDate: today.AddDate(0, 0, -1).Format(time.DateOnly), DueDate: today.AddDate(0, 0, 30).Format(time.DateOnly)}); err != nil {
		return err
	}
	body := filepath.Join(root, "issue.md")
	writeFileTo(body, "## Requirements\n\n1. Add the rig note.\n\n## Acceptance Criteria\n\n- The note exists (test: note_exists; environment: local)\n")
	var filed program.FileResult
	out, err := words(clone, "issue", "file", "--project", project, "--title", "Rig run "+stamp, "--body-file", body,
		"--plane", "data", "--kind", "build", "--component", "rig", "--milestone", milestone, "--json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, &filed); err != nil {
		return err
	}
	c.expect(filed.IID > 0 && filed.Lint.Admitted, "file: %+v", filed)

	var popped program.PopResult
	if out, err = words(clone, "issue", "pop", "--project", project, "--issue", fmt.Sprint(filed.IID), "--dir", clone, "--json"); err != nil {
		return err
	}
	if err := json.Unmarshal(out, &popped); err != nil {
		return err
	}
	c.expect(strings.Contains(popped.Review, "/merge_requests/"), "pop opened no merge request: %+v", popped)
	var mrs []struct {
		IID   int  `json:"iid"`
		Draft bool `json:"draft"`
	}
	if err := get(fmt.Sprintf("projects/%s/merge_requests?state=opened&source_branch=%s", url.PathEscape(project), url.QueryEscape(popped.Branch)), &mrs); err != nil {
		return err
	}
	c.expect(len(mrs) == 1 && mrs[0].Draft, "the merge request for %s is not one draft: %+v", popped.Branch, mrs)

	writeFileTo(filepath.Join(popped.Worktree, "RIG.md"), "# Rig run "+stamp+"\n")
	for _, args := range [][]string{{"add", "RIG.md"}, {"commit", "-q", "-m", "Add the rig note"}, {"push", "-q", "-u", "origin", "HEAD"}} {
		if err := gitIn(popped.Worktree, args...); err != nil {
			return err
		}
	}
	// The default policy requires no approval, so the one identity merges
	// its own merge request (srd013 R5.5).
	var merged program.MergeResult
	if out, err = words(popped.Worktree, "issue", "merge", "--project", project, "--issue", fmt.Sprint(filed.IID), "--dir", popped.Worktree, "--json"); err != nil {
		c.expect(false, "merge of the author's own merge request under the default policy: %v", err)
		for _, m := range mrs {
			api.Do("PUT", fmt.Sprintf("projects/%s/merge_requests/%d", url.PathEscape(project), m.IID), map[string]string{"state_event": "close"})
		}
		exec.Command("git", "-C", clone, "push", "-q", "origin", "--delete", popped.Branch).Run()
		closed := "close"
		gl.UpdateIssue(project, filed.IID, tracker.IssueUpdate{State: &closed})
		return c.err("glab")
	}
	if err := json.Unmarshal(out, &merged); err != nil {
		return err
	}
	c.expect(merged.Merged && merged.Commit != "" && merged.ApprovedBy == "", "merge: %+v", merged)
	var after struct {
		State string `json:"state"`
	}
	if len(mrs) == 1 {
		err := get(fmt.Sprintf("projects/%s/merge_requests/%d", url.PathEscape(project), mrs[0].IID), &after)
		c.expect(err == nil && after.State == "merged", "the merge request for %s is %q (%v), want merged", popped.Branch, after.State, err)
	}
	issue, err := gl.GetIssue(project, filed.IID)
	c.expect(err == nil && issue.State == "closed", "issue #%d after its merge: %+v, %v", filed.IID, issue.State, err)
	exec.Command("git", "-C", clone, "push", "-q", "origin", "--delete", popped.Branch).Run()
	return c.err("glab")
}
