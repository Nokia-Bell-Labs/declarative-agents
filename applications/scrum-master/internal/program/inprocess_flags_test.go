// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package program

import (
	"flag"
	"io"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
)

// The words run many times in one agent process (GH-28): a repeatable flag
// one run set must be empty on the next.
func TestRepeatableFlagsStartEmptyEachRun(t *testing.T) {
	for _, name := range []string{"merge", "merge-branch"} {
		var cmd *cli.Command
		for _, k := range cli.Default.Commands("issue") {
			if k.Name == name {
				cmd = k
			}
		}
		if cmd == nil {
			t.Fatalf("no command issue %s", name)
		}
		first := flag.NewFlagSet("first", flag.ContinueOnError)
		first.SetOutput(io.Discard)
		cmd.Flags(first)
		if err := first.Parse([]string{"--closes", "g/po-1#1"}); err != nil {
			t.Fatal(err)
		}
		second := flag.NewFlagSet("second", flag.ContinueOnError)
		cmd.Flags(second)
		if v := second.Lookup("closes").Value.String(); v != "" {
			t.Errorf("issue %s --closes keeps %q from an earlier run", name, v)
		}
	}
}

// A scrum-master audit command runs through the words in this process, so a merge's
// gate needs no scrum-master installed; a shell command still runs in sh (GH-28).
func TestAuditRunsATpmCommandInProcess(t *testing.T) {
	var got []string
	var gotDir string
	prev := Words
	Words = func(dir string, args []string, out io.Writer) int {
		gotDir, got = dir, args
		io.WriteString(out, "hierarchy gate: 1 findings")
		return 1
	}
	defer func() { Words = prev }()
	out, err := ShellAudit("/repo", BacklogAudit)
	if err == nil || gotDir != "/repo" || strings.Join(got, " ") != "backlog check" || !strings.Contains(out, "1 findings") {
		t.Fatalf("dir %q args %v out %q err %v", gotDir, got, out, err)
	}
	got = nil
	if _, err := ShellAudit(t.TempDir(), "scrum-master backlog check | cat"); err != nil || got != nil {
		t.Fatalf("a pipeline must run in sh: args %v err %v", got, err)
	}
}
