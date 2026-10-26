// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// statusInProgress is the label commands use for in-progress; the
// beads tracker shows it from the state and fires the transition when a
// command adds or removes it.
const statusInProgress = "status::in-progress"

// Tracker serves scrum-master's tracker interface over beads (srd042 R1.1): the issue
// half from the tracker repository, the code half from the git checkout the
// command runs in. A project path is an issue's Repository; the code half
// serves its one checkout whatever project it is asked about.
type Tracker struct {
	Store *Store
	// Code is the code half: each project's checkout and its origin, which
	// every tracking system reads the same way.
	*checkout.Code
}

var (
	_ tracker.Tracker = (*Tracker)(nil)
	_ tracker.Reader  = (*Tracker)(nil)
	_ tracker.Triager = (*Tracker)(nil)
)

// LastRead names the tracker commit the last read reflected.
func (t *Tracker) LastRead() (tracker.Read, bool) {
	fr, ok := t.Store.Last()
	if !ok {
		return tracker.Read{}, false
	}
	return tracker.Read{Commit: fr.Commit, At: fr.At.UTC().Format(time.RFC3339), Offline: fr.Offline}, true
}

// NewTracker serves the tracker repository cloned at clone, as user, with
// the code half answered by the checkout at code. The transitions table is
// the checkout's control/states.md on main, read on the first transition;
// without one, the embedded default (srd041 R8).
func NewTracker(clone, user, code string) *Tracker {
	t := &Tracker{Store: NewStore(clone, user), Code: &checkout.Code{Dir: code}}
	t.Store.LoadStatesFrom(func() ([]Row, error) {
		b, err := t.Code.GetFile("", StatesFile, "main")
		if errors.Is(err, tracker.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return ParseStates(b)
	})
	return t
}

// DefaultBranch is the tracker repository's branch when none is named.
const DefaultBranch = "main"

// BranchEnv names, for one person's run, the tracker branch in place of the
// one control/trackers.md names (config format trackers).
const BranchEnv = "SCRUM_MASTER_TRACKER_BRANCH"

// Configured reports whether SCRUM_MASTER_TRACKER names a tracker repository.
func Configured() bool { return strings.TrimSpace(os.Getenv("SCRUM_MASTER_TRACKER")) != "" } //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.

// FromEnv opens the beads tracker SCRUM_MASTER_TRACKER names. Its clone is
// SCRUM_MASTER_TRACKER_CLONE, else a directory under the user cache, cloned on first
// use; the person is SCRUM_MASTER_USER, else git's user.name; the code half serves
// the checkout around the working directory.
func FromEnv() (*Tracker, error) {
	return OpenRepository(strings.TrimSpace(os.Getenv("SCRUM_MASTER_TRACKER")), "") //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
}

// cachedClones is where OpenRepository clones a tracker no
// SCRUM_MASTER_TRACKER_CLONE names.
func cachedClones() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "scrum-master", "trackers"), nil
}

