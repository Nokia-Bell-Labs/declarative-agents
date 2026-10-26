// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package words

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// The issue group serves every subcommand, and program is not a group
// (srd013 R2).
func TestIssueGroupServesSubcommands(t *testing.T) {
	var names []string
	for _, c := range cli.Default.Commands("issue") {
		names = append(names, c.Name)
	}
	for _, want := range []string{"approve", "close", "file", "labels", "lint", "merge", "pop", "recur"} {
		if !slices.Contains(names, want) {
			t.Errorf("issue has no %s; it has %v", want, names)
		}
	}
	if groups := cli.Default.Groups(); slices.Contains(groups, "program") {
		t.Errorf("program is still a group: %v", groups)
	}
}

// The retired program group runs nothing and names what replaced it and
// how a skill installed by an earlier release is updated (srd013 R2.2).
func TestProgramGroupRefusesWithPointer(t *testing.T) {
	opened := false
	open := func() (tracker.Tracker, error) { opened = true; return nil, errors.New("no tracker in this test") }
	for _, args := range [][]string{{"program"}, {"program", "pop", "--project", "g/po-1", "--issue", "1"}} {
		var out bytes.Buffer
		err := cli.Default.Run(args, &out, open)
		if err == nil || errors.Is(err, cli.ErrUsage) {
			t.Fatalf("%s: want a non-usage error, got %v", strings.Join(args, " "), err)
		}
		for _, want := range []string{"scrum-master issue", "run the upgrade profile"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: %q names no %q", strings.Join(args, " "), err, want)
			}
		}
		if len(args) > 1 && !strings.Contains(err.Error(), "scrum-master issue pop") {
			t.Errorf("%s: %q does not name the subcommand to run", strings.Join(args, " "), err)
		}
	}
	if opened {
		t.Error("program opened the tracker; it must run nothing")
	}
}
