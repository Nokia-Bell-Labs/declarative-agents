// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// RecordedStory is one story the record swept: unchanged when its issues
// field already matches the tracking system, recorded when the word wrote
// it, with the refs the field now holds.
type RecordedStory struct {
	Story  string   `json:"story"`
	Path   string   `json:"path"`
	Action string   `json:"action"` // unchanged, recorded
	Issues []string `json:"issues,omitempty"`
}

// RecordResult is what scrum-master backlog record reports (srd045 R4.4).
type RecordResult struct {
	Scrum    string          `json:"scrum"`
	Projects []string        `json:"projects"`
	Stories  []RecordedStory `json:"stories"`
	Commit   string          `json:"commit,omitempty"`
	Notes    []string        `json:"notes,omitempty"`
	DryRun   bool            `json:"dry_run"`
}

// Record reconciles the issues record of every story in the scrum
// repository checkout around dir — or of the one story names — with the
// tracking system (srd045 R4.4): the projects swept are the team's own,
// its scrum repository and the code repositories its subsystem SRD
// declares, and an entry whose issue lives in a project outside them is
// kept as written, since the sweep cannot judge it. The write is
// idempotent; commit records the changed documents on the current branch;
// a dry run writes nothing and reports what it would.
func Record(t tracker.Tracker, dir, story string, commit, dryRun bool) (RecordResult, error) {
	r := RecordResult{DryRun: dryRun}
	root, err := toplevelOf(dir)
	if err != nil {
		return r, err
	}
	own := program.ProjectPath(checkout.Origin(root))
	if own == "" {
		return r, fmt.Errorf("%s has no origin naming its project, so its issues cannot be found", root)
	}
	r.Scrum = own
	r.Projects = []string{own}
	if sub, note := subsystemOf(root, own); note != "" {
		r.Notes = append(r.Notes, note)
	} else {
		r.Projects = sub.Repositories()
		if !slices.Contains(r.Projects, own) {
			r.Projects = append([]string{own}, r.Projects...)
		}
		// Run from a code repository, the record lands in the team's
		// scrum repository, found as a checkout beside this one; its
		// stories are the record's (srd045 R4.4).
		if scrum := sub.ScrumRepository(); scrum != "" && scrum != own {
			dir, ok := checkout.Locate(root, scrum)
			if !ok {
				return r, fmt.Errorf("the stories live in %s, which has no checkout beside %s; scrum-master checkouts lays it down", scrum, root)
			}
			root, own, r.Scrum = dir, scrum, scrum
		}
	}
	entries, swept, notes := TrackerLedgers(t, r.Projects, own)
	r.Notes = append(r.Notes, notes...)
	paths, err := filepath.Glob(filepath.Join(root, "docs", "specs", "stories", "st[0-9]*.md"))
	if err != nil {
		return r, err
	}
	sort.Strings(paths)
	var changed []string
	for _, p := range paths {
		id := strings.TrimSuffix(filepath.Base(p), ".md")
		if story != "" && id != story {
			continue
		}
		doc, err := os.ReadFile(p)
		if err != nil {
			return r, err
		}
		held, err := LedgerOf(doc)
		if err != nil {
			r.Notes = append(r.Notes, fmt.Sprintf("%s: its issues field does not parse, so it is left as written: %v", id, err))
			continue
		}
		want := entries[id]
		// An entry whose issue lives in a project the sweep did not read
		// is kept as written: the sweep cannot judge it.
		for _, e := range held {
			project, _, _ := strings.Cut(e.Ref, "#")
			if !swept[project] {
				want = append(want, e)
			}
		}
		next := WriteLedger(doc, want)
		rel, _ := filepath.Rel(root, p)
		entry := RecordedStory{Story: id, Path: filepath.ToSlash(rel), Action: "unchanged"}
		for _, e := range want {
			entry.Issues = append(entry.Issues, e.Ref)
		}
		sort.Strings(entry.Issues)
		if !bytes.Equal(next, doc) {
			entry.Action = "recorded"
			if !dryRun {
				if err := os.WriteFile(p, next, 0o644); err != nil {
					return r, err
				}
			}
			changed = append(changed, filepath.ToSlash(rel))
		}
		r.Stories = append(r.Stories, entry)
	}
	if story != "" && len(r.Stories) == 0 {
		return r, fmt.Errorf("%s has no story %s under docs/specs/stories", own, story)
	}
	if commit && !dryRun && len(changed) > 0 {
		args := append([]string{"-C", root, "add", "--"}, changed...)
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return r, fmt.Errorf("add the recorded stories: %v: %s", err, bytes.TrimSpace(out))
		}
		msg := "The stories record their issues (scrum-master backlog record)"
		if out, err := exec.Command("git", "-C", root, "commit", "-q", "-m", msg).CombinedOutput(); err != nil {
			return r, fmt.Errorf("commit the recorded stories: %v: %s", err, bytes.TrimSpace(out))
		}
		sha, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
		if err != nil {
			return r, err
		}
		r.Commit = strings.TrimSpace(string(sha))
	}
	return r, nil
}

