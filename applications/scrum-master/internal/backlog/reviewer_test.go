// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package backlog

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
)

func reviewerCheckout(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	for p, id := range map[string]string{
		"docs/VISION.md": "program-vision", "docs/ARCHITECTURE.md": "architecture-x", "docs/specs/stories/st001-a.md": "st001-a",
		"docs/specs/software-requirements/srd045-b.md": "srd045-b", "docs/specs/use-cases/rel01.0-uc001-c.md": "rel01.0-uc001-c", "docs/guides/process.md": "process",
	} {
		file := filepath.Join(dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(file), 0o755)
		if err := os.WriteFile(file, []byte("---\nid: "+id+"\ntitle: T\n---\n\nBody\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFindDocument(t *testing.T) {
	dir := reviewerCheckout(t)
	for name, want := range map[string]string{
		"st001-a": "docs/specs/stories/st001-a.md", "srd045-b": "docs/specs/software-requirements/srd045-b.md",
		"rel01.0-uc001-c": "docs/specs/use-cases/rel01.0-uc001-c.md", "program-vision": "docs/VISION.md",
		"architecture-x": "docs/ARCHITECTURE.md", "docs/VISION.md": "docs/VISION.md",
	} {
		if got, err := FindDocument(dir, name); err != nil || got != want {
			t.Errorf("FindDocument(%s) = %q, %v; want %q", name, got, err, want)
		}
	}
	for _, name := range []string{"st999-gone", "docs/guides/process.md", "docs/specs/stories/st999-gone.md"} {
		if got, err := FindDocument(dir, name); err == nil {
			t.Errorf("FindDocument(%s) = %q, want a refusal", name, got)
		}
	}
}

func TestSetReviewer(t *testing.T) {
	dir := reviewerCheckout(t)
	read := func() string {
		b, err := os.ReadFile(filepath.Join(dir, "docs", "VISION.md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	before := read()
	if r, err := SetReviewer(dir, "program-vision", "carol", false, false, true); err != nil || r.Action != "named" || read() != before {
		t.Fatalf("dry run = %+v, %v", r, err)
	}
	if r, err := SetReviewer(dir, "program-vision", "carol", false, false, false); err != nil || r.Action != "named" || !strings.Contains(read(), "reviewer: carol") {
		t.Fatalf("name = %+v, %v:\n%s", r, err, read())
	}
	if r, err := SetReviewer(dir, "program-vision", "carol", false, false, false); err != nil || r.Action != "unchanged" {
		t.Fatalf("second run = %+v, %v", r, err)
	}
	if _, err := SetReviewer(dir, "program-vision", "dan", false, false, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("replacing without force = %v", err)
	}
	if r, err := SetReviewer(dir, "program-vision", "dan", true, false, false); err != nil || r.Previous != "carol" || !strings.Contains(read(), "reviewer: dan") {
		t.Fatalf("replacing with force = %+v, %v", r, err)
	}
	if r, err := SetReviewer(dir, "program-vision", "", false, false, false); err != nil || r.Action != "dropped" || read() != before {
		t.Fatalf("clear = %+v, %v:\n%s", r, err, read())
	}
	exec.Command("git", "-C", dir, "add", "-A").Run()
	exec.Command("git", "-C", dir, "commit", "-q", "-m", "start").Run()
	r, err := SetReviewer(dir, "st001-a", "carol", false, true, false)
	if err != nil || r.Commit == "" {
		t.Fatalf("with commit = %+v, %v", r, err)
	}
}

func TestReviewerCommandRefusesBadUsage(t *testing.T) {
	for _, args := range [][]string{
		{"backlog", "reviewer", "--person", "a"},
		{"backlog", "reviewer", "--document", "st001-a"},
		{"backlog", "reviewer", "--document", "st001-a", "--person", "a", "--clear"},
		{"backlog", "reviewer", "--document", "st001-a", "--person", "../a"},
	} {
		var out bytes.Buffer
		if err := cli.Default.Run(args, &out, nil); !errors.Is(err, cli.ErrUsage) {
			t.Errorf("%v: err = %v, want usage", args, err)
		}
	}
}

func TestSetAuthor(t *testing.T) {
	dir := reviewerCheckout(t)
	read := func() string {
		b, err := os.ReadFile(filepath.Join(dir, "docs", "specs", "stories", "st001-a.md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	before := read()
	if r, err := SetAuthor(dir, "st001-a", "alice", false, false, true); err != nil || r.Action != "added" || read() != before {
		t.Fatalf("dry run = %+v, %v", r, err)
	}
	if _, err := SetAuthor(dir, "st001-a", "alice", false, false, false); err != nil || !strings.Contains(read(), "authors: alice\n") {
		t.Fatalf("add = %v:\n%s", err, read())
	}
	if r, err := SetAuthor(dir, "st001-a", "alice", false, false, false); err != nil || r.Action != "unchanged" {
		t.Fatalf("second run = %+v, %v", r, err)
	}
	if _, err := SetAuthor(dir, "st001-a", "bob", false, false, false); err != nil || !strings.Contains(read(), "authors: alice, bob\n") {
		t.Fatalf("add a second = %v:\n%s", err, read())
	}
	if r, err := SetAuthor(dir, "st001-a", "carol", true, false, false); err != nil || r.Action != "unchanged" {
		t.Fatalf("removing who is not named = %+v, %v", r, err)
	}
	if _, err := SetAuthor(dir, "st001-a", "alice", true, false, false); err != nil || !strings.Contains(read(), "authors: bob\n") {
		t.Fatalf("remove = %v:\n%s", err, read())
	}
	if _, err := SetAuthor(dir, "st001-a", "bob", true, false, false); err != nil || read() != before {
		t.Fatalf("removing the last = %v:\n%s", err, read())
	}
	if _, err := SetAuthor(dir, "st999-gone", "bob", false, false, false); err == nil {
		t.Fatal("a missing document took an author")
	}
}

// TestSetAuthorReplacesThePlaceholder: a template's line names no one.
func TestSetAuthorReplacesThePlaceholder(t *testing.T) {
	dir := reviewerCheckout(t)
	file := filepath.Join(dir, "docs", "VISION.md")
	if err := os.WriteFile(file, []byte("---\nid: program-vision\ntitle: T\nauthors: "+AuthorPlaceholder+"\n---\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := SetAuthor(dir, "docs/VISION.md", "alice", false, false, false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(file); !strings.Contains(string(b), "authors: alice\n") {
		t.Fatalf("after = %s", b)
	}
}

// TestAuthorChecks: a document newer than the release must name its authors,
// and older ones draw one warning, as does every document with no release at all.
func TestAuthorChecks(t *testing.T) {
	dir := reviewerCheckout(t)
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	git("add", "-A")
	git("commit", "-q", "-m", "start")
	findings, warning := authorChecks(dir, "")
	if len(findings) != 0 || warning == nil || !strings.Contains(warning.Message, "5 documents") {
		t.Fatalf("no tag = %v, %+v", findings, warning)
	}
	git("tag", "v1")
	added := filepath.Join(dir, "docs", "specs", "stories", "st002-b.md")
	if err := os.WriteFile(added, []byte("---\nid: st002-b\ntitle: T\nauthors: "+AuthorPlaceholder+"\n---\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	findings, warning = authorChecks(dir, "v1")
	if len(findings) != 1 || findings[0].File != "docs/specs/stories/st002-b.md" || warning == nil {
		t.Fatalf("a new document with the placeholder = %v, %+v", findings, warning)
	}
	if _, err := SetAuthor(dir, "st002-b", "alice", false, false, false); err != nil {
		t.Fatal(err)
	}
	if findings, _ = authorChecks(dir, "v1"); len(findings) != 0 {
		t.Fatalf("a new document naming its author = %v", findings)
	}
}

func TestAuthorCommandRefusesBadUsage(t *testing.T) {
	for _, args := range [][]string{
		{"backlog", "author", "--person", "a"},
		{"backlog", "author", "--document", "st001-a"},
		{"backlog", "author", "--document", "st001-a", "--person", "a,b"},
	} {
		var out bytes.Buffer
		if err := cli.Default.Run(args, &out, nil); !errors.Is(err, cli.ErrUsage) {
			t.Errorf("%v: err = %v, want usage", args, err)
		}
	}
}
