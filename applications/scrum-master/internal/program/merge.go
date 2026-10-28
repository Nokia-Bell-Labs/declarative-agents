// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// MergeResult is what merge reports.
type MergeResult struct {
	Project      string `json:"project"`
	IssueProject string `json:"issue_project,omitempty"`
	IID          int    `json:"issue"`
	Branch       string `json:"branch"`
	Commit       string `json:"commit"`
	Audit        bool   `json:"audit_run"` // the branch changes docs/, so the audit ran
	AuditCommand string `json:"audit_command"`
	// AuditReason says why the audit ran or did not: "docs/ changed" or
	// "no docs/ change" (srd013 R5.3).
	AuditReason string   `json:"audit_reason"`
	ApprovedBy  string   `json:"approved_by,omitempty"`
	Merged      bool     `json:"merged"` // merge made the merge commit now
	Closed      []string `json:"closed,omitempty"`
	// AlreadyClosed are --closes issues another merge closed first.
	AlreadyClosed []string `json:"already_closed,omitempty"`
	// Reported are the Sprint Goals the merge closed as reported complete,
	// each with a reported_complete event on its ledger (srd038 R2.2).
	Reported []string `json:"reported_complete,omitempty"`
	// AlreadyReported are the closed Sprint Goals whose ledgers held a
	// reported_complete event before this merge: it reports nothing new
	// for them (GH-24).
	AlreadyReported []string `json:"already_reported,omitempty"`
	// ToClose is what merge-branch leaves for close-delivered, the --closes
	// references comma-joined. Only the merge word outputs it (as "closes",
	// always present so the machine's close word resolves it; GH-201): in
	// the one-command merge's JSON it would read as "closed nothing" beside
	// closed (GH-20).
	ToClose []string `json:"-"`
	// Refused is why scrum-master refused the merge, set on the JSON a refusal
	// emits; the exit status is non-zero (GH-22).
	Refused string `json:"refused,omitempty"`
	DryRun  bool   `json:"dry_run"`
	// Cleanup is what scrum-master issue merge removed after the closes:
	// the issue's worktree and its branch, here and on origin (GH-2796).
	Cleanup *Cleanup `json:"cleanup,omitempty"`
}

// MergedByTrailer names, on a merge commit, the tracker user who ran the
// merge, whatever git identity made the commit: reports attribute merges by
// it (srd018).
const MergedByTrailer = "Merged-by:"

// Merge is delivery on the beads tracker, which delivers by local merge
// (srd042 R2.3): the gates and the merge of MergeBranch, then the closes of
// CloseDelivered — the two words the merge profile composes before its
// clean-up, so the command and the machine run the same code (GH-201,
// srd013 R5.7). A branch already merged is not merged again; its issues
// still close. scrum-master issue merge then cleans up (GH-2796).
func Merge(t tracker.Tracker, git Git, project, issueProject string, iid int, branch, dir string, gate MergeGate, closes []string, dryRun bool) (MergeResult, error) {
	r, err := MergeBranch(t, git, project, issueProject, iid, branch, dir, gate, closes, dryRun)
	if err != nil || dryRun {
		return r, err
	}
	r.Closed, r.Reported, r.AlreadyReported, r.AlreadyClosed, err = CloseDelivered(t, git, dir, r.Commit, issueProject, iid, closes)
	return r, err
}

// MergeAndCleanUp is Merge, then, unless keep or a dry run, CleanUp of the
// branch it delivered: a clean-up that cannot remove something reports why
// and fails nothing, since the delivery has landed (GH-2796).
func MergeAndCleanUp(t tracker.Tracker, git Git, project, issueProject string, iid int, branch, dir string, gate MergeGate, closes []string, dryRun, keep bool) (MergeResult, error) {
	r, err := Merge(t, git, project, issueProject, iid, branch, dir, gate, closes, dryRun)
	if err != nil || dryRun || keep {
		return r, err
	}
	c, cerr := CleanUp(git, dir, r.Branch, false)
	if cerr != nil {
		c.Reasons = append(c.Reasons, cerr.Error())
	}
	r.Cleanup = &c
	return r, nil
}

