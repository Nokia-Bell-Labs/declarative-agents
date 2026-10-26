// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// AuditRunner runs the corpus gate command in a directory and returns its
// combined output. Tests replace it.
type AuditRunner func(dir, command string) (string, error)

// WordsBinary is the name the profiles and the policy's commands give the
// words (GH-32); tpm, the binary that ran them before, still names them in
// a policy an earlier release wrote.
const WordsBinary = "scrum-master"

// Words runs a command line of words in this process, with dir as its
// working directory, and returns its exit code; the words package sets it.
var Words func(dir string, args []string, out io.Writer) int

// wordsMu serializes the commands RunWords runs: a word runs in the
// process's working directory, and gates of one stage run at once.
var wordsMu sync.Mutex

// RunWords runs command in this process when it is a plain command line of
// words — scrum-master or tpm, then its arguments, with no shell syntax —
// and reports whether it did. A merge's audit and a release's gate run a
// policy command through it, so neither needs a words binary installed
// (GH-28, GH-32).
func RunWords(dir, command string, out io.Writer) (bool, error) {
	args := strings.Fields(command)
	if Words == nil || len(args) < 2 || (args[0] != WordsBinary && args[0] != "tpm") || strings.ContainsAny(command, "|&;<>()$`\\\"'*?") {
		return false, nil
	}
	wordsMu.Lock()
	defer wordsMu.Unlock()
	if code := Words(dir, args[1:], out); code != 0 {
		return true, fmt.Errorf("%s exited %d", command, code)
	}
	return true, nil
}

// ShellAudit runs the command with sh -c, or a command line of words in
// process.
func ShellAudit(dir, command string) (string, error) {
	var out bytes.Buffer
	if ok, err := RunWords(dir, command, &out); ok {
		return out.String(), err
	}
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

// MergeGate is what merge checks before merging. The corpus gate runs on a
// branch that changes docs/: the command (mage audit by default, policy
// admission.audit_command) run in Dir, the worktree holding the branch.
// RequireApproval (policy admission.require_approval) refuses a branch
// without another person's approval of its tip.
type MergeGate struct {
	Command         string
	Dir             string
	Run             AuditRunner
	RequireApproval bool
}

// checkCloses parses each --closes reference, project#iid, and refuses one
// that is malformed or missing. One already closed, as a Sprint Goal both a
// team's and the program's sprint-end merges name, is returned apart: the
// merge reports it and closes the rest.
func checkCloses(t tracker.Tracker, closes []string) (open [][2]any, closed []string, err error) {
	for _, ref := range closes {
		p, n, ok := strings.Cut(ref, "#")
		other, err := strconv.Atoi(n)
		if !ok || p == "" || err != nil || other <= 0 {
			return nil, nil, fmt.Errorf("--closes %q is not project#iid", ref)
		}
		c, err := t.GetIssue(p, other)
		if err != nil {
			return nil, nil, fmt.Errorf("--closes %s: %w", ref, err)
		}
		if c.State == "closed" {
			closed = append(closed, ref)
			continue
		}
		open = append(open, [2]any{p, other})
	}
	return open, closed, nil
}

// checkBefore runs the gate, naming what was withheld when it fails.
func (g MergeGate) checkBefore(withheld string) error {
	if g.Command == "" {
		return fmt.Errorf("the branch changes docs/ and no audit command is configured")
	}
	run := g.Run
	if run == nil {
		run = ShellAudit
	}
	out, err := run(g.Dir, g.Command)
	if err == nil {
		return nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return fmt.Errorf("%s failed in %s; %s:\n%s", g.Command, g.Dir, withheld, strings.Join(lines, "\n"))
}
