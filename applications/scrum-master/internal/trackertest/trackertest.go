// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package trackertest runs one test against every tracker scrum-master serves: the
// in-memory fake and a beads tracker over real git repositories on local disk
// (srd042 R1.1). A test describes its program through Program and asserts
// through the tracker interface, never a backend's internals.
package trackertest

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads/beadstest"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// User is the person every Program's tracker acts as, the fake's Me.
const User = "me"

// Program is a tracker and the repositories it serves. Files and tags are
// declared first; Tracker builds them on its first call.
type Program interface {
	AddFile(project, ref, path string, content []byte)
	AddTag(project, tag string)
	Tracker() tracker.Tracker
	// Writes counts the writes the tracker has made, so a test can prove a
	// dry run or a second run changed nothing.
	Writes() int
	// As is the program's tracker acting as another person, for a step
	// someone other than User takes, such as a peer read.
	As(user string) tracker.Tracker
	// AddMerge declares a merge into project: on beads a
	// --no-ff merge commit on its default branch, numbered mr.IID, by
	// mr.Author on mr.MergedAt, with mr.Title and mr.Description as its
	// message.
	AddMerge(project string, mr tracker.Merge)
	// AddRepository declares a project's repository with nothing in it, for
	// a test that works in its checkout.
	AddRepository(project string)
	// Checkout is a project's local checkout once Tracker has built it; the
	// fake has none and returns the empty string.
	Checkout(project string) string
}

// Each runs test once per tracker, as subtests named fake and beads.
func Each(t *testing.T, test func(t *testing.T, p Program)) {
	t.Run("fake", func(t *testing.T) { test(t, fakeProgram{fake.New()}) })
	t.Run("beads", func(t *testing.T) { test(t, newBeadsProgram(t)) })
}

type fakeProgram struct{ *fake.Fake }

func (f fakeProgram) Tracker() tracker.Tracker { return f.Fake }
func (f fakeProgram) Writes() int              { return f.Fake.Writes }
func (f fakeProgram) AddMerge(project string, mr tracker.Merge) {
	mr.State = "merged"
	f.Fake.MRs[project] = append(f.Fake.MRs[project], &mr)
}

func (f fakeProgram) AddRepository(string)   {}
func (f fakeProgram) Checkout(string) string { return "" }

func (f fakeProgram) As(user string) tracker.Tracker {
	return fakeAs{f.Fake, user}
}

// fakeAs is the fake acting as another person: it is who CurrentUser
// names and who authors the notes it posts.
type fakeAs struct {
	*fake.Fake
	user string
}

func (f fakeAs) CurrentUser() (tracker.User, error) { return tracker.User{Username: f.user}, nil }

func (f fakeAs) CreateNote(project string, iid int, body string) error {
	me := f.Fake.Me
	f.Fake.Me = tracker.User{Username: f.user}
	defer func() { f.Fake.Me = me }()
	return f.Fake.CreateNote(project, iid, body)
}

// beadsProgram builds each project as a code repository named for the
// project's last path segment, siblings in one directory, which is how the
// beads tracker finds a project's checkout.
type beadsProgram struct {
	t       *testing.T
	root    string
	remote  string
	files   map[string]map[string]map[string][]byte // project, ref, path
	tags    map[string][]string
	merges  map[string][]tracker.Merge
	repos   map[string]bool
	tracker *beads.Tracker
	work    string
	as      map[string]*beads.Tracker
}

// NewBeads is the beads program alone, for a test that sets the tracker
// clone's clock or reads beads-only state.
func NewBeads(t *testing.T) Program { return newBeadsProgram(t) }

func newBeadsProgram(t *testing.T) *beadsProgram {
	root := t.TempDir()
	return &beadsProgram{t: t, root: root, remote: beadstest.TrackerRepository(t, root),
		files: map[string]map[string]map[string][]byte{}, tags: map[string][]string{},
		merges: map[string][]tracker.Merge{}, repos: map[string]bool{}}
}

func (b *beadsProgram) AddMerge(project string, mr tracker.Merge) {
	if b.tracker != nil {
		b.t.Fatal("trackertest: AddMerge after Tracker")
	}
	b.merges[project] = append(b.merges[project], mr)
}

func (b *beadsProgram) AddRepository(project string) {
	if b.tracker != nil {
		b.t.Fatal("trackertest: AddRepository after Tracker")
	}
	b.repos[project] = true
}

func (b *beadsProgram) Checkout(project string) string {
	b.Tracker()
	return filepath.Join(b.root, path.Base(project))
}

func (b *beadsProgram) AddFile(project, ref, p string, content []byte) {
	if b.tracker != nil {
		b.t.Fatal("trackertest: AddFile after Tracker")
	}
	if b.files[project] == nil {
		b.files[project] = map[string]map[string][]byte{}
	}
	if b.files[project][ref] == nil {
		b.files[project][ref] = map[string][]byte{}
	}
	b.files[project][ref][p] = content
}

