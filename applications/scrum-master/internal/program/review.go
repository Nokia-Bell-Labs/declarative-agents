// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/people"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// ReviewerKey opens the issue body's line naming who signs off on the issue
// (config format issue-body).
const ReviewerKey = "Reviewer"

// ReviewerLine is the person the description's Reviewer line names, if any.
func ReviewerLine(description string) string {
	for _, l := range strings.Split(description, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(l), ReviewerKey+":"); ok {
			return strings.TrimSpace(rest)
		}
	}
	return ""
}

// Signoff is one person a merge needs, and why. Own is an issue's Reviewer
// line, naming them outright; the rest are inherited.
type Signoff struct {
	Person string `json:"person"`
	Why    string `json:"why"`
	Own    bool   `json:"own,omitempty"`
}

// Reviews finds who signs off (srd013 R11.1) from the control files a policy
// was read from. They are read at that ref, never from the branch being merged,
// so a branch cannot name its own reviewer.
type Reviews struct {
	t       tracker.Tracker
	system  string
	group   string
	ref     string
	def     string
	people  people.Config
	subs    Subsystems
	subsErr error
	loaded  bool
	parents map[string]string
}

// LoadReviews reads who the program names from where p was read. A program with
// no control/people.md and no default names no one, and only an issue's own line
// can.
func LoadReviews(t tracker.Tracker, p Policy) (*Reviews, error) {
	r := &Reviews{t: t, ref: p.Ref, def: p.DefaultReviewer, parents: map[string]string{}}
	if p.Control == "" {
		return r, nil
	}
	r.system = ProjectPath(p.Control)
	r.group = path.Dir(r.system)
	b, err := t.GetFile(p.Control, people.ControlFile, p.Ref)
	if errors.Is(err, tracker.ErrNotFound) {
		return r, nil
	}
	if err != nil {
		return r, fmt.Errorf("%s %s: %w", p.Control, people.ControlFile, err)
	}
	if r.people, err = people.Parse(b); err != nil {
		return r, fmt.Errorf("%s: %w", p.Control, err)
	}
	return r, nil
}

// inherits reports whether the program names anyone beyond an issue's own line,
// so a program that names no one reads no document on a merge.
func (r *Reviews) inherits() bool { return r.def != "" || r.people.HasReviewers() }

// name is a document as the site and control/people.md name it: the system's
// by id, a team's as <name>/<id>.
func (r *Reviews) name(project, id string) string {
	if project == r.system {
		return id
	}
	return RepositoryName(r.group, project) + "/" + id
}

// parent is the document's parent as holder and id, read at the holder's
// default branch; a document that cannot be read has none.
func (r *Reviews) parent(project, id string) (string, string) {
	key := project + "\x00" + id
	target, ok := r.parents[key]
	if !ok {
		target = r.readParent(project, id)
		r.parents[key] = target
	}
	return targetDocument(project, target)
}

func (r *Reviews) readParent(project, id string) string {
	file := hierarchy.PathOf(id)
	if file == "" {
		return ""
	}
	branch, err := r.t.DefaultBranch(project)
	if err != nil {
		return ""
	}
	b, err := r.t.GetFile(project, file, branch)
	if err != nil {
		return ""
	}
	d, err := hierarchy.Parse(file, b)
	if err != nil {
		return ""
	}
	return d.Parent
}

// targetDocument is what a same-repository id or a pinned URL names.
func targetDocument(project, target string) (string, string) {
	if target == "" {
		return "", ""
	}
	if strings.Contains(target, "://") {
		ref, err := links.Parse(target)
		if err != nil {
			return "", ""
		}
		return ref.Project, docID(ref.Path)
	}
	return project, docID(target)
}

// ofDocument is step two of the lookup: the document's reviewer, else its
// parents' up to its epic.
func (r *Reviews) ofDocument(project, id string) (Signoff, bool) {
	for depth := 0; id != "" && depth < 8; depth++ {
		name := r.name(project, id)
		if who := r.people.ReviewerOf(name); who != "" {
			return Signoff{Person: who, Why: "the reviewer of " + name}, true
		}
		if kind, ok := hierarchy.KindOf(id); !ok || kind == hierarchy.Epic {
			break
		}
		project, id = r.parent(project, id)
	}
	return Signoff{}, false
}