// OpenRepository opens the beads tracker on branch of the tracker repository
// remote: SCRUM_MASTER_TRACKER_BRANCH, else branch, else main. It is cloned
// at SCRUM_MASTER_TRACKER_CLONE or under the user cache directory, for
// SCRUM_MASTER_USER or git's user.name. A branch other than main is a
// single-branch clone, so a tracker on a branch of a code repository never
// fetches the code (srd042 R3.1).
func OpenRepository(remote, branch string) (*Tracker, error) {
	if env := strings.TrimSpace(os.Getenv(BranchEnv)); env != "" { //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
		branch = env
	}
	if branch == "" {
		branch = DefaultBranch
	}
	clone := strings.TrimSpace(os.Getenv("SCRUM_MASTER_TRACKER_CLONE")) //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
	if clone == "" {
		root, err := cachedClones()
		if err != nil {
			return nil, err
		}
		key := remote
		if branch != DefaultBranch {
			key += "#" + branch
		}
		sum := sha256.Sum256([]byte(key))
		clone = filepath.Join(root, hex.EncodeToString(sum[:6]))
	}
	// A clone of a local tracker that was deleted and made again at the same
	// path holds the old one's history: it is set aside, renamed, never
	// removed, since it may be that history's last copy, and the new tracker
	// is cloned in its place. A clone whose origin is simply gone is read as
	// it is (GH-141).
	if remade(clone) {
		if err := os.Rename(clone, clone+".stale-"+time.Now().UTC().Format("20060102T150405Z")); err != nil {
			return nil, err
		}
	}
	if _, err := os.Stat(filepath.Join(clone, ".git")); err != nil {
		if err := os.MkdirAll(filepath.Dir(clone), 0o755); err != nil {
			return nil, err
		}
		args := []string{"clone", "-q", remote, clone}
		if branch != DefaultBranch {
			args = []string{"clone", "-q", "--single-branch", "--branch", branch, remote, clone}
		}
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("clone the tracker repository %s: %w: %s", remote, err, strings.TrimSpace(string(out)))
		}
	}
	user := strings.TrimSpace(os.Getenv("SCRUM_MASTER_USER")) //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
	if user == "" {
		out, err := exec.Command("git", "config", "user.name").Output()
		if user = strings.TrimSpace(string(out)); err != nil || user == "" {
			return nil, errors.New("no person for the beads tracker: set SCRUM_MASTER_USER or git config user.name")
		}
	}
	code, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	t := NewTracker(clone, user, code)
	t.Store.Branch = branch
	return t, nil
}

func (t *Tracker) find(project string, iid int) (Issue, error) {
	i, ok, err := t.Store.Find(project, iid)
	if err != nil {
		return Issue{}, err
	}
	if !ok {
		return Issue{}, fmt.Errorf("%s#%d: %w", project, iid, tracker.ErrNotFound)
	}
	return i, nil
}

func (t *Tracker) view(i Issue) tracker.Issue {
	labels := slices.Clone(i.Labels)
	if i.State == InProgress {
		labels = append(labels, statusInProgress)
	}
	state := "opened"
	if i.Status == StatusClosed {
		state = "closed"
	}
	var assignees []string
	if i.Assignee != "" {
		assignees = []string{i.Assignee}
	}
	return tracker.Issue{Project: i.Repository, IID: i.IID, Title: i.Title, Description: i.Description, Labels: labels,
		State: state, WebURL: "beads:" + i.ID, DueDate: i.DueDate, Milestone: i.Release, Assignees: assignees,
		Author: i.CreatedBy, CreatedAt: i.CreatedAt, UpdatedAt: i.UpdatedAt, ClosedAt: i.ClosedAt}
}

func (t *Tracker) GetIssue(project string, iid int) (tracker.Issue, error) {
	i, err := t.find(project, iid)
	if err != nil {
		return tracker.Issue{}, err
	}
	return t.view(i), nil
}

func (t *Tracker) ListIssues(project string, q tracker.IssueQuery) ([]tracker.Issue, error) {
	issues, _, err := t.Store.Issues()
	if err != nil {
		return nil, err
	}
	var out []tracker.Issue
	for _, i := range issues {
		v := t.view(i)
		switch {
		case i.Repository != project,
			q.State == "opened" && v.State != "opened",
			q.State == "closed" && v.State != "closed",
			slices.ContainsFunc(q.Labels, func(l string) bool { return !v.HasLabel(l) }),
			q.Search != "" && !strings.Contains(strings.ToLower(v.Title+"\n"+v.Description), strings.ToLower(q.Search)),
			q.UpdatedAfter != "" && v.UpdatedAt < q.UpdatedAfter:
			continue
		}
		out = append(out, v)
	}
	slices.SortFunc(out, func(a, b tracker.Issue) int { return a.IID - b.IID })
	return out, nil
}

