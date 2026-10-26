// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package sprintend

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
)

// TestSprintEndRecordsStoriesDone records the listed stories the owner
// reports done in the sprint-end summary, replaces the record on a second
// run rather than appending, and reads the newest record back for velocity
// (srd038 R2.4, srd045 R5.3).
func TestSprintEndRecordsStoriesDone(t *testing.T) {
	dir := t.TempDir()
	ends := filepath.Join(dir, sprintgoal.SummaryDir)
	if err := os.MkdirAll(ends, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, text string) string {
		p := filepath.Join(ends, name)
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write("01.9.md", "# Sprint end 01.9\n\n## Stories done\n\n- st001-old\n")
	p := write("01.10.md", "# Sprint end 01.10\n\nThe grader shipped.\n")

	if changed, err := sprintgoal.RecordStoriesDone(p, []string{"st002-store-scores", "st003-scorecard-page"}, true); err != nil || !changed {
		t.Fatalf("dry run changed=%v err=%v", changed, err)
	}
	if data, _ := os.ReadFile(p); strings.Contains(string(data), "Stories done") {
		t.Fatal("dry run wrote the summary")
	}
	if changed, err := sprintgoal.RecordStoriesDone(p, []string{"st002-store-scores", "st003-scorecard-page"}, false); err != nil || !changed {
		t.Fatalf("record changed=%v err=%v", changed, err)
	}
	if changed, err := sprintgoal.RecordStoriesDone(p, []string{"st002-store-scores", "st003-scorecard-page"}, false); err != nil || changed {
		t.Fatalf("second record changed=%v err=%v", changed, err)
	}
	if _, err := sprintgoal.RecordStoriesDone(p, []string{"st002-store-scores"}, false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(p)
	if strings.Count(string(data), "## Stories done") != 1 || !strings.Contains(string(data), "The grader shipped.") || strings.Contains(string(data), "st003") {
		t.Fatalf("summary:\n%s", data)
	}
	done, err := sprintgoal.StoriesDone(dir)
	if err != nil || done.Latest != "01.10" || !slices.Equal(done.Last, []string{"st002-store-scores"}) || done.Release["st001-old"] != "01.9" {
		t.Fatalf("done %+v err %v", done, err)
	}
	if got := sprintgoal.SprintBacklog("x\n## Sprint Backlog\n\n- st002-store-scores Store scores (3 points)\n- st003-a A (8 points)\n\n<!-- scrum-master goals sprint-goal release=01.1 serves=a#R1 -->\n"); !slices.Equal(got, []string{"st002-store-scores", "st003-a"}) {
		t.Fatalf("sprint backlog %v", got)
	}
}

// TestSummaryFormat checks a summary's Stories done section: project#story
// entries, or None.; the backlog reads its own stories back by id.
func TestSummaryFormat(t *testing.T) {
	for text, want := range map[string]int{
		"# Sprint end 01.0\n\n## Stories done\n\n- g/po-1#st001-score\n- g/agent-ops#st001-deploy\n": 0,
		"# Sprint end 01.0\n\n## Stories done\n\nNone.\n":                                            0,
		"# Sprint end 01.0\n\nNo stories section.\n":                                                 1,
		"# Sprint end 01.0\n\n## Stories done\n\n- st001-score\n":                                    1,
		"# Sprint end 01.0\n\n## Stories done\n\n":                                                   1,
	} {
		if got := sprintgoal.CheckSummary(text); len(got) != want {
			t.Errorf("CheckSummary(%q) = %v, want %d problem(s)", text, got, want)
		}
	}
	dir := t.TempDir()
	ends := filepath.Join(dir, sprintgoal.SummaryDir)
	if err := os.MkdirAll(ends, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ends, "01.0.md"), []byte("# Sprint end 01.0\n\n## Stories done\n\n- g/po-1#st001-score\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if done, err := sprintgoal.StoriesDone(dir); err != nil || !slices.Equal(done.Last, []string{"st001-score"}) {
		t.Fatalf("done %+v, %v", done, err)
	}
}
