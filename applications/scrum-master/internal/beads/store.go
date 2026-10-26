// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	jitter "math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// pushAttempts bounds how often a write replays after its push is
// rejected by a newer tracker repository; between attempts it waits a
// growing, jittered delay so contending writers stop colliding in lockstep.
// A writer that keeps losing needs many quick retries more than long
// waits, so the delay is capped low and the attempts are many: the bound on
// a write's wait is about two minutes (mage integration:beadsScale, eng04).
const (
	pushAttempts  = 60
	pushBaseDelay = 200 * time.Millisecond
	pushMaxDelay  = 2 * time.Second
)

// Store is one working clone of the tracker repository, used on behalf of
// one person. Its files are a cache; the tracker repository is the source
// of truth (srd042 R3.1).
type Store struct {
	Dir  string
	User string
	// Branch is the tracker repository's branch holding .beads/, main when
	// empty: a program of one repository keeps it on a dedicated branch of
	// its code repository (srd042 R3.1).
	Branch string
	Now    func() time.Time
	// beforeWrite runs between a write's synchronization and its commit, so
	// a test can land another clone's write inside that window.
	beforeWrite func()
	// beforePush runs before each push of a write, so a test can make the
	// tracker repository move under it as many times as it likes.
	beforePush func()
	// sleep waits between rejected pushes; tests replace it.
	sleep func(time.Duration)
	// last is the most recent synchronization, what the last read reflects.
	last *Freshness
	// statesLoader reads the program's control/states.md rows once, on the
	// first transition; nil, or a loader finding no file, keeps the embedded
	// default Table (srd041 R8).
	statesLoader func() ([]Row, error)
	loadStates   sync.Once
	stateRows    []Row
	statesErr    error
}

// LoadStatesFrom sets where the store reads the program's transitions table,
// read once on the first transition.
func (s *Store) LoadStatesFrom(loader func() ([]Row, error)) { s.statesLoader = loader }

// states is the transitions table in force: the loaded rows, else the
// embedded default. A table that fails to load refuses every transition
// with its reason, so a malformed control/states.md never silently
// defaults.
func (s *Store) states() ([]Row, error) {
	s.loadStates.Do(func() {
		if s.statesLoader == nil {
			return
		}
		s.stateRows, s.statesErr = s.statesLoader()
	})
	if s.statesErr != nil {
		return nil, fmt.Errorf("%s: %w", StatesFile, s.statesErr)
	}
	if s.stateRows == nil {
		return Table, nil
	}
	return s.stateRows, nil
}

// Last is the most recent synchronization, if any.
func (s *Store) Last() (Freshness, bool) {
	if s.last == nil {
		return Freshness{}, false
	}
	return *s.last, true
}

// NewStore returns the store for the clone at dir, acting as user.
func NewStore(dir, user string) *Store {
	return &Store{Dir: dir, User: user, Now: time.Now}
}

// branch is the tracker branch the store reads and writes.
func (s *Store) branch() string {
	if s.Branch == "" {
		return DefaultBranch
	}
	return s.Branch
}

// Freshness names the tracker repository commit a read reflects. Offline
// means the clone could not synchronize and served its cache, last pulled
// at At (srd042 R3.3).
type Freshness struct {
	Commit  string    `json:"commit"`
	At      time.Time `json:"at"`
	Offline bool      `json:"offline,omitempty"`
}

// PopResult is what a pop reports. A pop taken offline is Provisional until
// Reconcile confirms it or reports it lost; an online pop whose push lost to
// an earlier taker is Lost (eng04-beads-sync).
type PopResult struct {
	ID          string    `json:"id"`
	Assignee    string    `json:"assignee"`
	Provisional bool      `json:"provisional,omitempty"`
	Lost        bool      `json:"lost,omitempty"`
	LostTo      string    `json:"lost_to,omitempty"`
	Read        Freshness `json:"read"`
}

// CloseResult is what a close reports; a repeated close changes nothing.
type CloseResult struct {
	ID      string    `json:"id"`
	Changed bool      `json:"changed"`
	Read    Freshness `json:"read"`
}

// Reconciled is the outcome of one provisional pop once the clone is back
// online: "confirmed" or "lost to <taker>".
type Reconciled struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
}

// ReconcileResult is what reconcile did: the queued writes it pushed, by
// their commit subjects, the tracker commit after the push, and each
// provisional pop's outcome.
type ReconcileResult struct {
	Pushed     []string     `json:"pushed"`
	Commit     string       `json:"tracker_commit"`
	Reconciled []Reconciled `json:"reconciled"`
}