// CreateAdmitted creates the issue and records lint's verdict on it in one
// tracker write: the srd041 triage rows and, on a failing issue, admit's
// note as a comment. The dependencies its link lines name go in the same
// write; a Blocks line's target records its edge in a write of its own.
func (t *Tracker) CreateAdmitted(project string, in tracker.NewIssue, admit func(tracker.Issue) ([]string, string)) (tracker.Issue, error) {
	me := t.Store.User
	own, blocks, err := t.linked(in.Description)
	if err != nil {
		return tracker.Issue{}, err
	}
	i, err := t.Store.CreateWith(Issue{Repository: project, Title: in.Title, Description: in.Description,
		Labels: slices.Clone(in.Labels), DueDate: in.DueDate, Release: in.Milestone}, func(i *Issue) error {
		t.Store.depend(i, own)
		reasons, note := admit(t.view(*i))
		if _, err := t.Store.triage(i, len(reasons) == 0, me); err != nil {
			return err
		}
		if note != "" {
			i.Comments = append(i.Comments, Comment{ID: newID(), IssueID: i.ID, Author: me, CreatedAt: t.Store.stamp(), Text: note})
		}
		return nil
	})
	if err != nil {
		return tracker.Issue{}, err
	}
	if err := t.blockOn(blocks, i.ID); err != nil {
		return tracker.Issue{}, err
	}
	return t.view(i), nil
}

func (t *Tracker) CreateIssue(project string, in tracker.NewIssue) (tracker.Issue, error) {
	own, blocks, err := t.linked(in.Description)
	if err != nil {
		return tracker.Issue{}, err
	}
	i, err := t.Store.CreateWith(Issue{Repository: project, Title: in.Title, Description: in.Description,
		Labels: slices.Clone(in.Labels), DueDate: in.DueDate, Release: in.Milestone}, func(i *Issue) error {
		t.Store.depend(i, own)
		return nil
	})
	if err != nil {
		return tracker.Issue{}, err
	}
	if err := t.blockOn(blocks, i.ID); err != nil {
		return tracker.Issue{}, err
	}
	return t.view(i), nil
}

// UpdateIssue applies an update. Closing is a person's decline and
// reopening a person's reopen (srd041); the in-progress label fires the
// assignee's start and stop.
func (t *Tracker) UpdateIssue(project string, iid int, u tracker.IssueUpdate) (tracker.Issue, error) {
	found, err := t.find(project, iid)
	if err != nil {
		return tracker.Issue{}, err
	}
	me := t.Store.User
	var own []Dependency
	var blocks []string
	if u.Description != nil {
		// A description's link lines add the edges they name; an edge
		// is never removed here, as a Blocks line elsewhere may have made it.
		if own, blocks, err = t.linked(*u.Description); err != nil {
			return tracker.Issue{}, err
		}
	}
	_, _, _, err = t.Store.mutate(found.ID, "update", func(i *Issue) (bool, error) {
		t.Store.depend(i, own)
		if u.State != nil {
			to := map[string]string{"close": Closed, "reopen": Open}[*u.State]
			if to == "" {
				return false, fmt.Errorf("state %q is neither close nor reopen", *u.State)
			}
			if err := t.Store.transition(i, ActorPerson, to, me); err != nil {
				return false, err
			}
			if to == Closed {
				i.ClosedAt = t.Store.stamp()
				if u.CloseReason != nil {
					i.CloseReason = *u.CloseReason
				}
			}
		}
		if u.Description != nil {
			i.Description = *u.Description
		}
		if u.DueDate != nil {
			i.DueDate = *u.DueDate
		}
		if u.Milestone != nil {
			i.Release = *u.Milestone
		}
		for _, l := range u.AddLabels {
			if l == statusInProgress {
				if i.State != InProgress {
					if err := t.Store.transition(i, ActorPerson, InProgress, me); err != nil {
						return false, err
					}
				}
			} else if !i.HasLabel(l) {
				i.Labels = append(i.Labels, l)
			}
		}
		for _, l := range u.RemoveLabels {
			if l == statusInProgress {
				if i.State == InProgress {
					if err := t.Store.transition(i, ActorPerson, Taken, me); err != nil {
						return false, err
					}
				}
			} else {
				i.Labels = slices.DeleteFunc(i.Labels, func(x string) bool { return x == l })
			}
		}
		return true, nil
	})
	if err != nil {
		return tracker.Issue{}, err
	}
	if err := t.blockOn(blocks, found.ID); err != nil {
		return tracker.Issue{}, err
	}
	return t.GetIssue(project, iid)
}

