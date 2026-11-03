// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package people

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
)

// TestPeopleFormatExampleDecodes holds the people format to its example, and
// the written document to the same reader.
func TestPeopleFormatExampleDecodes(t *testing.T) {
	b, err := os.ReadFile("../../docs/specs/config-formats/people.md")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(string(b), "<!-- field: config_file_layout -->")
	_, rest, _ = strings.Cut(rest, "```markdown\n")
	doc, _, _ := strings.Cut(rest, "\n```")
	c, err := Parse([]byte(doc + "\n"))
	if err != nil || len(c.People) == 0 || len(c.People[0].Epics) == 0 || len(c.People[0].Scrums) == 0 {
		t.Fatalf("example = %+v, %v", c, err)
	}
	written, err := Document(c)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Parse(written)
	if err != nil || !reflect.DeepEqual(again, c) {
		t.Fatalf("round trip = %+v, %v; want %+v", again, err, c)
	}
}

func TestParseRefuses(t *testing.T) {
	for name, table := range map[string]string{
		"twice":   "| username | epics |\n| --- | --- |\n| a | e1 |\n| a | e2 |\n",
		"no name": "| username | epics |\n| --- | --- |\n|  | e1 |\n",
		"a path":  "| username | epics |\n| --- | --- |\n| ../a | e1 |\n",
	} {
		if _, err := Parse([]byte("---\nid: people\ntitle: People\n---\n\n<!-- field: people -->\n## People\n\n" + table)); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}

func TestLoadWithNoDocumentIsNoPeople(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil || len(c.People) != 0 {
		t.Fatalf("Load = %+v, %v", c, err)
	}
}

func TestApplyKeepsPeopleAndNamesSorted(t *testing.T) {
	var c Config
	c, _ = Apply(c, Change{Username: "bob", Epics: []string{"ep2", "ep1"}})
	c, _ = Apply(c, Change{Username: "alice", Scrums: []string{"s1"}})
	c, _ = Apply(c, Change{Username: "bob", Epics: []string{"ep1"}, Scrums: []string{"s1"}})
	want := []Person{{Username: "alice", Scrums: Names{"s1"}}, {Username: "bob", Scrums: Names{"s1"}, Epics: Names{"ep1", "ep2"}}}
	if !reflect.DeepEqual(c.People, want) {
		t.Fatalf("people = %+v, want %+v", c.People, want)
	}
	c, _ = Apply(c, Change{Username: "alice", Scrums: []string{"s1"}, Release: true})
	c, p := Apply(c, Change{Username: "bob", Epics: []string{"ep1"}, Release: true})
	if len(c.People) != 1 || !reflect.DeepEqual(p.Epics, Names{"ep2"}) {
		t.Fatalf("after release: %+v, %+v; alice on nothing leaves the document", c.People, p)
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "control", "people.md")
	ch := Change{Username: "alice", Epics: []string{"ep1"}}

	r, err := Run(dir, ch, false, true)
	if err != nil || r.Action != "allocated" || !r.DryRun {
		t.Fatalf("dry run = %+v, %v", r, err)
	}
	if _, err := os.Stat(file); err == nil {
		t.Fatal("a dry run wrote the document")
	}
	if r, err = Run(dir, ch, false, false); err != nil || r.Action != "allocated" {
		t.Fatalf("run = %+v, %v", r, err)
	}
	first, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if r, err = Run(dir, ch, false, false); err != nil || r.Action != "unchanged" {
		t.Fatalf("second run = %+v, %v", r, err)
	}
	second, _ := os.ReadFile(file)
	if string(first) != string(second) {
		t.Fatalf("a second run changed the document:\n%s", second)
	}
	if r, err = Run(dir, Change{Username: "alice", Epics: []string{"ep9"}, Release: true}, false, false); err != nil || r.Action != "unchanged" {
		t.Fatalf("releasing what they are not on = %+v, %v", r, err)
	}
	if r, err = Run(dir, Change{Username: "alice", Epics: []string{"ep1"}, Release: true}, false, false); err != nil || r.Action != "released" {
		t.Fatalf("release = %+v, %v", r, err)
	}
	c, err := Load(dir)
	if err != nil || len(c.People) != 0 {
		t.Fatalf("after release = %+v, %v", c, err)
	}
}

func TestRunCommits(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"}, {"commit", "-q", "--allow-empty", "-m", "start"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	r, err := Run(dir, Change{Username: "alice", Scrums: []string{"s1"}}, true, false)
	if err != nil || r.Commit == "" {
		t.Fatalf("run = %+v, %v", r, err)
	}
	out, err := exec.Command("git", "-C", dir, "log", "-1", "--name-only", "--format=%s").Output()
	if err != nil || !strings.Contains(string(out), "control/people.md") || !strings.Contains(string(out), "alice") {
		t.Fatalf("log = %s, %v", out, err)
	}
	r, err = Run(dir, Change{Username: "alice", Scrums: []string{"s1"}}, true, false)
	if err != nil || r.Commit != "" {
		t.Fatalf("an unchanged run committed: %+v, %v", r, err)
	}
}

// TestCommands runs the words as a caller does, finding the system checkout
// from one beside it.
func TestCommands(t *testing.T) {
	root := t.TempDir()
	system, beside := filepath.Join(root, "system"), filepath.Join(root, "team")
	for _, d := range []string{filepath.Join(system, "control"), beside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(system, "control", "trackers.md"), []byte("---\nid: trackers\ntitle: Tracking systems\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := cli.Default.Run(append([]string{"people"}, args...), &out, nil)
		return out.String(), err
	}
	for _, args := range [][]string{{"allocate", "--dir", system}, {"allocate", "--dir", system, "--person", "alice"}, {"allocate", "--dir", system, "--person", "../a", "--epic", "e"}} {
		if _, err := run(args...); !errors.Is(err, cli.ErrUsage) {
			t.Errorf("%v: err = %v, want usage", args, err)
		}
	}
	if out, err := run("allocate", "--dir", system, "--person", "alice", "--epic", "ep1", "--scrum", "s1", "--dry-run"); err != nil || !strings.Contains(out, "would be allocated") {
		t.Fatalf("dry run = %q, %v", out, err)
	}
	if out, err := run("allocate", "--dir", beside, "--person", "alice", "--epic", "ep1", "--epic", "ep2"); err != nil || !strings.Contains(out, "allocated") {
		t.Fatalf("allocate = %q, %v", out, err)
	}
	// Names from the run before are not carried into this one.
	if out, err := run("release", "--dir", system, "--person", "alice", "--scrum", "s1"); err != nil || !strings.Contains(out, "unchanged") {
		t.Fatalf("release = %q, %v", out, err)
	}
	out, err := run("list", "--dir", system)
	if err != nil || !strings.Contains(out, "alice: scrums none; epics ep1, ep2") {
		t.Fatalf("list = %q, %v", out, err)
	}
}

func TestReviews(t *testing.T) {
	dir := t.TempDir()
	if _, err := Run(dir, Change{Username: "alice", Reviews: []string{"platform", "ep1"}}, false, false); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil || c.ReviewerOf("ep1") != "alice" || c.ReviewerOf("ep2") != "" || !c.HasReviewers() {
		t.Fatalf("after review = %+v, %v", c, err)
	}
	if _, err := Run(dir, Change{Username: "bob", Reviews: []string{"ep1"}}, false, false); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("taking a name without force = %v", err)
	}
	if r, err := Run(dir, Change{Username: "alice", Reviews: []string{"ep1"}}, false, false); err != nil || r.Action != "unchanged" {
		t.Fatalf("second run = %+v, %v", r, err)
	}
	if _, err := Run(dir, Change{Username: "bob", Reviews: []string{"ep1"}, Force: true}, false, false); err != nil {
		t.Fatal(err)
	}
	if c, _ = Load(dir); c.ReviewerOf("ep1") != "bob" || c.ReviewerOf("platform") != "alice" {
		t.Fatalf("after moving = %+v", c)
	}
	// Reviewing keeps a person in the document, and a move drops one left on nothing.
	if _, err := Run(dir, Change{Username: "bob", Reviews: []string{"platform"}, Force: true}, false, false); err != nil {
		t.Fatal(err)
	}
	if c, _ = Load(dir); len(c.People) != 1 || c.People[0].Username != "bob" {
		t.Fatalf("alice on nothing should have left: %+v", c)
	}
	if r, err := Run(dir, Change{Username: "bob", Reviews: []string{"ep1", "platform"}, Release: true}, false, false); err != nil || r.Action != "removed as reviewer" {
		t.Fatalf("unreview = %+v, %v", r, err)
	}
	if c, _ = Load(dir); len(c.People) != 0 {
		t.Fatalf("after unreview = %+v", c)
	}
}

func TestParseRefusesTwoReviewers(t *testing.T) {
	doc := "---\nid: people\ntitle: People\n---\n\n<!-- field: people -->\n## People\n\n| username | reviews |\n| --- | --- |\n| a | ep1 |\n| b | ep1, ep2 |\n"
	if _, err := Parse([]byte(doc)); err == nil || !strings.Contains(err.Error(), "two reviewers") {
		t.Fatalf("Parse = %v", err)
	}
}

func TestReviewCommands(t *testing.T) {
	root := t.TempDir()
	system := filepath.Join(root, "system")
	if err := os.MkdirAll(filepath.Join(system, "control"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(system, "control", "trackers.md"), []byte("---\nid: trackers\ntitle: Tracking systems\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) (string, error) {
		var out bytes.Buffer
		err := cli.Default.Run(append([]string{"people"}, args...), &out, nil)
		return out.String(), err
	}
	for _, args := range [][]string{{"review", "--dir", system}, {"review", "--dir", system, "--person", "alice"}, {"unreview", "--dir", system, "--person", "../a", "--scrum", "e"}} {
		if _, err := run(args...); !errors.Is(err, cli.ErrUsage) {
			t.Errorf("%v: err = %v, want usage", args, err)
		}
	}
	if out, err := run("review", "--dir", system, "--person", "alice", "--scrum", "serving", "--scrum", "platform"); err != nil || !strings.Contains(out, "named reviewer") {
		t.Fatalf("review = %q, %v", out, err)
	}
	out, err := run("list", "--dir", system)
	if err != nil || !strings.Contains(out, "alice: scrums none; epics none; reviews platform, serving") {
		t.Fatalf("list = %q, %v", out, err)
	}
	if out, err := run("unreview", "--dir", system, "--person", "alice", "--scrum", "serving"); err != nil || !strings.Contains(out, "removed as reviewer") {
		t.Fatalf("unreview = %q, %v", out, err)
	}
}
