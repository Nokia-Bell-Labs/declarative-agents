// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/trackertest"
)

// TestIssueCommentPostsANote is srd013 R9: scrum-master issue comment posts the
// person's text on an issue once, on every tracking system, refuses an
// empty body and both body flags, and a dry run posts nothing (#188).
func TestIssueCommentPostsANote(t *testing.T) {
	trackertest.Each(t, func(t *testing.T, p trackertest.Program) {
		f := p.Tracker()
		i, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Score one agent"})
		if err != nil {
			t.Fatal(err)
		}
		run := func(args ...string) (string, error) {
			var out bytes.Buffer
			err := cli.Default.Run(append([]string{"issue", "comment", "--project", "g/po-1"}, args...), &out, func() (tracker.Tracker, error) { return f, nil })
			return out.String(), err
		}
		iid := strconv.Itoa(i.IID)
		if _, err := run("--issue", iid, "--dry-run", "--body", "Actual LOC: 42"); err != nil {
			t.Fatal(err)
		}
		if notes, _ := f.ListNotes("g/po-1", i.IID); len(notes) != 0 {
			t.Fatalf("a dry run posted %+v", notes)
		}
		if _, err := run("--issue", iid, "--body", "Actual LOC: 42 (Estimated: 40)"); err != nil {
			t.Fatal(err)
		}
		notes, _ := f.ListNotes("g/po-1", i.IID)
		if len(notes) != 1 || !strings.Contains(notes[0].Body, "Actual LOC: 42") {
			t.Fatalf("notes = %+v, want the one comment", notes)
		}
		for _, bad := range [][]string{{"--issue", iid, "--body", "  "}, {"--issue", iid, "--body", "a", "--body-file", "x"}, {"--issue", "999", "--body", "a"}} {
			if _, err := run(bad...); err == nil {
				t.Errorf("issue comment %v succeeded, want a refusal", bad)
			}
		}
	})
}