// syncState is what a clone remembers between commands, kept under its git
// directory so it is never tracked.
type syncState struct {
	Commit      string        `json:"commit"`
	At          time.Time     `json:"at"`
	Provisional []Provisional `json:"provisional,omitempty"`
}

// Provisional is a pop taken while offline.
type Provisional struct {
	ID   string `json:"id"`
	User string `json:"user"`
}

// Issues pulls the tracker repository and returns every issue with the
// commit it reflects.
func (s *Store) Issues() ([]Issue, Freshness, error) {
	fr, err := s.Sync()
	if err != nil {
		return nil, fr, err
	}
	issues, err := s.read()
	return issues, fr, err
}

// Find pulls the tracker repository and returns the issue numbered iid in
// repository. In the files layout it reads the index entry and that one
// issue; an entry that is missing or names another issue falls back to
// reading them all, so a damaged index costs time, never the answer.
func (s *Store) Find(repository string, iid int) (Issue, bool, error) {
	if _, err := s.Sync(); err != nil {
		return Issue{}, false, err
	}
	if s.files() {
		if id := indexID(s.Dir, repository, iid); id != "" {
			if i, err := s.issue(id); err == nil && i.Repository == repository && i.IID == iid {
				return i, true, nil
			}
		}
	}
	issues, err := s.read()
	if err != nil {
		return Issue{}, false, err
	}
	for _, i := range issues {
		if i.Repository == repository && i.IID == iid {
			return i, true, nil
		}
	}
	return Issue{}, false, nil
}

// Pop takes a ready issue for the store's user (srd041 R9.1). A second pop
// by the same person changes nothing; a pop of someone else's issue is
// refused.
func (s *Store) Pop(id string) (PopResult, error) {
	r := PopResult{ID: id}
	tries := 0
	fr, written, queued, err := s.mutate(id, "pop", func(i *Issue) (bool, error) {
		tries++
		if i.Assignee == s.User {
			return false, nil
		}
		if i.Assignee != "" && tries > 1 {
			// A replay after a rejected push finds the issue taken by
			// the writer who pushed first: the race is lost, reported
			// below, not a pop of someone else's issue.
			return false, nil
		}
		if i.Assignee != "" {
			return false, fmt.Errorf("%s is assigned to %s; only that person pops it", id, i.Assignee)
		}
		return true, s.transition(i, ActorPop, Taken, s.User)
	})
	r.Read = fr
	if err != nil {
		return r, err
	}
	if written && queued {
		r.Provisional = true
		r.Assignee = s.User
		return r, s.update(func(st *syncState) {
			st.Provisional = append(st.Provisional, Provisional{ID: id, User: s.User})
		})
	}
	i, err := s.issue(id)
	if err != nil {
		return r, err
	}
	r.Assignee = i.Assignee
	if i.Assignee != s.User {
		r.Lost, r.LostTo = true, i.Assignee
	}
	return r, nil
}

// Close closes an issue on the merge that delivered it, recording the
// merge commit as the closing comment (srd042 R2.3): a merge closes any open
// issue it cites (srd041 R7.1).
func (s *Store) Close(id, commit string) (CloseResult, error) {
	fr, written, _, err := s.mutate(id, "close", func(i *Issue) (bool, error) {
		if i.State == Closed && i.CloseReason == commit {
			return false, nil
		}
		if err := s.transition(i, ActorMerge, Closed, s.User); err != nil {
			return false, err
		}
		now := s.stamp()
		i.ClosedAt, i.CloseReason = now, commit
		i.Comments = append(i.Comments, Comment{ID: newID(), IssueID: i.ID, Author: s.User, CreatedAt: now,
			Text: "Closed by merge commit " + commit + "."})
		return true, nil
	})
	return CloseResult{ID: id, Changed: written, Read: fr}, err
}

// Fire makes one srd041 transition as actor on behalf of the store's user,
// or refuses it.
func (s *Store) Fire(id, actor, to string) error {
	_, _, _, err := s.mutate(id, actor+" "+to, func(i *Issue) (bool, error) {
		return true, s.transition(i, actor, to, s.User)
	})
	return err
}

// Comment appends a comment by the store's user.
func (s *Store) Comment(id, text string) error {
	_, _, _, err := s.mutate(id, "comment", func(i *Issue) (bool, error) {
		i.Comments = append(i.Comments, Comment{ID: newID(), IssueID: i.ID, Author: s.User, CreatedAt: s.stamp(), Text: text})
		return true, nil
	})
	return err
}