// MergeBranch is the merge word (GH-201, srd013 R5.7): the gates — the
// issue and each closes open, commits of its own, the sprint-end summary
// format, the corpus audit when docs/ changes, and, when the gate requires
// it, another person's approval of the commit it merges (srd013 R5.5; an
// approval given without the requirement is still reported) — then the
// --no-ff merge into the default branch of the repository's main checkout,
// with a message citing the issue and each issue it closes, pushed. It
// closes nothing: close-delivered does, on the commit this word reports.
func MergeBranch(t tracker.Tracker, git Git, project, issueProject string, iid int, branch, dir string, gate MergeGate, closes []string, dryRun bool) (MergeResult, error) {
	r := MergeResult{Project: project, IID: iid, Branch: branch, DryRun: dryRun}
	if _, ok := tracker.For(t, issueProject).(MergeCloser); !ok {
		return r, fmt.Errorf("this tracker does not close issues on a local merge; scrum-master issue merge serves the beads tracker")
	}
	if issueProject != project {
		r.IssueProject = issueProject
	}
	others, already, err := checkCloses(t, closes)
	if err != nil {
		return r, err
	}
	r.AlreadyClosed = already
	base, err := t.DefaultBranch(project)
	if err != nil {
		return r, err
	}
	if branch == base {
		// Merging the default branch into itself would find it "already
		// merged" and take an earlier merge citing the issue for this one's
		// (GH-22).
		return r, fmt.Errorf("%s is the default branch; merge the issue's own branch, from its worktree or with --branch", branch)
	}
	run := func(args ...string) (string, error) {
		out, err := git.Run(dir, args...)
		return strings.TrimSpace(out), err
	}
	common, err := run("rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return r, fmt.Errorf("merge needs a local checkout of %s: %w", project, err)
	}
	main := strings.TrimSuffix(strings.TrimSuffix(common, "/"), "/.git")
	inMain := func(args ...string) (string, error) {
		out, err := git.Run(main, args...)
		return strings.TrimSpace(out), err
	}

	// A reference already closed is reported, not cited: the merge closes
	// only what is open, and every cited reference reads as closed by it.
	cite := fmt.Sprintf("%s#%d", issueProject, iid)
	for _, ref := range closes {
		if !slices.Contains(already, ref) {
			cite += ", closes " + ref
		}
	}
	inMain("fetch", "-q", "origin", base)
	// The base is what origin holds now: the main checkout's own branch may
	// be behind it, and a diff against that would count others' merges as
	// this branch's changes.
	upstream := "refs/heads/" + base
	if _, err := inMain("rev-parse", "--verify", "-q", "refs/remotes/origin/"+base); err == nil {
		upstream = "refs/remotes/origin/" + base
	}
	if _, err := inMain("merge-base", "--is-ancestor", "refs/heads/"+branch, upstream); err == nil {
		exact := "--grep=" + regexp.QuoteMeta(fmt.Sprintf("%s#%d", issueProject, iid)) + "([^0-9]|$)"
		r.Commit, _ = inMain("rev-list", "-1", "--merges", "-E", exact, upstream)
		if r.Commit == "" {
			return r, fmt.Errorf("%s has no commits over %s; commit the work first", branch, base)
		}
		// A dry run of a branch already merged answers what a run would do:
		// close what is still open, or nothing when the issue is closed —
		// which it refuses, as it refuses a closed issue it has not merged
		// (GH-20).
		if dryRun {
			i, err := t.GetIssue(issueProject, iid)
			if err != nil {
				return r, err
			}
			if i.State == "closed" {
				return r, fmt.Errorf("%s#%d is already delivered: %s merged on %s and the issue is closed; nothing would merge or close", issueProject, iid, branch, short12(r.Commit))
			}
		}
	} else {
		i, err := t.GetIssue(issueProject, iid)
		if err != nil {
			return r, err
		}
		if i.State == "closed" {
			return r, fmt.Errorf("%s#%d is closed", issueProject, iid)
		}
		paths, err := inMain("diff", "--name-only", upstream+"...refs/heads/"+branch)
		if err != nil {
			return r, err
		}
		for _, p := range strings.Split(paths, "\n") {
			if p == "docs" || strings.HasPrefix(p, "docs/") {
				r.Audit = true
			}
			if path.Dir(p) == sprintgoal.SummaryDir && path.Ext(p) == ".md" {
				// Through textconv, as GetFile reads: in a git-crypt
				// repository the blob is ciphertext.
				summary, err := inMain("cat-file", "--textconv", "refs/heads/"+branch+":"+p)
				if err != nil {
					continue // the branch deletes it
				}
				if problems := sprintgoal.CheckSummary(summary); len(problems) > 0 {
					return r, fmt.Errorf("the sprint-end summary %s is not in its format (design.md sprint_end_summary): %s; nothing merged", p, strings.Join(problems, "; "))
				}
			}
		}
		r.AuditCommand, r.AuditReason = gate.Command, "no docs/ change"
		if r.Audit {
			r.AuditReason = "docs/ changed"
			if err := gate.checkBefore("nothing merged"); err != nil {
				return r, err
			}
		}
		me, err := tracker.UserIn(t, issueProject)
		if err != nil {
			return r, err
		}
		tip, err := inMain("rev-parse", "refs/heads/"+branch)
		if err != nil {
			return r, err
		}
		d := deliveryFor(t, git, project)
		by, err := d.approval(issueProject, iid, branch, tip, me.Username)
		if err != nil && gate.RequireApproval {
			return r, err
		}
		if err == nil {
			r.ApprovedBy = by
		}
		if dryRun {
			return r, nil
		}
		if r.Commit, err = d.merge(main, base, branch, fmt.Sprintf("Merge %s (%s)\n\n%s %s", branch, cite, MergedByTrailer, me.Username)); err != nil {
			return r, err
		}
		r.Merged = true
	}
	if dryRun {
		return r, nil
	}
	_ = others
	r.ToClose = closes
	return r, nil
}

