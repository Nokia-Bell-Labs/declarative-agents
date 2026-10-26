// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package release

import (
	"bytes"
	"errors"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// gatesFile is the test repository's gates file, a config document, with
// the given push setting.
func gatesFile(push string) string {
	b, err := corpus.FormatConfig("release-gates", "Release gates", Gates{Push: push, Stages: []Stage{
		{Name: "audit", Gates: []Gate{{Name: "audit", Command: "true"}}},
		{Name: "test", Gates: []Gate{{Name: "unit", Command: "true"}, {Name: "install", Command: "true", Directory: "sub"}}},
	}})
	if err != nil {
		panic(err)
	}
	return string(b)
}

var gatesMarkdown = gatesFile("auto")

// gatesDoc writes g as a gates file.
func gatesDoc(t *testing.T, g Gates) string {
	t.Helper()
	b, err := corpus.FormatConfig("release-gates", "Release gates", g)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeGates(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), DefaultGatesFile)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadGates(t *testing.T) {
	g, err := LoadGates(writeGates(t, gatesDoc(t, Gates{Stages: []Stage{{Name: "a", Gates: []Gate{{Name: "x", Command: "go test ./..."}}}}})))
	if err != nil || g.Push != PushAsk || len(g.Stages) != 1 {
		t.Fatalf("g=%+v err=%v", g, err)
	}
	for name, bad := range map[string]Gates{
		"unknown push":    {Push: "always", Stages: []Stage{{Name: "a", Gates: []Gate{{Name: "x", Command: "c"}}}}},
		"no stages":       {Push: "ask"},
		"empty stage":     {Stages: []Stage{{Name: "a"}}},
		"no command":      {Stages: []Stage{{Name: "a", Gates: []Gate{{Name: "x"}}}}},
		"outside the dir": {Stages: []Stage{{Name: "a", Gates: []Gate{{Name: "x", Command: "c", Directory: "../other"}}}}},
	} {
		if _, err := LoadGates(writeGates(t, gatesDoc(t, bad))); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestStageFailureStopsLaterStages(t *testing.T) {
	g := Gates{Stages: []Stage{
		{Name: "one", Gates: []Gate{{Name: "ok", Command: "ok"}, {Name: "bad", Command: "bad"}}},
		{Name: "two", Gates: []Gate{{Name: "later", Command: "later"}}},
	}}
	var mu sync.Mutex
	var ran []string
	run := func(_, command string, out io.Writer) error {
		mu.Lock()
		ran = append(ran, command)
		mu.Unlock()
		io.WriteString(out, "line one\nline two")
		if command == "bad" {
			return errors.New("exit 1")
		}
		return nil
	}
	var out bytes.Buffer
	_, err := RunStages(t.TempDir(), g, run, &out)
	if err == nil || !strings.Contains(err.Error(), "stage one failed: bad") {
		t.Fatalf("err=%v", err)
	}
	for _, c := range ran {
		if c == "later" {
			t.Fatal("a later stage ran after a failure")
		}
	}
	// A stage's gates run at once, so their lines interleave; each line
	// carries its gate's name.
	for _, line := range []string{"[bad] line one\n", "[bad] line two\n", "[ok] line one\n", "[ok] line two\n"} {
		if !strings.Contains(out.String(), line) {
			t.Fatalf("output not prefixed by gate:\n%s", out.String())
		}
	}
}

func TestGatesInAStageRunTogether(t *testing.T) {
	g := Gates{Stages: []Stage{{Name: "s", Gates: []Gate{{Name: "a", Command: "a"}, {Name: "b", Command: "b"}}}}}
	var started sync.WaitGroup
	started.Add(2)
	run := func(_, _ string, _ io.Writer) error {
		started.Done()
		done := make(chan struct{})
		go func() { started.Wait(); close(done) }()
		select {
		case <-done:
			return nil
		case <-time.After(5 * time.Second):
			return errors.New("the other gate never started")
		}
	}
	if _, err := RunStages(t.TempDir(), g, run, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func TestNextDateTag(t *testing.T) {
	day := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	if got := NextDateTag(day); got != "v0.20261015.0" {
		t.Fatalf("no tags: %s", got)
	}
	local := "v0.20261015.0\nv0.20261014.7"
	remote := "v0.20261015.2\nother/v0.20261015.9"
	if got := NextDateTag(day, local, remote); got != "v0.20261015.3" {
		t.Fatalf("got %s", got)
	}
}

// repo is a clone of a bare origin on main with one commit holding the
// gates file.
func repo(t *testing.T, gates string) (string, Git) {
	t.Helper()
	base, _ := filepath.EvalSymlinks(t.TempDir())
	origin, dir := filepath.Join(base, "origin.git"), filepath.Join(base, "work")
	for _, args := range [][]string{
		{"init", "-q", "--bare", "-b", "main", origin},
		{"clone", "-q", origin, dir},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git := ExecGit(dir)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "keep"), nil, 0o644)
	os.WriteFile(filepath.Join(dir, DefaultGatesFile), []byte(gates), 0o644)
	for _, args := range [][]string{
		{"checkout", "-q", "-b", "main"},
		{"add", "."},
		{"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "first"},
		{"push", "-q", "origin", "main"},
	} {
		if _, err := git(args...); err != nil {
			t.Fatal(err)
		}
	}
	return dir, git
}

func release(dir string, git Git, run Exec, out io.Writer) Release {
	return Release{
		Root: dir, Git: git, Exec: run, Out: out,
		Now:     func() time.Time { return time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC) },
		Confirm: func(string) (bool, error) { return false, nil },
	}
}

func TestTagCreatesAndPushesTheNextTag(t *testing.T) {
	dir, git := repo(t, gatesMarkdown)
	if _, err := git("push", "-q", "origin", "HEAD:refs/tags/v0.20261015.0"); err != nil {
		t.Fatal(err)
	}
	git("tag", "-d", "v0.20261015.0")
	// The gates of a stage run together, so the recorder takes a lock.
	var mu sync.Mutex
	var dirs []string
	run := func(d, _ string, _ io.Writer) error { mu.Lock(); dirs = append(dirs, d); mu.Unlock(); return nil }
	var out bytes.Buffer
	if err := release(dir, git, run, &out).Tag(); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	msg, _ := git("tag", "-l", "--format=%(contents)", "v0.20261015.1")
	if !strings.Contains(msg, "Gates passed:") || !strings.Contains(msg, "test / install") || !strings.Contains(msg, "first") {
		t.Fatalf("annotation:\n%s", msg)
	}
	if remote, _ := RemoteTags(git, "v0.*"); !strings.Contains(remote, "v0.20261015.1") {
		t.Fatalf("push: auto did not push: %q", remote)
	}
	if len(dirs) != 3 || !slices.ContainsFunc(dirs, func(d string) bool { return strings.HasSuffix(d, "/sub") }) {
		t.Fatalf("gate directories %v", dirs)
	}

	out.Reset()
	if err := release(dir, git, run, &out).Tag(); err != nil || !strings.Contains(out.String(), "nothing to release") {
		t.Fatalf("second release: err=%v out=%s", err, out.String())
	}
}

func TestAskLeavesTheTagLocalWhenDeclined(t *testing.T) {
	dir, git := repo(t, gatesFile("ask"))
	var out bytes.Buffer
	if err := release(dir, git, func(string, string, io.Writer) error { return nil }, &out).Tag(); err != nil {
		t.Fatal(err)
	}
	if local, _ := git("tag", "-l"); local != "v0.20261015.0" {
		t.Fatalf("local tags %q", local)
	}
	if remote, _ := RemoteTags(git, "v0.*"); remote != "" || !strings.Contains(out.String(), "stays local") {
		t.Fatalf("remote=%q out=%s", remote, out.String())
	}
}

func TestDryRunCreatesNothing(t *testing.T) {
	dir, git := repo(t, gatesMarkdown)
	var out bytes.Buffer
	if err := release(dir, git, func(string, string, io.Writer) error { return nil }, &out).DryRun(); err != nil {
		t.Fatal(err)
	}
	if local, _ := git("tag", "-l"); local != "" || !strings.Contains(out.String(), "dry run passed") {
		t.Fatalf("tags=%q out=%s", local, out.String())
	}
}

func TestReleaseRefusals(t *testing.T) {
	ok := func(string, string, io.Writer) error { return nil }

	dir, git := repo(t, gatesMarkdown)
	git("checkout", "-q", "-b", "feature")
	if err := release(dir, git, ok, io.Discard).Tag(); err == nil || !strings.Contains(err.Error(), "runs on main") {
		t.Fatalf("off main: %v", err)
	}

	dir, git = repo(t, gatesMarkdown)
	os.WriteFile(filepath.Join(dir, "dirty"), nil, 0o644)
	if err := release(dir, git, ok, io.Discard).Tag(); err == nil || !strings.Contains(err.Error(), "clean worktree") {
		t.Fatalf("dirty: %v", err)
	}

	dir, git = repo(t, gatesMarkdown)
	var once sync.Once
	moves := func(string, string, io.Writer) (err error) {
		once.Do(func() {
			_, err = git("-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "moved")
		})
		return err
	}
	if err := release(dir, git, moves, io.Discard).Tag(); err == nil || !strings.Contains(err.Error(), "HEAD moved") {
		t.Fatalf("moved: %v", err)
	}
	if tags, _ := git("tag", "-l"); tags != "" {
		t.Fatalf("tagged after HEAD moved: %q", tags)
	}

	dir, git = repo(t, gatesMarkdown)
	failing := func(_, _ string, _ io.Writer) error { return errors.New("exit 1") }
	if err := release(dir, git, failing, io.Discard).Tag(); err == nil {
		t.Fatal("tagged after a failed gate")
	}
	if tags, _ := git("tag", "-l"); tags != "" {
		t.Fatalf("tags after a failed gate: %q", tags)
	}
}

func TestLockRefusesAConcurrentRelease(t *testing.T) {
	dir, git := repo(t, gatesMarkdown)
	r := release(dir, git, nil, io.Discard)
	unlock, err := r.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := r.lock(); err == nil || !strings.Contains(err.Error(), "another release") {
		t.Fatalf("second lock: %v", err)
	}
}

// TestRepositoryGatesFileLoads reads this repository's own gates file, the
// one mage tag runs.
func TestRepositoryGatesFileLoads(t *testing.T) {
	g, err := LoadGates(filepath.Join("..", "..", DefaultGatesFile))
	if err != nil {
		t.Fatal(err)
	}
	if g.Push != PushAsk || len(g.Stages) != 2 || g.Stages[0].Gates[0].Command != "mage audit" || len(g.Stages[1].Gates) != 3 || g.Stages[1].Gates[2].Command != "mage crossBuild" {
		t.Fatalf("gates = %+v", g)
	}
}