// Reconcile is the first synchronization back online: it merges, pushes the
// writes queued offline, and reports each provisional pop as confirmed or
// lost to the taker who pushed first.
func (s *Store) Reconcile() (ReconcileResult, error) {
	var r ReconcileResult
	fr, err := s.Sync()
	if err != nil {
		return r, err
	}
	if fr.Offline {
		return r, fmt.Errorf("the tracker repository is still unreachable; last pulled %s at %s", fr.Commit, fr.At.Format(time.RFC3339))
	}
	if log, err := s.git("log", "--reverse", "--format=%s", "origin/"+s.branch()+"..HEAD"); err != nil {
		return r, err
	} else if log != "" {
		r.Pushed = strings.Split(log, "\n")
	}
	if queued, err := s.push(); err != nil {
		return r, err
	} else if queued {
		return r, errors.New("the tracker repository became unreachable during reconcile")
	}
	if r.Commit, err = s.git("rev-parse", "HEAD"); err != nil {
		return r, err
	}
	if err := s.synced(r.Commit); err != nil {
		return r, err
	}
	st, err := s.state()
	if err != nil {
		return r, err
	}
	for _, p := range st.Provisional {
		i, err := s.issue(p.ID)
		if err != nil {
			return r, err
		}
		outcome := "confirmed"
		if i.Assignee != p.User {
			outcome = "lost to " + i.Assignee
		}
		r.Reconciled = append(r.Reconciled, Reconciled{ID: p.ID, Outcome: outcome})
	}
	return r, s.update(func(st *syncState) { st.Provisional = nil })
}

// synced records commit, which the clone just pushed, as both this command's
// read and the last commit the clone and its origin agree on, so a later
// command that cannot reach the origin names it (srd042 R3.3).
func (s *Store) synced(commit string) error {
	at := s.Now().UTC()
	s.last = &Freshness{Commit: commit, At: at}
	return s.update(func(st *syncState) { st.Commit, st.At = commit, at })
}

// Sync pulls the tracker repository into the clone, merging by the eng04
// rules when both have moved. A clone that cannot reach the tracker
// repository is not an error: it reports Offline with the last pull.
func (s *Store) Sync() (Freshness, error) {
	st, err := s.state()
	if err != nil {
		return Freshness{}, err
	}
	if _, err := s.git("fetch", "-q", "origin"); err != nil {
		fr := Freshness{Commit: st.Commit, At: st.At, Offline: true}
		s.last = &fr
		return fr, nil
	}
	remote, err := s.git("rev-parse", "--verify", "-q", "origin/"+s.branch())
	if err != nil {
		return Freshness{}, fmt.Errorf("the tracker repository has no main branch: %w", err)
	}
	head, err := s.git("rev-parse", "HEAD")
	if err != nil {
		return Freshness{}, err
	}
	base, err := s.git("merge-base", "HEAD", remote)
	if err != nil {
		return Freshness{}, err
	}
	switch {
	case head == remote, base == remote:
	case base == head:
		if _, err := s.git("merge", "-q", "--ff-only", remote); err != nil {
			return Freshness{}, err
		}
	default:
		if err := s.merge(base, remote); err != nil {
			return Freshness{}, err
		}
	}
	at := s.Now()
	if err := s.update(func(st *syncState) { st.Commit, st.At = remote, at }); err != nil {
		return Freshness{}, err
	}
	fr := Freshness{Commit: remote, At: at}
	s.last = &fr
	return fr, nil
}

// merge records a merge with the tracker repository whose content is the
// eng04 merge of the tracker file, not git's line merge.
func (s *Store) merge(base, remote string) error {
	if s.files() {
		return s.mergeFiles(base, remote)
	}
	b, err := s.show(base)
	if err != nil {
		return err
	}
	o, err := s.show("HEAD")
	if err != nil {
		return err
	}
	t, err := s.show(remote)
	if err != nil {
		return err
	}
	rows, err := s.states()
	if err != nil {
		return err
	}
	merged := mergeIssues(rows, b, o, t, s.stamp())
	if _, err := s.git("merge", "-q", "--no-ff", "--no-commit", "-s", "ours", remote); err != nil {
		return err
	}
	if err := writeIssues(filepath.Join(s.Dir, IssuesFile), merged); err != nil {
		return err
	}
	if err := mergeRecordFile[Milestone](s, MilestonesFile, base, remote); err != nil {
		return err
	}
	if err := mergeRecordFile[Label](s, LabelsFile, base, remote); err != nil {
		return err
	}
	if _, err := s.git("add", "-A", "--", ".beads"); err != nil {
		return err
	}
	_, err = s.git("commit", "-q", "-m", "scrum-master: merge the tracker repository")
	return err
}