// CloseDelivered is the close word (GH-201, srd013 R5.7): on the merge
// commit the merge word reports, it closes the issue and each --closes
// reference that is still open, posts a reported_complete event on each
// closed Sprint Goal's ledger, and reports complete a Sprint Goal another
// merge closed first — its ledger gets one now, or keeps the one that
// merge posted, and the goal is then already reported, not reported again
// (GH-24); one closed as met is neither (srd038 R2.2). A second run
// changes nothing: a closed issue stays closed on its commit.
func CloseDelivered(t tracker.Tracker, git Git, dir, commit, issueProject string, iid int, closes []string) (closed, reported, alreadyReported, alreadyClosed []string, err error) {
	others, already, err := checkCloses(t, closes)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	out, err := git.Run(dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("close-delivered needs a local checkout of %s: %w", issueProject, err)
	}
	main := strings.TrimSuffix(strings.TrimSuffix(strings.TrimSpace(out), "/"), "/.git")
	targets := append([][2]any{{issueProject, iid}}, others...)
	for _, tgt := range targets {
		p, n := tgt[0].(string), tgt[1].(int)
		c, err := Close(t, git, p, n, main, commit, false)
		if err != nil {
			return closed, reported, alreadyReported, already, fmt.Errorf("merged %s, but %s#%d did not close: %w", commit, p, n, err)
		}
		if c.Closed {
			closed = append(closed, p+"#"+strconv.Itoa(n))
		}
	}
	for _, tgt := range others {
		p, n := tgt[0].(string), tgt[1].(int)
		i, err := t.GetIssue(p, n)
		if err != nil {
			return closed, reported, alreadyReported, already, err
		}
		m, ok := sprintgoal.Parse(i.Description)
		if !ok {
			continue
		}
		e, text := events.ReportedComplete(m.Release, commit)
		posted, err := events.Post(t, p, n, e, events.ReportedCompleteRequired, text, false)
		if err != nil {
			return closed, reported, alreadyReported, already, fmt.Errorf("closed %s#%d, but its ledger has no reported_complete event: %w", p, n, err)
		}
		if posted {
			reported = append(reported, p+"#"+strconv.Itoa(n))
		} else {
			alreadyReported = append(alreadyReported, p+"#"+strconv.Itoa(n))
		}
	}
	for _, ref := range already {
		posted, was, err := reportClosed(t, ref, commit)
		if err != nil {
			return closed, reported, alreadyReported, already, err
		}
		switch {
		case posted:
			reported = append(reported, ref)
		case was:
			alreadyReported = append(alreadyReported, ref)
		}
	}
	return closed, reported, alreadyReported, already, nil
}

// reportClosed reports, for the closed issue ref, whether it posted a
// reported_complete event, which it does for a Sprint Goal whose ledger
// has neither that nor a met event, and whether the ledger already held
// one.
func reportClosed(t tracker.Tracker, ref, commit string) (posted, already bool, err error) {
	p, n, ok := strings.Cut(ref, "#")
	iid, err := strconv.Atoi(n)
	if !ok || err != nil {
		return false, false, nil
	}
	i, err := t.GetIssue(p, iid)
	if err != nil {
		return false, false, err
	}
	m, ok := sprintgoal.Parse(i.Description)
	if !ok {
		return false, false, nil
	}
	notes, err := t.ListNotes(p, iid)
	if err != nil {
		return false, false, err
	}
	met, reported := false, false
	for _, note := range notes {
		if e, ok := events.Parse(note.Body); ok {
			met = met || e.Name == "met"
			reported = reported || e.Name == "reported_complete"
		}
	}
	if met || reported {
		return false, !met, nil
	}
	e, text := events.ReportedComplete(m.Release, commit)
	if _, err := events.Post(t, p, iid, e, events.ReportedCompleteRequired, text, false); err != nil {
		return false, false, fmt.Errorf("%s was closed, but its ledger has no reported_complete event: %w", ref, err)
	}
	return true, false, nil
}

// short12 is a commit's first twelve characters.
func short12(c string) string { return c[:min(12, len(c))] }

// DeliveredResult is what close-delivered reports (GH-64): the issues it closed
// on the merge commit and the Sprint Goals it reported complete. It carries
// none of the merge's fields, which the merge word reported; a run's
// summary holds both documents.
type DeliveredResult struct {
	Project         string   `json:"project"`
	IssueProject    string   `json:"issue_project,omitempty"`
	IID             int      `json:"issue"`
	Commit          string   `json:"commit"`
	Closed          []string `json:"closed"`
	AlreadyClosed   []string `json:"already_closed,omitempty"`
	Reported        []string `json:"reported_complete"`
	AlreadyReported []string `json:"already_reported,omitempty"`
}
