// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

func listIn(t *testing.T, dir string, open func() (tracker.Tracker, error), flags ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := cli.Default.Run(append([]string{"backlog", "list", "--dir", dir}, flags...), &out, open); err != nil {
		t.Fatalf("backlog list: %v\n%s", err, out.String())
	}
	return out.String()
}

func gitAt(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// oneProjectDown is a tracker that can't read one project.
type oneProjectDown struct {
	*fake.Fake
	down string
}

func (o oneProjectDown) ListIssues(project string, q tracker.IssueQuery) ([]tracker.Issue, error) {
	if project == o.down {
		return nil, errors.New("503")
	}
	return o.Fake.ListIssues(project, q)
}

// One project it can't read keeps what the document records for that project,
// beside what the others say, and the listing names it (srd045 R8.1).
func TestListKeepsTheRecordOfAProjectItCannotRead(t *testing.T) {
	_, scrum, _ := recordProgram(t)
	p := filepath.Join(scrum, "docs", "specs", "stories", "st001-score-trajectories.md")
	doc, _ := os.ReadFile(p)
	os.WriteFile(p, WriteLedger(doc, []LedgerEntry{{Ref: "g/code-1#3", Title: "Wire the scorer", State: "open"}}), 0o644)
	f := fake.New()
	f.Issues["g/po-1"] = []*tracker.Issue{{Project: "g/po-1", IID: 7, Title: "Define scoring", State: "opened", Description: "Implements: st001-score-trajectories"}}
	out := listIn(t, scrum, func() (tracker.Tracker, error) { return oneProjectDown{f, "g/code-1"}, nil }, "--dry-run")
	for _, want := range []string{"note: g/code-1 is not read", "503", "code-1#3 Wire the scorer", "po-1#7 Define scoring"} {
		if !strings.Contains(out, want) {
			t.Errorf("the listing lacks %q:\n%s", want, out)
		}
	}
}

// The listing shows a story's issues as the tracker holds them, open first, not
// as the document last recorded them, and writes no document (srd045 R8.1).
func TestListShowsTheTrackersIssues(t *testing.T) {
	_, scrum, _ := recordProgram(t)
	p := filepath.Join(scrum, "docs", "specs", "stories", "st001-score-trajectories.md")
	doc, _ := os.ReadFile(p)
	stale := WriteLedger(doc, []LedgerEntry{{Ref: "g/po-1#7", Title: "Define scoring", State: "open"}})
	if err := os.WriteFile(p, stale, 0o644); err != nil {
		t.Fatal(err)
	}
	f := fake.New()
	f.Issues["g/po-1"] = []*tracker.Issue{{Project: "g/po-1", IID: 7, Title: "Define scoring", State: "closed", ClosedAt: "2026-11-01T08:00:00Z",
		Description: "Implements: st001-score-trajectories", Assignees: []string{"ana"}}}
	f.Issues["g/code-1"] = []*tracker.Issue{{Project: "g/code-1", IID: 3, Title: "Wire the scorer", State: "opened",
		Description: "Implements: https://gitlab.example.com/g/po-1/-/blob/main/docs/specs/stories/st001-score-trajectories.md"}}
	open := func() (tracker.Tracker, error) { return f, nil }

	out := listIn(t, scrum, open, "--dry-run")
	opened, closed := strings.Index(out, "open   code-1#3 Wire the scorer"), strings.Index(out, "closed po-1#7 Define scoring (ana)")
	if opened < 0 || closed < opened {
		t.Fatalf("the listing should show the open issue, then the closed one:\n%s", out)
	}
	if got, _ := os.ReadFile(p); !bytes.Equal(got, stale) {
		t.Fatal("the listing wrote the document")
	}

	var entries []Entry
	if err := json.Unmarshal([]byte(listIn(t, scrum, open, "--dry-run", "--json")), &entries); err != nil {
		t.Fatalf("--json stays one object per document: %v", err)
	}
	if len(entries) != 1 || len(entries[0].Issues) != 2 {
		t.Fatalf("entries = %+v", entries)
	}
}

// A tracker it can't reach still lists the documents, shows what they record,
// and says so (srd045 R8.1).
func TestListSaysWhenTheTrackerCannotBeReached(t *testing.T) {
	_, scrum, _ := recordProgram(t)
	p := filepath.Join(scrum, "docs", "specs", "stories", "st001-score-trajectories.md")
	doc, _ := os.ReadFile(p)
	if err := os.WriteFile(p, WriteLedger(doc, []LedgerEntry{{Ref: "g/po-1#7", Title: "Define scoring", State: "open"}}), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAt(t, scrum, "add", "-A")
	gitAt(t, scrum, "-c", "user.email=a@example.com", "-c", "user.name=a", "commit", "-q", "-m", "the record")
	out := listIn(t, scrum, func() (tracker.Tracker, error) { return nil, errors.New("no route to host") }, "--dry-run")
	for _, want := range []string{"note: the tracker can't be reached", "no route to host", "st001-score-trajectories", "open   po-1#7 Define scoring"} {
		if !strings.Contains(out, want) {
			t.Errorf("the listing lacks %q:\n%s", want, out)
		}
	}

	var entries []Entry
	json.Unmarshal([]byte(listIn(t, scrum, func() (tracker.Tracker, error) { return nil, errors.New("no route to host") }, "--dry-run", "--json")), &entries)
	if len(entries) != 1 || len(entries[0].IssueNotes) != 1 {
		t.Errorf("--json should carry the note on the story: %+v", entries)
	}
}

// The listing pulls a clean checkout on its default branch before it reads, and
// a checkout it can't pull is listed as it stands, with a note saying so (srd045
// R8.1).
func TestListPullsFirst(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "po-1.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", "-b", "main", bare).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	theirs, mine := filepath.Join(root, "theirs"), filepath.Join(root, "mine")
	gitAt(t, root, "clone", "-q", bare, theirs)
	story := filepath.Join("docs", "specs", "stories", "st001-score-trajectories.md")
	commit := func(dir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(story)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, story), teamStory(t), 0o644); err != nil {
			t.Fatal(err)
		}
		gitAt(t, dir, "add", "-A")
		gitAt(t, dir, "-c", "user.email=a@example.com", "-c", "user.name=a", "commit", "-q", "-m", "a story")
		gitAt(t, dir, "push", "-q", "origin", "HEAD:main")
	}
	if err := os.WriteFile(filepath.Join(theirs, "README.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitAt(t, theirs, "checkout", "-q", "-b", "main")
	gitAt(t, theirs, "add", "-A")
	gitAt(t, theirs, "-c", "user.email=a@example.com", "-c", "user.name=a", "commit", "-q", "-m", "start")
	gitAt(t, theirs, "push", "-q", "origin", "HEAD:main")
	gitAt(t, root, "clone", "-q", "-b", "main", bare, mine)
	commit(theirs)

	noTracker := func() (tracker.Tracker, error) { return nil, errors.New("offline") }
	dirty := filepath.Join(mine, "scratch")
	if err := os.WriteFile(dirty, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := listIn(t, mine, noTracker); !strings.Contains(out, "note: not pulled") || !strings.Contains(out, "holds changes") || strings.Contains(out, "st001-score-trajectories") {
		t.Fatalf("a dirty checkout should be listed as it stands, with a note:\n%s", out)
	}
	if err := os.Remove(dirty); err != nil {
		t.Fatal(err)
	}
	if out := listIn(t, mine, noTracker); strings.Contains(out, "not pulled") || !strings.Contains(out, "st001-score-trajectories") {
		t.Fatalf("a clean checkout should be pulled first:\n%s", out)
	}
}

// A directory inside a checkout only has its documents read: a fixture under
// this repository must not pull it or sweep its tracker.
func TestListOfADirectoryInsideACheckoutTouchesNothing(t *testing.T) {
	_, scrum, _ := recordProgram(t)
	asked := false
	open := func() (tracker.Tracker, error) { asked = true; return fake.New(), nil }
	out := listIn(t, filepath.Join(scrum, "docs"), open)
	if asked || strings.Contains(out, "note:") {
		t.Errorf("asked the tracker = %v, output:\n%s", asked, out)
	}
}