func (t *Tracker) ListNotes(project string, iid int) ([]tracker.Note, error) {
	i, err := t.find(project, iid)
	if err != nil {
		return nil, err
	}
	notes := make([]tracker.Note, 0, len(i.Comments))
	for k, c := range i.Comments {
		notes = append(notes, tracker.Note{ID: k + 1, Body: c.Text, Author: c.Author, System: c.Author == "scrum-master" || c.Author == "tpm"})
	}
	return notes, nil
}

func (t *Tracker) CreateNote(project string, iid int, body string) error {
	i, err := t.find(project, iid)
	if err != nil {
		return err
	}
	return t.Store.Comment(i.ID, body)
}

func (t *Tracker) CurrentUser() (tracker.User, error) {
	return tracker.User{Username: t.Store.User}, nil
}

// AssignIssue is pop's assignment (srd041 R9.1): only the person running
// it, and a pop that lost to an earlier taker is an error naming them.
func (t *Tracker) AssignIssue(project string, iid int, user tracker.User) error {
	if user.Username != t.Store.User {
		return fmt.Errorf("%w: pop assigns only the person who runs it, not %s (srd041 R9.1)", ErrRefused, user.Username)
	}
	i, err := t.find(project, iid)
	if err != nil {
		return err
	}
	r, err := t.Store.Pop(i.ID)
	if err != nil {
		return err
	}
	if r.Lost {
		return fmt.Errorf("%s#%d was taken by %s first", project, iid, r.LostTo)
	}
	return nil
}

// Triage is admission as srd041's lint rows: an admitted open issue becomes
// ready, an admitted issue in needs-triage becomes ready or, already taken
// by its author, taken; a failing open issue goes to needs-triage. Only a
// data issue enters the backlog (srd041 R3.1), so admission moves only a
// data issue's state; for any other issue, and for an issue already past
// admission that later fails, for which srd041 has no row, lint sets or
// clears only the label.
func (t *Tracker) Triage(project string, iid int, admitted bool) (bool, error) {
	found, err := t.find(project, iid)
	if err != nil {
		return false, err
	}
	me := t.Store.User
	_, written, _, err := t.Store.mutate(found.ID, "lint", func(i *Issue) (bool, error) { return t.Store.triage(i, admitted, me) })
	return written, err
}

// triage is lint's srd041 rows on one issue: an admitted open data issue
// becomes ready, an admitted one in needs-triage ready or, already taken by
// its author, taken; a failing open data issue goes to needs-triage, and an
// issue on another plane carries the needs-triage label instead.
func (s *Store) triage(i *Issue, admitted bool, me string) (bool, error) {
	data := i.HasLabel("plane::data")
	switch {
	case !data && admitted && i.HasLabel(NeedsTriage):
		i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return l == NeedsTriage })
		return true, nil
	case !data && !admitted && !i.HasLabel(NeedsTriage):
		i.Labels = append(i.Labels, NeedsTriage)
		return true, nil
	case !data:
		return false, nil
	case admitted && (i.State == Open || i.State == NeedsTriage) && i.Assignee == "":
		return true, s.transition(i, ActorLint, Ready, me)
	case admitted && i.State == NeedsTriage:
		return true, s.transition(i, ActorLint, Taken, me)
	case admitted && i.HasLabel(NeedsTriage):
		i.Labels = slices.DeleteFunc(i.Labels, func(l string) bool { return l == NeedsTriage })
		return true, nil
	case !admitted && i.State == Open:
		return true, s.transition(i, ActorLint, NeedsTriage, me)
	case !admitted && !i.HasLabel(NeedsTriage):
		i.Labels = append(i.Labels, NeedsTriage)
		return true, nil
	}
	return false, nil
}

