// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.SingleRepo)
}

// singleRepoBranch is the tracker branch of the program of one repository.
const singleRepoBranch = "scrum-master/tracker"

// SingleRepo proves a program of one repository (GH-16, srd042 R3.1,
// srd043 R1.5): the init profile, with the tracker named as the
// repository's own remote and tracker_branch scrum-master/tracker, seeds
// that branch as an orphan and records it in control/trackers.md; an issue
// then files, pops, and merges through the merge profile with code and
// tracker in one repository, its close lands on the tracker branch, and
// main never holds .beads/ (test: a program of one repository keeps its
// tracker on a branch; environment: local). It needs no rig: the
// repository is a bare one on local disk.
func (Integration) SingleRepo() error {
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "scrum-master-single-repo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if tmp, err = filepath.EvalSymlinks(tmp); err != nil {
		return err
	}
	words := filepath.Join(tmp, "scrum-master")
	if err := runStreaming(root, "go", "build", "-o", words, "./magefiles/words"); err != nil {
		return fmt.Errorf("build the words: %w", err)
	}
	origin, repo := filepath.Join(tmp, "solo.git"), filepath.Join(tmp, "solo")
	const user, group, project = "operator", "solo", "solo/app"
	// The program reads its tracker from control/trackers.md alone, so no
	// SCRUM_MASTER_TRACKER reaches the runs.
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "SCRUM_MASTER_TRACKER") && !strings.HasPrefix(kv, "TPM_TRACKER") {
			env = append(env, kv)
		}
	}
	env = append(env, "PATH="+pathWithout("tpm", "scrum-master"), install.SourceEnv+"="+rel.Src,
		"SCRUM_MASTER_USER="+user, "SCRUM_MASTER_TRACKER_CLONE="+filepath.Join(tmp, "tracker-"+user),
		"GIT_AUTHOR_NAME="+user, "GIT_AUTHOR_EMAIL="+user+"@example.invalid",
		"GIT_COMMITTER_NAME="+user, "GIT_COMMITTER_EMAIL="+user+"@example.invalid")
	sh := func(dir, name string, args ...string) (string, error) {
		cmd := exec.Command(name, args...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return string(out), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, out)
		}
		return strings.TrimSpace(string(out)), nil
	}
	agent := func(profile, dir, issue string, params map[string]any) (string, error) {
		args := []string{"--profile", filepath.Join(root, "plugin", "agents", profile, "profile.yaml"),
			"--core-root", rel.CoreRoot(), "--directory", dir, "--output", filepath.Join(tmp, profile+"-run")}
		if params != nil {
			req, _ := json.Marshal(map[string]any{"parameters": params})
			path := filepath.Join(tmp, profile+"-request.json")
			if err := os.WriteFile(path, req, 0o644); err != nil {
				return "", err
			}
			args = append(args, "--request", path)
		}
		cmd := exec.Command(rel.Agent, args...)
		cmd.Dir = dir
		cmd.Env = append(slices.Clone(env), "SCRUM_MASTER_PROJECT="+project, "SCRUM_MASTER_ISSUE="+issue)
		var b bytes.Buffer
		cmd.Stdout, cmd.Stderr = &b, &b
		err := cmd.Run()
		return b.String(), err
	}
	for _, step := range [][]string{
		{tmp, "git", "init", "-q", "--bare", "-b", "main", origin},
		{tmp, "git", "clone", "-q", origin, repo},
		{repo, "git", "commit", "-q", "--allow-empty", "-m", "The program's first commit"},
		{repo, "git", "push", "-q", "-u", "origin", "main"},
	} {
		if _, err := sh(step[0], step[1], step[2:]...); err != nil {
			return err
		}
	}
	first, err := sh(tmp, "git", "--git-dir", origin, "rev-parse", "main")
	if err != nil {
		return err
	}
	c := &check{}

	// The init profile seeds the tracker branch beside the code.
	out, runErr := agent("init", repo, "", map[string]any{"agent": "claude", "tracker": origin, "tracker_branch": singleRepoBranch, "group": group})
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the init run must end in Done: %v: %s", runErr, tail17(out))
	after, _ := sh(tmp, "git", "--git-dir", origin, "rev-parse", "main")
	c.expect(after == first, "the init run moved main from %s to %s", first, after)
	files, _ := sh(tmp, "git", "--git-dir", origin, "ls-tree", "-r", "--name-only", singleRepoBranch)
	c.expect(strings.Contains(files, beads.ConfigFile) && !slices.ContainsFunc(strings.Fields(files), func(f string) bool { return !strings.HasPrefix(f, ".beads/") }),
		"the tracker branch must hold the config file and nothing outside .beads/: %q", files)
	_, shared := sh(tmp, "git", "--git-dir", origin, "merge-base", "main", singleRepoBranch)
	c.expect(shared != nil, "the tracker branch shares history with main")
	doc, err := os.ReadFile(filepath.Join(repo, routing.File))
	cfg, perr := routing.Parse(doc)
	c.expect(err == nil && perr == nil && cfg.Default.TrackerRepository == origin && cfg.Default.TrackerBranch == singleRepoBranch,
		"%s must name the repository's own remote on %s: %+v %v %v", routing.File, singleRepoBranch, cfg.Default, err, perr)
	if len(c.failures) > 0 {
		return c.err("singleRepo")
	}
	for _, args := range [][]string{{"add", "-A"}, {"commit", "-q", "-m", "Lay down the program"}, {"push", "-q", "origin", "main"}} {
		if _, err := sh(repo, "git", args...); err != nil {
			return err
		}
	}

	// The program's milestone, written to the tracker branch as any
	// member's clone would.
	clone := filepath.Join(tmp, "milestone-clone")
	if _, err := sh(tmp, "git", "clone", "-q", "--single-branch", "--branch", singleRepoBranch, origin, clone); err != nil {
		return err
	}
	tr := beads.NewTracker(clone, user, repo)
	tr.Store.Branch = singleRepoBranch
	if _, err := tr.EnsureMilestone(group, tracker.Milestone{Title: rigMilestone,
		StartDate: time.Now().AddDate(0, 0, -1).Format(time.DateOnly), DueDate: rigDue}); err != nil {
		return err
	}

	// File and pop an issue, and merge its branch through the merge profile.
	body := filepath.Join(tmp, "issue.md")
	writeFileTo(body, "## Requirements\n\n1. Add the note.\n\n## Acceptance Criteria\n\n- The note exists (test: note_exists; environment: local)\n")
	filed, err := sh(repo, words, "issue", "file", "--project", project, "--title", "Add the note", "--body-file", body,
		"--plane", "data", "--kind", "build", "--component", "solo", "--milestone", rigMilestone, "--json")
	if err != nil {
		return err
	}
	var fr program.FileResult
	if err := json.Unmarshal([]byte(filed), &fr); err != nil {
		return fmt.Errorf("issue file: %w: %s", err, filed)
	}
	popped, err := sh(repo, words, "issue", "pop", "--project", project, "--issue", fmt.Sprint(fr.IID), "--dir", repo, "--json")
	if err != nil {
		return err
	}
	var pr program.PopResult
	for dec := json.NewDecoder(strings.NewReader(popped)); ; {
		var p program.PopResult
		if dec.Decode(&p) != nil {
			break
		}
		if p.Worktree != "" {
			pr = p
		}
	}
	if pr.Worktree == "" {
		return fmt.Errorf("issue pop named no worktree: %s", popped)
	}
	writeFileTo(filepath.Join(pr.Worktree, "NOTE.md"), "# The note\n")
	for _, args := range [][]string{{"add", "NOTE.md"}, {"commit", "-q", "-m", "Add the note"}, {"push", "-q", "-u", "origin", "HEAD"}} {
		if _, err := sh(pr.Worktree, "git", args...); err != nil {
			return err
		}
	}
	out, runErr = agent("merge", pr.Worktree, fmt.Sprint(fr.IID), nil)
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the merge run must end in Done: %v: %s", runErr, tail17(out))

	// The code merged into main, the close landed on the tracker branch, and
	// main holds no tracker file.
	note, err := sh(tmp, "git", "--git-dir", origin, "show", "main:NOTE.md")
	c.expect(err == nil && strings.Contains(note, "The note"), "main does not hold the merged note: %v", err)
	mainFiles, _ := sh(tmp, "git", "--git-dir", origin, "ls-tree", "-r", "--name-only", "main")
	c.expect(!strings.Contains(mainFiles, ".beads/"), "main holds tracker files:\n%s", mainFiles)
	_, shared = sh(tmp, "git", "--git-dir", origin, "merge-base", "main", singleRepoBranch)
	c.expect(shared != nil, "the tracker branch and main share history after the merge")
	i, err := tr.GetIssue(project, fr.IID)
	c.expect(err == nil && i.State == "closed", "the merge must close %s#%d on the tracker branch: %+v %v", project, fr.IID, i.State, err)
	return c.err("singleRepo")
}