// ofScrum and the default are the lookup's last steps.
func (r *Reviews) ofScrum(project string) (Signoff, bool, error) {
	if !r.loaded {
		r.loaded = true
		if r.system != "" {
			r.subs, r.subsErr = LoadSubsystems(r.t, r.system, r.ref)
			if errors.Is(r.subsErr, tracker.ErrNotFound) {
				r.subsErr = nil
			}
		}
	}
	if r.subsErr != nil {
		return Signoff{}, false, r.subsErr
	}
	if sub, ok := r.subs.SubsystemFor(project); ok {
		name := RepositoryName(r.group, sub.ScrumRepository())
		if who := r.people.ReviewerOf(name); who != "" {
			return Signoff{Person: who, Why: "the reviewer of the scrum " + name}, true, nil
		}
	}
	if r.def != "" {
		return Signoff{Person: r.def, Why: "the program's default reviewer"}, true, nil
	}
	return Signoff{}, false, nil
}

// ForIssue is who signs off on an issue in project, by srd013 R11.1's order.
func (r *Reviews) ForIssue(project string, i tracker.Issue) (Signoff, bool, error) {
	if who := ReviewerLine(i.Description); who != "" {
		return Signoff{Person: who, Why: "the issue's Reviewer line", Own: true}, true, nil
	}
	if !r.inherits() {
		return Signoff{}, false, nil
	}
	for _, l := range links.ParseLinks(i.Description) {
		if l.Kind != links.Implements {
			continue
		}
		if holder, id := targetDocument(project, l.Target); id != "" {
			if s, ok := r.ofDocument(holder, id); ok {
				return s, true, nil
			}
		}
	}
	return r.ofScrum(project)
}

// ForDocument is who signs off on a document in project.
func (r *Reviews) ForDocument(project, id string) (Signoff, bool, error) {
	if !r.inherits() {
		return Signoff{}, false, nil
	}
	if s, ok := r.ofDocument(project, id); ok {
		return s, true, nil
	}
	return r.ofScrum(project)
}