// TrackerLedgers reads from the tracking system the issues record each of
// the scrum repository's stories should hold (srd045 R4.4): over projects,
// every issue that is no Sprint Goal whose Implements line resolves to one
// of its stories, keyed by story id. swept says which projects were read,
// so a caller keeps or leaves what it cannot judge, and the notes say
// which were not and why.
func TrackerLedgers(t tracker.Tracker, projects []string, scrum string) (map[string][]LedgerEntry, map[string]bool, []string) {
	entries := map[string][]LedgerEntry{}
	swept := map[string]bool{}
	var notes []string
	for _, p := range projects {
		issues, err := t.ListIssues(p, tracker.IssueQuery{State: "all"})
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s is not read, so its issues are not recorded: %v", p, err))
			continue
		}
		swept[p] = true
		for _, i := range issues {
			if _, isGoal := sprintgoal.Parse(i.Description); isGoal {
				continue
			}
			for _, l := range links.ParseLinks(i.Description) {
				if l.Kind != links.Implements {
					continue
				}
				if id, ok := implementsStory(l.Target, p, scrum); ok {
					entries[id] = append(entries[id], EntryFor(p, i))
				}
			}
		}
	}
	return entries, swept, notes
}

// implementsStory resolves an Implements target to a story of the scrum
// repository: a bare id is the issue's own repository's story, so it
// counts only for an issue the scrum repository holds, and a pinned URL
// counts when it names the scrum repository (GH-2738, GH-2811).
func implementsStory(target, project, scrum string) (string, bool) {
	file, _, _ := strings.Cut(strings.TrimSpace(target), "#")
	id := strings.TrimSuffix(path.Base(file), ".md")
	ref, err := links.Parse(target)
	switch {
	case err != nil:
		return id, project == scrum
	case ref.Project == scrum:
		return id, true
	}
	return "", false
}

// subsystemOf is the registry row whose repositories include own — its
// scrum repository or one of its code repositories — read from the system
// checkout found through the trackers document; the note says why the
// sweep is the repository alone when there is no row to read.
func subsystemOf(root, own string) (program.Subsystem, string) {
	file, err := checkout.FindTrackers(root)
	if err != nil || file == "" {
		return program.Subsystem{}, "no checkout beside this one holds the trackers document, so only this repository's own issues are recorded"
	}
	system := filepath.Dir(filepath.Dir(file))
	subs, err := program.SubsystemsIn(system)
	if err != nil {
		return program.Subsystem{}, fmt.Sprintf("the subsystem SRDs in %s are not read, so only this repository's own issues are recorded: %v", system, err)
	}
	for _, sub := range subs {
		if sub.ScrumRepository() == own || slices.Contains(sub.Repositories(), own) {
			return sub, ""
		}
	}
	return program.Subsystem{}, fmt.Sprintf("no subsystem SRD names %s among its repositories, so only its own issues are recorded", own)
}

// toplevelOf is the checkout root around dir.
func toplevelOf(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("%s is not inside a checkout", dir)
	}
	return strings.TrimSpace(string(out)), nil
}

func init() {
	var ra struct {
		dir, story string
		commit     bool
	}
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "record", Summary: "reconcile each story's issues record with the tracking system: who worked on its issues and when they opened and closed, written into the story documents (srd045 R4.4)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&ra.dir, "dir", ".", "the scrum repository's checkout")
			fs.StringVar(&ra.story, "story", "", "record one story, by its id")
			fs.BoolVar(&ra.commit, "commit", false, "commit the recorded documents on the current branch")
		},
		Run: func(ctx *cli.Context) error {
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := Record(t, ra.dir, ra.story, ra.commit, ctx.DryRun)
			if err != nil {
				return err
			}
			var h strings.Builder
			for _, s := range r.Stories {
				action := s.Action
				if r.DryRun && action == "recorded" {
					action = "would record"
				}
				fmt.Fprintf(&h, "%-10s %s: %s\n", action, s.Story, strings.Join(s.Issues, ", "))
			}
			if len(r.Stories) == 0 {
				h.WriteString("no stories under docs/specs/stories\n")
			}
			if r.Commit != "" {
				fmt.Fprintf(&h, "committed %s\n", r.Commit)
			}
			for _, n := range r.Notes {
				fmt.Fprintf(&h, "note: %s\n", n)
			}
			return ctx.Emit(r, h.String())
		},
	})
}
