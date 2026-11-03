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
	// Pushed is whether the record commit reached origin (--push).
	Pushed bool     `json:"pushed,omitempty"`
	Notes  []string `json:"notes,omitempty"`
	DryRun bool     `json:"dry_run"`
}

// LedgerDocuments are the documents in the checkout at root that hold an
// issues record: its stories, then its epics (srd045 R4.4).
func LedgerDocuments(root string) ([]string, error) {
	var out []string
	for _, pattern := range []string{"stories/st[0-9]*.md", "epics/ep[0-9]*.md"} {
		paths, err := filepath.Glob(filepath.Join(root, "docs", "specs", filepath.FromSlash(pattern)))
		if err != nil {
			return nil, err
		}
		sort.Strings(paths)
		out = append(out, paths...)
	}
	return out, nil
}

// Record reconciles the issues record of every story and epic in the scrum
// repository checkout around dir — or of the one story names — with the
// tracking system (srd045 R4.4): the projects swept are the team's own,
// its scrum repository and the code repositories its subsystem SRD
// declares, or in the system repository, whose epics span teams, every
// repository the registry names; an entry whose issue lives in a project
// outside them is kept as written, since the sweep cannot judge it. The write is
// idempotent; commit records the changed documents on the current branch;
// a dry run writes nothing and reports what it would.
func Record(t tracker.Tracker, dir, story string, commit, dryRun bool) (RecordResult, error) {
	return record(t, dir, story, commit, false, dryRun)
}

// RecordAndPush is Record landing on origin (srd045 R4.4): the record goes on the
// default branch of the repository's main checkout, and is pushed, so everyone and
// every site sees it. A checkout it should not touch, or a push the host refuses,
// is a note and leaves the checkout level with origin; the next run writes the same
// record.
func RecordAndPush(t tracker.Tracker, dir, story string, dryRun bool) (RecordResult, error) {
	return record(t, dir, story, true, true, dryRun)
}

func record(t tracker.Tracker, dir, story string, commit, push, dryRun bool) (RecordResult, error) {
	r := RecordResult{DryRun: dryRun}
	root, err := toplevelOf(dir)
	if err != nil {
		return r, err
	}
	root, own, projects, notes, err := ledgerScope(root)
	if err != nil {
		return r, err
	}
	r.Scrum, r.Projects = own, projects
	r.Notes = append(r.Notes, notes...)
	branch := ""
	if push {
		root = checkout.MainCheckout(root)
		var why string
		if branch, why = landable(root, dryRun); why != "" {
			r.Notes = append(r.Notes, fmt.Sprintf("%s: the record is not written: %s", own, why))
			return r, nil
		}
	}
	entries, swept, sweepNotes := TrackerLedgers(t, r.Projects, own)
	r.Notes = append(r.Notes, sweepNotes...)
	paths, err := LedgerDocuments(root)
	if err != nil {
		return r, err
	}
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
		want := withUnswept(entries[id], held, swept)
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
		return r, fmt.Errorf("%s has no story or epic %s under docs/specs", own, story)
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
		if push {
			if why := pushRecord(root, branch); why != "" {
				r.Commit = ""
				r.Notes = append(r.Notes, fmt.Sprintf("%s: the record is not pushed, so it is dropped until the next run: %s", own, why))
			} else {
				r.Pushed = true
			}
		}
	}
	return r, nil
}

// landable is the default branch of the checkout at root, brought level with
// origin, or why the record should not be written there: a person's changes or
// other branch are theirs.
func landable(root string, dryRun bool) (branch, why string) {
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if status, err := git("status", "--porcelain"); err != nil || status != "" {
		return "", "its checkout holds changes"
	}
	branch = "main"
	if head, err := git("symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		branch = strings.TrimPrefix(head, "origin/")
	}
	if cur, _ := git("branch", "--show-current"); cur != branch {
		return "", "its checkout is on " + cur + ", not " + branch
	}
	if dryRun {
		return branch, ""
	}
	if out, err := git("fetch", "-q", "origin", branch); err != nil {
		return "", "fetch: " + out
	}
	if out, err := git("merge", "-q", "--ff-only", "FETCH_HEAD"); err != nil {
		return "", "fast-forward: " + out
	}
	return branch, ""
}

// pushRecord pushes the record commit, rebasing once over someone else's push. A
// push still refused, as a protected branch refuses it, drops the commit: the
// checkout was clean before it, and the next run writes the same record.
func pushRecord(root, branch string) (why string) {
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	out, err := git("push", "-q", "origin", branch)
	if err == nil {
		return ""
	}
	if _, ferr := git("fetch", "-q", "origin", branch); ferr == nil {
		if _, rerr := git("rebase", "-q", "FETCH_HEAD"); rerr == nil {
			if out, err = git("push", "-q", "origin", branch); err == nil {
				return ""
			}
		} else {
			git("rebase", "--abort")
		}
	}
	git("reset", "-q", "--hard", "origin/"+branch)
	return out
}

// RecordProgram records in the system repository and each team's scrum repository
// found beside it, landing each on origin. A repository with no checkout, or one
// that cannot be written, is a note: the site that runs this draws what there is.
func RecordProgram(t tracker.Tracker, dir string, dryRun bool) ([]RecordResult, []string, error) {
	file, err := checkout.FindTrackers(dir)
	if err != nil {
		return nil, nil, err
	}
	if file == "" {
		return nil, nil, fmt.Errorf("no system repository's checkout at or beside %s holds %s", dir, checkout.TrackersFile)
	}
	system := filepath.Dir(filepath.Dir(file))
	subs, err := program.SubsystemsIn(system)
	if err != nil {
		return nil, nil, err
	}
	dirs, notes := []string{system}, []string{}
	for _, sub := range subs.Teams(program.SystemControl(system)) {
		scrum := sub.ScrumRepository()
		found, ok := checkout.Locate(system, scrum)
		if !ok {
			notes = append(notes, scrum+": no checkout, so its record is not refreshed")
			continue
		}
		if !slices.Contains(dirs, found) {
			dirs = append(dirs, found)
		}
	}
	var out []RecordResult
	for _, d := range dirs {
		res, err := RecordAndPush(t, d, "", dryRun)
		if err != nil {
			notes = append(notes, d+": "+err.Error())
			continue
		}
		out = append(out, res)
		notes = append(notes, res.Notes...)
	}
	return out, notes, nil
}