func (b *beadsProgram) AddTag(project, tag string) {
	if b.tracker != nil {
		b.t.Fatal("trackertest: AddTag after Tracker")
	}
	b.tags[project] = append(b.tags[project], tag)
}

func (b *beadsProgram) Tracker() tracker.Tracker {
	if b.tracker != nil {
		return b.tracker
	}
	t := b.t
	t.Helper()
	projects := map[string]bool{}
	for p := range b.files {
		projects[p] = true
	}
	for p := range b.tags {
		projects[p] = true
	}
	for p := range b.merges {
		projects[p] = true
	}
	for p := range b.repos {
		projects[p] = true
	}
	work := beadstest.CodeRepository(t, b.root, "work", User)
	for project := range projects {
		b.build(project)
	}
	clone := filepath.Join(b.root, User+"-tracker")
	beadstest.Clone(t, b.remote, clone, User)
	b.tracker, b.work = beads.NewTracker(clone, User, work), work
	return b.tracker
}

// As gives another person their own clone of the tracker repository.
func (b *beadsProgram) As(user string) tracker.Tracker {
	b.Tracker()
	if tr, ok := b.as[user]; ok {
		return tr
	}
	clone := filepath.Join(b.root, user+"-tracker")
	beadstest.Clone(b.t, b.remote, clone, user)
	if b.as == nil {
		b.as = map[string]*beads.Tracker{}
	}
	b.as[user] = beads.NewTracker(clone, user, b.work)
	return b.as[user]
}

// build makes one project's repository: main holds the files declared at
// main; every other ref is a branch from main holding its own files too,
// and a ref that is also a tag becomes that tag instead of a branch.
func (b *beadsProgram) build(project string) {
	t := b.t
	dir := beadstest.CodeRepository(t, b.root, path.Base(project), User)
	commit := func(files map[string][]byte, msg string) {
		for p, content := range files {
			full := filepath.Join(dir, p)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, content, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		beadstest.Git(t, dir, "add", "-A")
		beadstest.Git(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
	}
	if files := b.files[project]["main"]; len(files) > 0 {
		commit(files, "main files")
	}
	b.merge(dir, b.merges[project])
	beadstest.Git(t, dir, "push", "-q", "origin", "main")
	var refs []string
	for ref := range b.files[project] {
		if ref != "main" {
			refs = append(refs, ref)
		}
	}
	for _, tag := range b.tags[project] {
		if !slices.Contains(refs, tag) {
			refs = append(refs, tag)
		}
	}
	slices.Sort(refs)
	for _, ref := range refs {
		beadstest.Git(t, dir, "checkout", "-q", "-b", "build-"+ref, "main")
		commit(b.files[project][ref], ref+" files")
		if slices.Contains(b.tags[project], ref) {
			beadstest.Git(t, dir, "tag", ref)
			beadstest.Git(t, dir, "push", "-q", "origin", "refs/tags/"+ref)
		} else {
			beadstest.Git(t, dir, "branch", "-q", "-m", ref)
			beadstest.Git(t, dir, "push", "-q", "origin", ref)
		}
		beadstest.Git(t, dir, "checkout", "-q", "main")
	}
}

// Writes counts the commits the tracker repository gained after its seed.
func (b *beadsProgram) Writes() int {
	out, err := exec.Command("git", "--git-dir", b.remote, "rev-list", "--count", "main").Output()
	if err != nil {
		b.t.Fatal(err)
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n - 1
}

// merge lands each declared merge as a --no-ff merge on main, in
// IID order, with filler merges dated long before any test's window so
// that each lands at its declared number: on beads a merge's number is its
// place in the default branch's first-parent history.
func (b *beadsProgram) merge(dir string, mrs []tracker.Merge) {
	t := b.t
	slices.SortFunc(mrs, func(a, c tracker.Merge) int { return a.IID - c.IID })
	n := 0
	land := func(title, body, author, date string) {
		n++
		branch := "merge-" + strconv.Itoa(n)
		beadstest.Git(t, dir, "checkout", "-q", "-b", branch, "main")
		beadstest.Git(t, dir, "commit", "-q", "--allow-empty", "-m", "work for "+branch)
		beadstest.Git(t, dir, "checkout", "-q", "main")
		msg := title
		if body != "" {
			msg += "\n\n" + body
		}
		cmd := exec.Command("git", "merge", "-q", "--no-ff", branch, "-m", msg)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME="+author, "GIT_AUTHOR_EMAIL="+author+"@example.com",
			"GIT_AUTHOR_DATE="+date+"T12:00:00Z", "GIT_COMMITTER_DATE="+date+"T12:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("merge %s: %v: %s", branch, err, out)
		}
	}
	for _, mr := range mrs {
		for n+1 < mr.IID {
			land("Earlier merge", "", "earlier", "2000-01-01")
		}
		land(mr.Title, mr.Description, mr.Author, mr.MergedAt)
	}
}