// Create files a new issue in repository with the next IID there, and
// returns it as the tracker repository holds it after the push: a create
// that raced another clone's has been renumbered by the merge. Its ID is the
// tracker's issue prefix and eight hex digits.
func (s *Store) Create(i Issue) (Issue, error) { return s.CreateWith(i, nil) }

// CreateWith is Create with finish applied to the new issue in the same
// write, after its ID, IID, author, and first state are set.
func (s *Store) CreateWith(i Issue, finish func(*Issue) error) (Issue, error) {
	prefix, err := s.prefix()
	if err != nil {
		return Issue{}, err
	}
	i.ID = prefix + "-" + newID()[:8]
	template := i
	start := func(top int) error {
		// A rejected write replays from the issue as given, so finish
		// applies once to the issue that lands.
		i = template
		i.IID = top + 1
		now := s.stamp()
		i.CreatedAt, i.UpdatedAt, i.CreatedBy = now, now, s.User
		if i.State == "" {
			i.State, i.Status = Open, StatusOpen
		}
		if finish != nil {
			return finish(&i)
		}
		return nil
	}
	if s.files() {
		// One file per issue: the write numbers the issue from the
		// index and writes only its file and entry.
		_, _, _, err = s.writeFiles("create "+i.ID, func() (bool, error) {
			if _, err := ensureIndex(s.Dir); err != nil {
				return false, err
			}
			top, err := topIID(s.Dir, i.Repository)
			if err != nil {
				return false, err
			}
			if err := start(top); err != nil {
				return false, err
			}
			return writeChanged(s.Dir, nil, []Issue{i})
		})
	} else {
		_, _, _, err = s.write("create "+i.ID, func(issues *[]Issue) (bool, error) {
			top := 0
			for _, x := range *issues {
				if x.Repository == i.Repository {
					top = max(top, x.IID)
				}
			}
			if err := start(top); err != nil {
				return false, err
			}
			*issues = append(*issues, i)
			return true, nil
		})
	}
	if err != nil {
		return Issue{}, err
	}
	return s.issue(i.ID)
}

// mutate is every write to one issue: pull, apply fn, commit, push.
func (s *Store) mutate(id, what string, fn func(*Issue) (bool, error)) (fr Freshness, written, queued bool, err error) {
	if s.files() {
		// One file per issue: the write reads and rewrites only this one.
		return s.writeFiles(what+" "+id, func() (bool, error) {
			i, err := s.issue(id)
			if err != nil {
				return false, err
			}
			changed, err := fn(&i)
			if err != nil || !changed {
				return false, err
			}
			i.UpdatedAt = s.stamp()
			return writeChanged(s.Dir, nil, []Issue{i})
		})
	}
	return s.write(what+" "+id, func(issues *[]Issue) (bool, error) {
		for k := range *issues {
			if (*issues)[k].ID == id {
				changed, err := fn(&(*issues)[k])
				if changed && err == nil {
					(*issues)[k].UpdatedAt = s.stamp()
				}
				return changed, err
			}
		}
		return false, fmt.Errorf("issue %s: %w", id, ErrNotFound)
	})
}

// write is every write to the issues: pull, apply fn, commit, push.
func (s *Store) write(what string, fn func(*[]Issue) (bool, error)) (fr Freshness, written, queued bool, err error) {
	return s.writeFiles(what, func() (bool, error) {
		issues, err := s.read()
		if err != nil {
			return false, err
		}
		files := s.files()
		var before map[string][]byte
		if files {
			if before, err = encodedAll(issues); err != nil {
				return false, err
			}
		}
		changed, err := fn(&issues)
		if err != nil || !changed {
			return false, err
		}
		if files {
			return writeChanged(s.Dir, before, issues)
		}
		return true, writeIssues(filepath.Join(s.Dir, IssuesFile), issues)
	})
}

