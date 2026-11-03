// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package cli

import (
	"flag"
	"io"
	"os/exec"
	"testing"
)

func TestFillFromCheckout(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	git("init", "-q", "-b", "main")
	git("remote", "add", "origin", "https://gitlab.example.com/g/sub/repo.git")
	parse := func(args ...string) (string, int) {
		t.Helper()
		fs := flag.NewFlagSet("x", flag.ContinueOnError)
		project := fs.String("project", "", "")
		issue := fs.Int("issue", 0, "")
		fs.String("dir", dir, "")
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		fillFromCheckout(fs)
		return *project, *issue
	}
	if p, i := parse(); p != "g/sub/repo" || i != 0 {
		t.Fatalf("on main = %q, %d", p, i)
	}
	git("checkout", "-q", "-b", "7-some-title")
	if p, i := parse(); p != "g/sub/repo" || i != 7 {
		t.Fatalf("on an issue's branch = %q, %d", p, i)
	}
	if p, i := parse("--project", "g/other", "--issue", "9"); p != "g/other" || i != 9 {
		t.Fatalf("flags given = %q, %d", p, i)
	}
	git("checkout", "-q", "-b", "feature")
	if _, i := parse(); i != 0 {
		t.Fatalf("a branch named for no issue = %d", i)
	}
	git("remote", "set-url", "origin", "/srv/git/repo.git")
	if p, _ := parse(); p != "" {
		t.Fatalf("a local origin names no project, got %q", p)
	}
}

// TestAnEmptyFlagFromAnUnsetVariable: a profile's --issue=${VAR} reads empty when the
// variable is unset, which must mean unset and not a parse error.
func TestAnEmptyFlagFromAnUnsetVariable(t *testing.T) {
	r := &Registry{}
	var project string
	var issue int
	r.Register(&Command{Group: "x", Name: "y", FromCheckout: true,
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&project, "project", "", "")
			fs.IntVar(&issue, "issue", 0, "")
		},
		Run: func(*Context) error { return nil },
	})
	if err := r.Run([]string{"x", "y", "--project=", "--issue="}, io.Discard, nil); err != nil {
		t.Fatalf("empty flags = %v", err)
	}
	if issue != 0 {
		t.Fatalf("issue = %d", issue)
	}
	if err := r.Run([]string{"x", "y", "--project=g/p", "--issue=7"}, io.Discard, nil); err != nil || project != "g/p" || issue != 7 {
		t.Fatalf("given = %q, %d, %v", project, issue, err)
	}
}
