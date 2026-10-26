// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fixture is a tracker repository (bare, on local disk) and the clones the
// test opens against it. No network is involved: the remote is a path.
type fixture struct {
	t      *testing.T
	root   string
	remote string
	clock  time.Time
}

// newFixture creates the tracker repository holding issues as its first
// commit.
func newFixture(t *testing.T, issues ...Issue) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{t: t, root: root, remote: filepath.Join(root, "tracker.git"),
		clock: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)}
	f.git(root, "init", "-q", "--bare", "-b", "main", f.remote)
	seed := filepath.Join(root, "seed")
	f.git(root, "clone", "-q", f.remote, seed)
	f.identity(seed, "seed")
	if err := os.MkdirAll(filepath.Join(seed, ".beads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if fixtureFiles() {
		if err := WriteConfig(seed, Config{Layout: LayoutFiles}); err != nil {
			t.Fatal(err)
		}
		if len(issues) > 0 {
			if _, err := writeChanged(seed, nil, issues); err != nil {
				t.Fatal(err)
			}
		} else if err := os.MkdirAll(filepath.Join(seed, IssuesDir), 0o755); err != nil {
			t.Fatal(err)
		}
	} else if err := writeIssues(filepath.Join(seed, IssuesFile), issues); err != nil {
		t.Fatal(err)
	}
	f.git(seed, "add", "-A")
	f.git(seed, "commit", "-q", "-m", "seed")
	f.git(seed, "push", "-q", "origin", "HEAD:main")
	return f
}

// clone opens a new working clone of the tracker repository for user.
func (f *fixture) clone(user string) *Store {
	f.t.Helper()
	dir := filepath.Join(f.root, user)
	f.git(f.root, "clone", "-q", f.remote, dir)
	f.identity(dir, user)
	s := NewStore(dir, user)
	s.Now = f.now
	return s
}

// now advances the shared clock a minute per call, so every write has its
// own time and last-writer-wins is decidable.
func (f *fixture) now() time.Time {
	f.clock = f.clock.Add(time.Minute)
	return f.clock
}

// remoteIssue reads one issue as the tracker repository holds it, without
// any clone's cache.
func (f *fixture) remoteIssue(id string) Issue {
	f.t.Helper()
	issues := f.remoteIssues()
	for _, i := range issues {
		if i.ID == id {
			return i
		}
	}
	f.t.Fatalf("tracker repository has no issue %s", id)
	return Issue{}
}

// remoteIssues reads every issue the tracker repository's main holds, in
// either layout.
func (f *fixture) remoteIssues() []Issue {
	f.t.Helper()
	if fixtureFiles() {
		out, err := exec.Command("git", "--git-dir", f.remote, "ls-tree", "--name-only", "main", IssuesDir+"/").Output()
		if err != nil {
			f.t.Fatalf("list the tracker repository: %v", err)
		}
		var issues []Issue
		for _, p := range strings.Fields(string(out)) {
			b, err := exec.Command("git", "--git-dir", f.remote, "show", "main:"+p).Output()
			if err != nil {
				f.t.Fatal(err)
			}
			i, err := decodeIssue(p, b)
			if err != nil {
				f.t.Fatal(err)
			}
			issues = append(issues, i)
		}
		return issues
	}
	out, err := exec.Command("git", "--git-dir", f.remote, "show", "main:"+IssuesFile).Output()
	if err != nil {
		f.t.Fatalf("read tracker repository: %v", err)
	}
	issues, err := parseIssues(out)
	if err != nil {
		f.t.Fatal(err)
	}
	return issues
}

// fixtureFiles reports whether the fixtures keep one file per issue, the
// layout scrum-master init now seeds. BEADS_FIXTURE_LAYOUT=jsonl runs the package on
// bd's single JSONL file instead, which TestJSONLLayoutSuite does.
func fixtureFiles() bool { return os.Getenv("BEADS_FIXTURE_LAYOUT") != "jsonl" }

// TestJSONLLayoutSuite runs the package's tests again on a tracker that
// keeps bd's single JSONL file, the layout every tracker made before the
// files layout keeps until it migrates.
func TestJSONLLayoutSuite(t *testing.T) {
	if !fixtureFiles() || testing.Short() {
		t.Skip("runs in the files-layout pass only, and not in -short")
	}
	cmd := exec.Command(os.Args[0], "-test.run", "^Test", "-test.count=1")
	cmd.Env = append(os.Environ(), "BEADS_FIXTURE_LAYOUT=jsonl")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("the beads tests on the JSONL layout failed: %v\n%s", err, out)
	}
}

// offline points a clone's origin at a path that does not exist; online
// restores it.
func (f *fixture) offline(s *Store) {
	f.git(s.Dir, "remote", "set-url", "origin", filepath.Join(f.root, "unreachable.git"))
}

func (f *fixture) online(s *Store) {
	f.git(s.Dir, "remote", "set-url", "origin", f.remote)
}

func (f *fixture) identity(dir, user string) {
	f.git(dir, "config", "user.name", user)
	f.git(dir, "config", "user.email", user+"@example.com")
}

func (f *fixture) git(dir string, args ...string) {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

// ready is an admitted, unassigned data issue in the backlog.
func ready(id string) Issue {
	return Issue{ID: id, Title: "work " + id, Status: StatusOpen, State: Ready,
		Labels: []string{"plane::data"}, CreatedAt: "2026-11-01T09:00:00Z", UpdatedAt: "2026-11-01T09:00:00Z"}
}
