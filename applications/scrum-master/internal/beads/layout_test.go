// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package beads

import (
	"bytes"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestFilesLayoutMergesThroughGit creates an issue on two clones that
// cannot see each other, and changes one issue on both: after the later
// clone reconciles, the two new issues have distinct IIDs, the later one
// renumbered, and the shared issue carries both clones' changes (eng04).
func TestFilesLayoutMergesThroughGit(t *testing.T) {
	if !fixtureFiles() {
		t.Skip("the files layout's merge")
	}
	f := newFixture(t, ready("sm-1"))
	alice, bob := f.clone("alice"), f.clone("bob")
	bob.sleep = func(time.Duration) {}
	if _, _, err := bob.Issues(); err != nil {
		t.Fatal(err)
	}
	f.offline(bob)
	theirs, err := bob.Create(Issue{Repository: "g/po-1", Title: "bob's"})
	if err != nil {
		t.Fatal(err)
	}
	if err := bob.Comment("sm-1", "bob was here"); err != nil {
		t.Fatal(err)
	}
	ours, err := alice.Create(Issue{Repository: "g/po-1", Title: "alice's"})
	if err != nil {
		t.Fatal(err)
	}
	if err := alice.Comment("sm-1", "alice was here"); err != nil {
		t.Fatal(err)
	}
	if ours.IID != 1 || theirs.IID != 1 {
		t.Fatalf("before the merge both clones picked IID 1: alice %d, bob %d", ours.IID, theirs.IID)
	}
	f.online(bob)
	if _, err := bob.Reconcile(); err != nil {
		t.Fatal(err)
	}
	got := map[string]Issue{}
	for _, i := range f.remoteIssues() {
		got[i.Title] = i
	}
	if got["alice's"].IID != 1 || got["bob's"].IID != 2 {
		t.Fatalf("IIDs after the merge: alice's %d, bob's %d; want 1 and 2", got["alice's"].IID, got["bob's"].IID)
	}
	var texts []string
	for _, c := range got["work sm-1"].Comments {
		texts = append(texts, c.Text)
	}
	if !slices.Contains(texts, "alice was here") || !slices.Contains(texts, "bob was here") {
		t.Fatalf("sm-1's comments after the merge: %q", texts)
	}
	// Every issue is one file with one line.
	out, _ := exec.Command("git", "--git-dir", f.remote, "ls-tree", "--name-only", "main", IssuesDir+"/").Output()
	if n := len(strings.Fields(string(out))); n != 3 {
		t.Fatalf("%d issue files, want 3", n)
	}
}

// TestMigrateAndExport moves a JSONL tracker to one file per issue in one
// commit, keeps every issue, refuses nothing on a second run, and exports
// bd's JSONL back.
func TestMigrateAndExport(t *testing.T) {
	t.Setenv("BEADS_FIXTURE_LAYOUT", "jsonl")
	f := newFixture(t, ready("sm-1"), ready("sm-2"))
	alice := f.clone("alice")
	if err := alice.Comment("sm-2", "before the move"); err != nil {
		t.Fatal(err)
	}
	before := f.remoteIssues()
	r, err := alice.Migrate()
	if err != nil || r.Issues != 2 || r.Already {
		t.Fatalf("migrate = %+v, %v", r, err)
	}
	if !alice.files() {
		t.Fatal("the config does not say layout: files")
	}
	t.Setenv("BEADS_FIXTURE_LAYOUT", "files")
	after := f.remoteIssues()
	if len(after) != len(before) {
		t.Fatalf("%d issues after the move, %d before", len(after), len(before))
	}
	f.checkIndex()
	if out, _ := exec.Command("git", "--git-dir", f.remote, "show", "main:"+IssuesFile).CombinedOutput(); !strings.Contains(string(out), "does not exist") && !strings.Contains(string(out), "exists on disk, but not in") {
		t.Fatalf("the JSONL file is still in the tracker: %s", out)
	}
	again, err := alice.Migrate()
	if err != nil || !again.Already {
		t.Fatalf("second migrate = %+v, %v", again, err)
	}
	b, err := alice.Export()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := encodeJSONL(before)
	if !bytes.Equal(b, want) {
		t.Fatalf("export:\n%s\nwant:\n%s", b, want)
	}
	bob := f.clone("bob")
	if err := bob.Comment("sm-1", "after the move"); err != nil {
		t.Fatal(err)
	}
	if i := f.remoteIssue("sm-1"); len(i.Comments) != 1 {
		t.Fatalf("a write after the move: %+v", i)
	}
}

// remoteIndex is the tracker repository's index on main, by entry path.
func (f *fixture) remoteIndex() map[string]string {
	f.t.Helper()
	out, err := exec.Command("git", "--git-dir", f.remote, "ls-tree", "-r", "--name-only", "main", IndexDir+"/").Output()
	if err != nil {
		f.t.Fatal(err)
	}
	index := map[string]string{}
	for _, p := range strings.Fields(string(out)) {
		b, err := exec.Command("git", "--git-dir", f.remote, "show", "main:"+p).Output()
		if err != nil {
			f.t.Fatal(err)
		}
		index[p] = strings.TrimSpace(string(b))
	}
	return index
}

// checkIndex fails unless the tracker repository's index has exactly one
// entry per numbered issue, naming it.
func (f *fixture) checkIndex() {
	f.t.Helper()
	want := map[string]string{}
	for _, i := range f.remoteIssues() {
		if i.IID != 0 {
			want[IndexDir+"/"+url.PathEscape(i.Repository)+"/"+strconv.Itoa(i.IID)] = i.ID
		}
	}
	if got := f.remoteIndex(); !maps.Equal(got, want) {
		f.t.Fatalf("index:\n%v\nwant:\n%v", got, want)
	}
}

func numbered(id, repository string, iid int) Issue {
	i := ready(id)
	i.Repository, i.IID = repository, iid
	return i
}

// TestIndexSurvivesConcurrentCreates has two clones create the same IID:
// after the merge the IIDs differ and the index names each issue at its own
// (#143).
func TestIndexSurvivesConcurrentCreates(t *testing.T) {
	if !fixtureFiles() {
		t.Skip("the files layout's index")
	}
	f := newFixture(t, numbered("sm-1", "g/po-1", 1), numbered("sm-2", "g/po-2", 1))
	alice, bob := f.clone("alice"), f.clone("bob")
	bob.sleep = func(time.Duration) {}
	if _, _, err := bob.Issues(); err != nil {
		t.Fatal(err)
	}
	f.offline(bob)
	theirs, err := bob.Create(Issue{Repository: "g/po-1", Title: "bob's"})
	if err != nil {
		t.Fatal(err)
	}
	ours, err := alice.Create(Issue{Repository: "g/po-1", Title: "alice's"})
	if err != nil {
		t.Fatal(err)
	}
	if ours.IID != 2 || theirs.IID != 2 {
		t.Fatalf("before the merge both clones picked IID 2: alice %d, bob %d", ours.IID, theirs.IID)
	}
	f.online(bob)
	if _, err := bob.Reconcile(); err != nil {
		t.Fatal(err)
	}
	f.checkIndex()
	got, ok, err := bob.Find("g/po-1", 3)
	if err != nil || !ok || got.Title != "bob's" {
		t.Fatalf("Find(g/po-1, 3) = %q, %v, %v; want bob's", got.Title, ok, err)
	}
	next, err := alice.Create(Issue{Repository: "g/po-1", Title: "alice's second"})
	if err != nil || next.IID != 4 {
		t.Fatalf("the next create got IID %d, %v; want 4", next.IID, err)
	}
	f.checkIndex()
}

// TestFindReadsTheIndex finds an issue by its entry without reading the
// others, and falls back to reading them all when the entry is missing or
// names another issue.
func TestFindReadsTheIndex(t *testing.T) {
	if !fixtureFiles() {
		t.Skip("the files layout's index")
	}
	f := newFixture(t, numbered("sm-1", "g/po-1", 1), numbered("sm-2", "g/po-1", 2), numbered("sm-3", "g/po-1", 3))
	alice := f.clone("alice")
	if _, _, err := alice.Issues(); err != nil {
		t.Fatal(err)
	}
	// An unreadable sm-3 fails any read of every issue, so a Find that
	// succeeds read only the one it names.
	if err := os.WriteFile(issuePath(alice.Dir, "sm-3"), []byte("{not json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if i, ok, err := alice.Find("g/po-1", 2); err != nil || !ok || i.ID != "sm-2" {
		t.Fatalf("Find(g/po-1, 2) = %s, %v, %v", i.ID, ok, err)
	}
	f.git(alice.Dir, "checkout", "--", IssuesDir)
	// An entry naming the wrong issue, and a missing one, fall back.
	if err := os.WriteFile(indexPath(alice.Dir, "g/po-1", 1), []byte("sm-2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(indexPath(alice.Dir, "g/po-1", 3)); err != nil {
		t.Fatal(err)
	}
	for iid, id := range map[int]string{1: "sm-1", 3: "sm-3"} {
		if i, ok, err := alice.Find("g/po-1", iid); err != nil || !ok || i.ID != id {
			t.Fatalf("Find(g/po-1, %d) = %s, %v, %v; want %s", iid, i.ID, ok, err, id)
		}
	}
	if _, ok, err := alice.Find("g/po-1", 9); err != nil || ok {
		t.Fatalf("Find(g/po-1, 9) found %v, %v", ok, err)
	}
}

// TestMigrateIndexesAFilesTracker builds the index of a tracker that kept
// one file per issue before the index existed, and a create on such a
// tracker builds it before numbering, so it never reuses an IID.
func TestMigrateIndexesAFilesTracker(t *testing.T) {
	if !fixtureFiles() {
		t.Skip("the files layout's index")
	}
	unindex := func(f *fixture) {
		dir := filepath.Join(f.root, "unindex")
		f.git(f.root, "clone", "-q", f.remote, dir)
		f.identity(dir, "seed")
		f.git(dir, "rm", "-q", "-r", IndexDir)
		f.git(dir, "commit", "-q", "-m", "an index-less tracker")
		f.git(dir, "push", "-q", "origin", "HEAD:main")
		if len(f.remoteIndex()) != 0 {
			f.t.Fatal("the index is still there")
		}
	}
	f := newFixture(t, numbered("sm-1", "g/po-1", 1), numbered("sm-2", "g/po-1", 2))
	unindex(f)
	alice := f.clone("alice")
	r, err := alice.Migrate()
	if err != nil || !r.Already || r.Indexed != 2 || r.Commit == "" {
		t.Fatalf("migrate = %+v, %v", r, err)
	}
	f.checkIndex()
	if again, err := alice.Migrate(); err != nil || again.Indexed != 0 {
		t.Fatalf("second migrate = %+v, %v", again, err)
	}

	g := newFixture(t, numbered("sm-1", "g/po-1", 1), numbered("sm-2", "g/po-1", 2))
	unindex(g)
	bob := g.clone("bob")
	i, err := bob.Create(Issue{Repository: "g/po-1", Title: "bob's"})
	if err != nil || i.IID != 3 {
		t.Fatalf("create on an index-less tracker got IID %d, %v; want 3", i.IID, err)
	}
	g.checkIndex()
}