// CloseOnMerge closes an issue on the local merge that delivered it, citing
// the merge commit (srd042 R2.3). It reports whether anything changed.
func (t *Tracker) CloseOnMerge(project string, iid int, commit string) (bool, error) {
	i, err := t.find(project, iid)
	if err != nil {
		return false, err
	}
	r, err := t.Store.Close(i.ID, commit)
	return r.Changed, err
}

// FindMilestone looks in the project, then in its ancestor groups.
func (t *Tracker) FindMilestone(project, title string) (tracker.Milestone, error) {
	ms, _, err := t.Store.Milestones()
	if err != nil {
		return tracker.Milestone{}, err
	}
	for _, scope := range ancestors(project) {
		for _, m := range ms {
			if m.Scope == scope && m.Title == title {
				return toTracker(m), nil
			}
		}
	}
	return tracker.Milestone{}, fmt.Errorf("milestone %s for %s: %w", title, project, tracker.ErrNotFound)
}

// ActiveMilestones lists the milestones of the project and its ancestor
// groups by due date. A beads milestone is never closed, so all are active.
func (t *Tracker) ActiveMilestones(project string) ([]tracker.Milestone, error) {
	ms, _, err := t.Store.Milestones()
	if err != nil {
		return nil, err
	}
	scopes := ancestors(project)
	var out []tracker.Milestone
	for _, m := range ms {
		if slices.Contains(scopes, m.Scope) {
			out = append(out, toTracker(m))
		}
	}
	slices.SortStableFunc(out, func(a, b tracker.Milestone) int {
		return strings.Compare(a.DueDate+"\x00"+a.Title, b.DueDate+"\x00"+b.Title)
	})
	return out, nil
}

func (t *Tracker) EnsureMilestone(group string, m tracker.Milestone) (string, error) {
	return t.Store.EnsureMilestone(Milestone{Scope: group, Title: m.Title, Description: m.Description,
		StartDate: m.StartDate, DueDate: m.DueDate})
}

func toTracker(m Milestone) tracker.Milestone {
	return tracker.Milestone{Title: m.Title, Description: m.Description, StartDate: m.StartDate, DueDate: m.DueDate}
}

// IssueAwards is always empty: beads has no award emoji, and a peer read
// is an approval comment instead (srd015 R1.2).
func (t *Tracker) IssueAwards(string, int) ([]tracker.Award, error) { return nil, nil }

// GroupLabels lists the labels defined on the group itself.
func (t *Tracker) GroupLabels(group string) ([]tracker.Label, error) {
	ls, _, err := t.Store.Labels()
	if err != nil {
		return nil, err
	}
	var out []tracker.Label
	for _, l := range ls {
		if l.Scope == group {
			out = append(out, tracker.Label{Name: l.Name, Color: l.Color, Description: l.Description})
		}
	}
	return out, nil
}

func (t *Tracker) CreateGroupLabel(group string, l tracker.Label) error {
	return t.Store.PutLabel(Label{Scope: group, Name: l.Name, Color: l.Color, Description: l.Description}, false)
}

func (t *Tracker) UpdateGroupLabel(group string, l tracker.Label) error {
	err := t.Store.PutLabel(Label{Scope: group, Name: l.Name, Color: l.Color, Description: l.Description}, true)
	if errors.Is(err, ErrNotFound) {
		return fmt.Errorf("%w: %v", tracker.ErrNotFound, err)
	}
	return err
}

func (t *Tracker) checkout(project string) string { return t.Code.Checkout(project) }

func (t *Tracker) git(project string, args ...string) (string, error) {
	return t.Code.Git(project, args...)
}