// ledgerScope is where the issues of the checkout at root are recorded: the
// checkout holding the stories and epics, its project, and the projects whose
// issues implement them.
func ledgerScope(root string) (scrumRoot, own string, projects, notes []string, err error) {
	own = program.ProjectPath(checkout.Origin(root))
	if own == "" {
		return "", "", nil, nil, fmt.Errorf("%s has no origin naming its project, so its issues cannot be found", root)
	}
	projects = []string{own}
	if repositories := registryRepositories(root); len(repositories) > 0 {
		for _, p := range repositories {
			if !slices.Contains(projects, p) {
				projects = append(projects, p)
			}
		}
		return root, own, projects, nil, nil
	}
	sub, note := subsystemOf(root, own)
	if note != "" {
		return root, own, projects, []string{note}, nil
	}
	projects = sub.Repositories()
	if !slices.Contains(projects, own) {
		projects = append([]string{own}, projects...)
	}
	// Run from a code repository, the record lands in the team's scrum
	// repository, found as a checkout beside this one; its stories are the
	// record's (srd045 R4.4).
	if scrum := sub.ScrumRepository(); scrum != "" && scrum != own {
		dir, ok := checkout.Locate(root, scrum)
		if !ok {
			return "", "", nil, nil, fmt.Errorf("the stories live in %s, which has no checkout beside %s; scrum-master checkouts lays it down", scrum, root)
		}
		return dir, scrum, projects, nil, nil
	}
	return root, own, projects, nil, nil
}

// withUnswept is what the sweep says a document's record holds, plus each entry
// whose issue lives in a project the sweep did not read, kept as written: the
// sweep cannot judge it.
func withUnswept(read, held []LedgerEntry, swept map[string]bool) []LedgerEntry {
	want := read
	for _, e := range held {
		project, _, _ := strings.Cut(e.Ref, "#")
		if !swept[project] {
			want = append(want, e)
		}
	}
	return want
}

// TrackerLedgers reads from the tracking system the issues record each of
// the scrum repository's stories and epics should hold (srd045 R4.4): over
// projects, every issue that is no Sprint Goal whose Implements line resolves
// to one of them, keyed by document id. swept says which projects were read,
// so a caller keeps or leaves what it cannot judge, and the notes say
// which were not and why.
func TrackerLedgers(t tracker.Tracker, projects []string, scrum string) (map[string][]LedgerEntry, map[string]bool, []string) {
	entries := map[string][]LedgerEntry{}
	swept := map[string]bool{}
	var notes []string
	for _, p := range projects {
		issues, err := t.ListIssues(p, tracker.IssueQuery{State: "all"})
		if err != nil {
			notes = append(notes, fmt.Sprintf("%s is not read, so its issues stay as the documents record them: %v", p, err))
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

// implementsStory resolves an Implements target to a story or epic of the scrum
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

// registryRepositories is every repository the registry names when the
// checkout at root is the system repository itself, none otherwise.
func registryRepositories(root string) []string {
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(checkout.TrackersFile))); err != nil {
		return nil
	}
	subs, err := program.SubsystemsIn(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, sub := range subs {
		out = append(out, sub.Repositories()...)
	}
	return out
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
		push       bool
	}
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "record", Summary: "reconcile each story's and epic's issues record with the tracking system: who worked on its issues and when they opened and closed, written into the documents (srd045 R4.4)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&ra.dir, "dir", ".", "the scrum repository's checkout")
			fs.StringVar(&ra.story, "story", "", "record one story or epic, by its id")
			fs.BoolVar(&ra.commit, "commit", false, "commit the recorded documents on the current branch")
			fs.BoolVar(&ra.push, "push", false, "land the record on the default branch of the main checkout and push it; what stops that is a note")
		},
		Run: func(ctx *cli.Context) error {
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			var r RecordResult
			if ra.push {
				r, err = RecordAndPush(t, ra.dir, ra.story, ctx.DryRun)
			} else {
				r, err = Record(t, ra.dir, ra.story, ra.commit, ctx.DryRun)
			}
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

func init() {
	var a struct{ dir string }
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "record-program", Summary: "refresh the issues record in the system repository and each team's scrum repository, and push each; what stops one is a note",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&a.dir, "dir", ".", "the system repository's checkout, or a checkout beside it")
		},
		Run: func(ctx *cli.Context) error {
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			results, notes, err := RecordProgram(t, a.dir, ctx.DryRun)
			if err != nil {
				return err
			}
			var h strings.Builder
			for _, r := range results {
				changed := 0
				for _, s := range r.Stories {
					if s.Action == "recorded" {
						changed++
					}
				}
				fmt.Fprintf(&h, "%s: %d of %d documents recorded\n", r.Scrum, changed, len(r.Stories))
			}
			for _, n := range notes {
				fmt.Fprintf(&h, "note: %s\n", n)
			}
			return ctx.Emit(struct {
				Results []RecordResult `json:"results"`
				Notes   []string       `json:"notes,omitempty"`
				DryRun  bool           `json:"dry_run,omitempty"`
			}{results, notes, ctx.DryRun}, h.String())
		},
	})
}