// writeFiles is every write: pull, let fn change the tracker files, commit,
// push. A push the tracker repository rejects, because another writer
// pushed first, is not merged: the write's commit is dropped and the write
// replays on the newer tracker, fn reading it afresh, so every retry takes
// the same fast path as the first try and a create numbers its issue after
// the other writer's, never colliding with it (eng04). A write made
// offline, or whose retry finds the repository unreachable, is committed
// and queued for the next synchronization, which merges it.
func (s *Store) writeFiles(what string, fn func() (bool, error)) (fr Freshness, written, queued bool, err error) {
	delay := pushBaseDelay
	var last error
	for attempt := range pushAttempts {
		if attempt > 0 {
			s.wait(delay)
			delay = min(2*delay, pushMaxDelay)
		}
		if fr, err = s.Sync(); err != nil {
			return fr, false, false, err
		}
		if attempt == 0 && s.beforeWrite != nil {
			s.beforeWrite()
		}
		changed, err := fn()
		if err != nil || !changed {
			return fr, false, false, err
		}
		before, err := s.git("rev-parse", "HEAD")
		if err != nil {
			return fr, false, false, err
		}
		if _, err := s.git("add", "-A", "--", ".beads"); err != nil {
			return fr, false, false, err
		}
		if _, err := s.git("commit", "-q", "-m", fmt.Sprintf("scrum-master: %s (%s)", what, s.User)); err != nil {
			return fr, false, false, err
		}
		if fr.Offline {
			return fr, true, true, nil
		}
		if s.beforePush != nil {
			s.beforePush()
		}
		if _, last = s.git("push", "-q", "origin", "HEAD:"+s.branch()); last == nil {
			// The read a command reports is now its own write, not the
			// pull before it.
			if head, err := s.git("rev-parse", "HEAD"); err == nil {
				if err := s.synced(head); err != nil {
					return fr, true, false, err
				}
			}
			return fr, true, false, nil
		}
		if _, ferr := s.git("fetch", "-q", "origin"); ferr != nil {
			// Unreachable now: the commit stays, queued.
			return Freshness{Commit: fr.Commit, At: fr.At, Offline: true}, true, true, nil
		}
		// Rejected: drop this attempt's commit and replay. Writes queued
		// offline sit below before and stay queued.
		if _, rerr := s.git("reset", "-q", "--hard", before); rerr != nil {
			return fr, false, false, errors.Join(last, rerr)
		}
	}
	return fr, false, false, fmt.Errorf("push to the tracker repository rejected %d times; nothing was written (last: %v)", pushAttempts, last)
}

// push lands the clone's queued commits in the tracker repository when
// reconcile runs, merging and retrying when the repository moved since the
// last pull: a write made offline cannot replay, so it merges. It reports
// queued when the repository turned out to be unreachable.
func (s *Store) push() (queued bool, err error) {
	delay := pushBaseDelay
	var last error
	for range pushAttempts {
		if _, last = s.git("push", "-q", "origin", "HEAD:"+s.branch()); last == nil {
			return false, nil
		}
		s.wait(delay)
		delay = min(2*delay, pushMaxDelay)
		fr, err := s.Sync()
		if err != nil {
			return false, err
		}
		if fr.Offline {
			return true, nil
		}
	}
	return false, fmt.Errorf("push to the tracker repository rejected %d times; nothing was written (last: %v)", pushAttempts, last)
}

// wait sleeps a random duration up to d, so writers that collided retry at
// different moments.
func (s *Store) wait(d time.Duration) {
	sleep := s.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	sleep(time.Duration(jitter.Int64N(int64(d)) + 1))
}

func (s *Store) read() ([]Issue, error) {
	if s.files() {
		return readIssueDir(s.Dir)
	}
	b, err := os.ReadFile(filepath.Join(s.Dir, IssuesFile))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseIssues(b)
}

func (s *Store) issue(id string) (Issue, error) {
	if s.files() {
		b, err := os.ReadFile(issuePath(s.Dir, id))
		if errors.Is(err, os.ErrNotExist) {
			return Issue{}, fmt.Errorf("issue %s: %w", id, ErrNotFound)
		}
		if err != nil {
			return Issue{}, err
		}
		return decodeIssue(issuePath(s.Dir, id), b)
	}
	issues, err := s.read()
	if err != nil {
		return Issue{}, err
	}
	for _, i := range issues {
		if i.ID == id {
			return i, nil
		}
	}
	return Issue{}, fmt.Errorf("issue %s: %w", id, ErrNotFound)
}

func (s *Store) show(rev string) ([]Issue, error) {
	out, err := s.git("show", rev+":"+IssuesFile)
	if err != nil {
		return nil, nil
	}
	return parseIssues([]byte(out))
}

func (s *Store) stamp() string { return s.Now().UTC().Format(time.RFC3339) }

func (s *Store) statePath() (string, error) {
	dir, err := s.git("rev-parse", "--absolute-git-dir")
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "scrum-master-beads-sync.json"), nil
}

func (s *Store) state() (syncState, error) {
	var st syncState
	path, err := s.statePath()
	if err != nil {
		return st, err
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal(b, &st)
}

func (s *Store) update(fn func(*syncState)) error {
	st, err := s.state()
	if err != nil {
		return err
	}
	fn(&st)
	path, err := s.statePath()
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (s *Store) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.Dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
