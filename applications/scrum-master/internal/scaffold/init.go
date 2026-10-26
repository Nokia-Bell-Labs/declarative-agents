// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
)

// InitOptions are what scrum-master init is given.
type InitOptions struct {
	Dir     string
	Agents  []string // agents to install the skills for; none installs none
	Version string   // scrum-master version stamped into installed skills
	// Tracker is a tracker repository to seed; Group is the program group
	// whose labels are created there.
	Tracker, Group string
	// TrackerBranch is the tracker repository's branch holding .beads/,
	// main when empty; a program of one repository names a dedicated
	// branch of its own repository, seeded as an orphan (srd043 R1.5).
	TrackerBranch string
	// Prefix is the tracker's issue ID prefix; empty derives it from Group.
	Prefix        string
	User          string // the person the tracker's writes are made as
	Force, DryRun bool
}

// InitResult is what scrum-master init reports.
type InitResult struct {
	Dir     string           `json:"dir"`
	Files   []Result         `json:"files"`
	Skills  []install.Result `json:"skills,omitempty"`
	Tracker *TrackerResult   `json:"tracker,omitempty"`
	// Group is the program group recorded in control/trackers.md;
	// GroupReason says why --group was not recorded, when it was not
	// (GH-103).
	Group       string `json:"group,omitempty"`
	GroupReason string `json:"group_reason,omitempty"`
	DryRun      bool   `json:"dry_run"`
}

// TrackerResult is what init did to the tracker repository (srd043 R1.5).
type TrackerResult struct {
	Remote string                `json:"remote"`
	Branch string                `json:"branch,omitempty"`
	Seeded bool                  `json:"seeded"`
	Prefix string                `json:"prefix,omitempty"` // the issue ID prefix the first commit set
	Reason string                `json:"reason,omitempty"`
	Labels []program.LabelResult `json:"labels,omitempty"`
}

// Init lays down a program's system repository in o.Dir: the corpus
// skeleton, the control files, the skills for o.Agents, and, with o.Tracker,
// the tracker repository's first commit and the program's labels (srd043 R1).
func Init(o InitOptions) (InitResult, error) {
	r := InitResult{Dir: o.Dir, DryRun: o.DryRun}
	if err := requireRepository(o.Dir); err != nil {
		return r, err
	}
	if o.Tracker != "" && o.Group == "" {
		return r, errors.New("--tracker needs --group, the program group whose labels it creates")
	}
	files, err := corpusFiles("system", o.Dir)
	if err != nil {
		return r, err
	}
	for n, b := range controlFiles() {
		files[n] = b
	}
	// The program's tracking system, when init knows it: the tracker
	// repository it seeds, else the one SCRUM_MASTER_TRACKER names (srd042).
	s, ok := routing.Override(o.Tracker)
	if !ok {
		s, ok = routing.Override(os.Getenv("SCRUM_MASTER_TRACKER")) //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
	}
	if ok {
		if s.Name == routing.Beads && o.TrackerBranch != beads.DefaultBranch {
			s.TrackerBranch = o.TrackerBranch
		}
		// The program group is recorded beside the tracker, so plan push
		// dates milestones, and labels land, where init put the labels
		// (srd043 R1.5, GH-103).
		files[routing.File] = routing.Document(routing.Config{Group: o.Group, Default: s})
		r.Group = o.Group
	} else if o.Group != "" {
		r.GroupReason = "no tracking system is known, so init writes no control/trackers.md to record the group in; rerun with --tracker"
	}
	if r.Files, err = write(o.Dir, files, o.Force, o.DryRun); err != nil {
		return r, err
	}
	if len(o.Agents) > 0 {
		skills, err := install.EmbeddedSkills()
		if err != nil {
			return r, err
		}
		if r.Skills, err = install.Install(o.Dir, o.Agents, skills, o.Version, o.Force, o.DryRun); err != nil {
			return r, err
		}
	}
	switch {
	case o.Tracker != "":
		tr, err := seedTracker(o)
		r.Tracker = &tr
		if err != nil {
			return r, err
		}
	case !beads.Configured():
		r.Tracker = &TrackerResult{Reason: NoTracker}
	}
	return r, nil
}

// NoTracker is what init reports when it is given no tracker repository and
// SCRUM_MASTER_TRACKER names none: the issue workflow cannot run until one exists.
const NoTracker = "no tracker repository seeded; issues cannot be filed until one exists: rerun with --tracker and --group"

// seedTracker makes an empty tracker repository's first commit, holding the
// config file with the issue prefix and the files layout, and creates the program's labels there. A tracker
// repository that already has commits is left as it is. With a tracker
// branch, the commit is that branch's first, an orphan sharing no history
// with the repository's other branches, made when the branch does not
// exist yet; an existing branch is left as it is (srd043 R1.5).
func seedTracker(o InitOptions) (TrackerResult, error) {
	r := TrackerResult{Remote: o.Tracker}
	branch := o.TrackerBranch
	if branch == "" {
		branch = beads.DefaultBranch
	} else {
		r.Branch = branch
	}
	tmp, err := os.MkdirTemp("", "scrum-master-init-tracker-*")
	if err != nil {
		return r, err
	}
	defer os.RemoveAll(tmp)
	clone := filepath.Join(tmp, "tracker")
	if err := git(tmp, "clone", "-q", o.Tracker, clone); err != nil {
		return r, err
	}
	if branch == beads.DefaultBranch {
		if git(clone, "rev-parse", "--verify", "-q", "HEAD") == nil {
			r.Reason = "the tracker repository has commits; init left it as it is"
			return r, nil
		}
	} else {
		if git(clone, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+branch) == nil {
			r.Reason = "the tracker branch " + branch + " exists; init left it as it is"
			return r, nil
		}
		if !o.DryRun {
			// An orphan branch with an empty index: the code's files stay
			// on the code's branches.
			if err := git(clone, "checkout", "-q", "--orphan", branch); err != nil {
				return r, err
			}
			if err := git(clone, "rm", "-rqf", "--ignore-unmatch", "."); err != nil {
				return r, err
			}
		}
	}
	if o.DryRun {
		r.Seeded, r.Reason = true, "dry run: would make the first commit and create the labels"
		return r, nil
	}
	if err := os.MkdirAll(filepath.Join(clone, filepath.Dir(beads.ConfigFile)), 0o755); err != nil {
		return r, err
	}
	r.Prefix = o.Prefix
	if r.Prefix == "" {
		r.Prefix = beads.PrefixFor(o.Group)
	}
	// A new tracker keeps one file per issue (srd042 R3.1, eng04).
	if err := beads.WriteConfig(clone, beads.Config{IssuePrefix: r.Prefix, Layout: beads.LayoutFiles}); err != nil {
		return r, err
	}
	for _, args := range [][]string{
		{"add", beads.ConfigFile},
		{"commit", "-q", "-m", "scrum-master init: the tracker repository's first commit"},
		{"push", "-q", "origin", "HEAD:" + branch},
	} {
		if err := git(clone, args...); err != nil {
			return r, err
		}
	}
	r.Seeded = true
	t := beads.NewTracker(clone, o.User, o.Dir)
	t.Store.Branch = branch
	r.Labels, err = program.SyncLabels(t, o.Group, program.DefaultLabels(), false, false)
	return r, err
}

func git(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