// Required is everyone a merge of branch needs: the issue's reviewer and the
// reviewer of each document it changes. A document's reviewer is added, not
// replaced, so an issue's Reviewer line cannot stand in for them.
func (r *Reviews) Required(project, issueProject string, i tracker.Issue, documents []string) ([]Signoff, error) {
	var out []Signoff
	add := func(s Signoff, ok bool, err error) error {
		if err != nil {
			return err
		}
		if ok && !slices.ContainsFunc(out, func(o Signoff) bool { return o.Person == s.Person }) {
			out = append(out, s)
		}
		return nil
	}
	s, ok, err := r.ForIssue(issueProject, i)
	if err := add(s, ok, err); err != nil {
		return nil, err
	}
	for _, id := range documents {
		s, ok, err := r.ForDocument(project, id)
		if err := add(s, ok, err); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// changedDocuments are the stories, features, epics, and SRDs a branch changes.
// A change to a document's issues record alone is scrum-master backlog record's
// bookkeeping, not a change to what was agreed. Files are read through textconv,
// as GetFile reads them.
func changedDocuments(inMain func(...string) (string, error), upstream, branch string, paths []string) []string {
	dirs := []string{SRDDir, hierarchy.ObjectiveDir}
	for _, l := range hierarchy.Levels {
		dirs = append(dirs, l.Dir)
	}
	var out []string
	for _, p := range paths {
		if path.Ext(p) != ".md" || !slices.Contains(dirs, path.Dir(p)) {
			continue
		}
		before, _ := inMain("cat-file", "--textconv", upstream+":"+p)
		after, _ := inMain("cat-file", "--textconv", "refs/heads/"+branch+":"+p)
		if bytes.Equal(hierarchy.WithoutIssues([]byte(before)), hierarchy.WithoutIssues([]byte(after))) {
			continue
		}
		out = append(out, strings.TrimSuffix(path.Base(p), ".md"))
	}
	return out
}

// signedOff is who of required approved branch at tip, or an error naming
// everyone still needed. Only a required person's approval counts for them.
// An inherited reviewer who wrote the work cannot approve it, so that
// requirement falls back to anyone but excluded; an issue's own line naming
// them is refused, for a person to change.
func signedOff(d delivery, required []Signoff, excluded []string, issueProject string, iid int, branch, tip string) ([]string, error) {
	at, stale, err := d.approvals(issueProject, iid, branch, tip)
	if err != nil {
		return nil, err
	}
	short := func(c string) string { return c[:min(12, len(c))] }
	var approvers, missing []string
	anyone := false
	for _, s := range required {
		switch old, wasStale := stale[s.Person]; {
		case slices.Contains(excluded, s.Person) && s.Own:
			missing = append(missing, fmt.Sprintf("%s, %s, wrote the work; name someone else with scrum-master issue reviewer", s.Person, s.Why))
		case slices.Contains(excluded, s.Person):
			anyone = true
		case slices.Contains(at, s.Person):
			approvers = append(approvers, s.Person)
		case wasStale:
			missing = append(missing, fmt.Sprintf("%s, %s, approved %s and %s is now at %s", s.Person, s.Why, short(old), branch, short(tip)))
		default:
			missing = append(missing, fmt.Sprintf("%s, %s, has not approved %s at %s", s.Person, s.Why, branch, short(tip)))
		}
	}
	if anyone && len(approvers) == 0 {
		i := slices.IndexFunc(at, func(who string) bool { return !slices.Contains(excluded, who) })
		if i >= 0 {
			approvers = append(approvers, at[i])
		} else {
			missing = append(missing, fmt.Sprintf("someone other than %s has not approved %s at %s", strings.Join(excluded, ", "), branch, short(tip)))
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s needs sign-off (srd013 R11.2): %s; ask them to approve with the approve skill, or change who reviews it", branch, strings.Join(missing, "; "))
	}
	return approvers, nil
}

// requestReview asks whoever signs off on the issue to review its branch on the
// host's review. It returns who it asked; what stops it is a note and never
// fails the word, since the merge, not the request, holds the sign-off. A
// reviewer who wrote the work is not asked.
func requestReview(t tracker.Tracker, project, issueProject string, i tracker.Issue, branch string, p Policy, dryRun bool) (string, string) {
	rv, ok := tracker.For(t, project).(tracker.Reviewer)
	if !ok {
		return "", ""
	}
	reviews, err := LoadReviews(t, p)
	if err != nil {
		return "", err.Error()
	}
	s, ok, err := reviews.ForIssue(issueProject, i)
	if err != nil {
		return "", err.Error()
	}
	if !ok || slices.Contains(i.Assignees, s.Person) {
		return "", ""
	}
	me, err := tracker.UserIn(t, project)
	if err != nil {
		return "", err.Error()
	}
	if s.Person == me.Username {
		return "", ""
	}
	if !dryRun {
		if err := rv.RequestReview(project, branch, []string{s.Person}); err != nil {
			return "", fmt.Sprintf("could not ask %s to review %s: %v", s.Person, branch, err)
		}
	}
	return s.Person, ""
}

// ReviewerResult is what scrum-master issue reviewer did.
type ReviewerResult struct {
	Ref      string `json:"ref"`
	Reviewer string `json:"reviewer,omitempty"`
	Previous string `json:"previous,omitempty"`
	Action   string `json:"action"`
	// Requested is whether the host's review asked them; Note is why not.
	Requested bool   `json:"requested,omitempty"`
	Note      string `json:"note,omitempty"`
	DryRun    bool   `json:"dry_run,omitempty"`
}

// NameReviewer writes the issue's Reviewer line, or, with an empty person, drops
// it (srd013 R11.4). It refuses a closed issue, the issue's assignee, and an issue
// already naming someone else unless force says to replace them. Where the issue's
// branch has a review on the host, it asks them too.
func NameReviewer(t tracker.Tracker, project, issueProject string, iid int, person string, force bool, p Policy, dryRun bool) (ReviewerResult, error) {
	r := ReviewerResult{Ref: fmt.Sprintf("%s#%d", issueProject, iid), Reviewer: person, Action: "unchanged", DryRun: dryRun}
	i, err := t.GetIssue(issueProject, iid)
	if err != nil {
		return r, err
	}
	if i.State == "closed" {
		return r, fmt.Errorf("%s is closed", r.Ref)
	}
	if slices.Contains(i.Assignees, person) {
		return r, fmt.Errorf("%s is assigned %s, who cannot sign off on their own work", r.Ref, person)
	}
	r.Previous = ReviewerLine(i.Description)
	if r.Previous != "" && r.Previous != person && person != "" && !force {
		return r, fmt.Errorf("%s names %s as its reviewer; --force replaces them", r.Ref, r.Previous)
	}
	body := withLine(i.Description, ReviewerKey, person)
	if body == i.Description {
		return r, nil
	}
	r.Action = "named"
	if person == "" {
		r.Action = "dropped"
	}
	if dryRun {
		return r, nil
	}
	if i, err = t.UpdateIssue(issueProject, iid, tracker.IssueUpdate{Description: &body}); err != nil {
		return r, err
	}
	if person == "" {
		return r, nil
	}
	branch := fmt.Sprintf("%d-%s", iid, Slug(i.Title))
	if exists, err := t.BranchExists(project, branch); err != nil || !exists {
		return r, err
	}
	who, note := requestReview(t, project, issueProject, i, branch, p, false)
	r.Requested, r.Note = who == person, note
	return r, nil
}

// ListReviewing lists the open issues in projects the caller signs off on,
// by project then IID, leaving out any assigned to them.
func ListReviewing(t tracker.Tracker, reviews *Reviews, projects []string) (ListResult, error) {
	r := ListResult{Stories: []string{}, Issues: []Listed{}, Blocked: []Listed{}}
	if len(projects) == 1 {
		r.Project = projects[0]
	}
	seen := map[string]bool{}
	for _, project := range projects {
		if seen[project] {
			continue
		}
		seen[project] = true
		me, err := tracker.UserIn(t, project)
		if err != nil {
			return r, err
		}
		open, err := t.ListIssues(project, tracker.IssueQuery{State: "opened"})
		if err != nil {
			return r, fmt.Errorf("%s: %w", project, err)
		}
		slices.SortStableFunc(open, func(a, b tracker.Issue) int { return a.IID - b.IID })
		for _, i := range open {
			s, ok, err := reviews.ForIssue(project, i)
			if err != nil {
				return r, err
			}
			if !ok || s.Person != me.Username || slices.Contains(i.Assignees, me.Username) {
				continue
			}
			it, err := listing(t, project, i)
			if err != nil {
				return r, err
			}
			if len(it.BlockedBy) > 0 {
				r.Blocked = append(r.Blocked, it)
			} else {
				r.Issues = append(r.Issues, it)
			}
		}
	}
	return r, nil
}

func init() {
	var a struct {
		project, issueProject, person, control, ref string
		iid                                         int
		force, clear                                bool
	}
	cli.Default.Register(&cli.Command{
		Group: "issue", Name: "reviewer", Summary: "name who signs off on an issue, whose approval its merge then needs; agents never run it",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&a.project, "project", "", "project holding the issue's branch (required)")
			fs.IntVar(&a.iid, "issue", 0, "issue IID (required)")
			fs.StringVar(&a.person, "person", "", "the tracker username who signs off")
			fs.BoolVar(&a.clear, "clear", false, "drop the issue's Reviewer line instead of naming someone")
			fs.BoolVar(&a.force, "force", false, "replace the reviewer the issue already names")
			fs.StringVar(&a.issueProject, "issue-project", "", "project holding the issue, when not --project")
			fs.StringVar(&a.control, "control", "", "program repository holding control/ files; empty is the system repository beside the checkout")
			fs.StringVar(&a.ref, "ref", "main", "ref of the control files")
		},
		Run: func(ctx *cli.Context) error {
			if a.project == "" || a.iid == 0 || (a.person == "") == !a.clear {
				return fmt.Errorf("%w: reviewer needs --project, --issue, and one of --person or --clear", cli.ErrUsage)
			}
			if strings.ContainsAny(a.person, " \t/\\") {
				return fmt.Errorf("%w: %q is not a username", cli.ErrUsage, a.person)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			p, err := LoadPolicy(t, ControlFor(a.control, ".", a.project), a.ref)
			if err != nil {
				return err
			}
			issueProject := a.issueProject
			if issueProject == "" {
				issueProject = a.project
			}
			r, err := NameReviewer(t, a.project, issueProject, a.iid, a.person, a.force, p, ctx.DryRun)
			if err != nil {
				return err
			}
			action := r.Action
			if r.DryRun && action != "unchanged" {
				action = "would be " + action
			}
			h := fmt.Sprintf("%s reviewer %s: %s\n", r.Ref, orNone(a.person), action)
			if r.Note != "" {
				h += r.Note + "\n"
			}
			return ctx.Emit(r, h)
		},
	})
}
