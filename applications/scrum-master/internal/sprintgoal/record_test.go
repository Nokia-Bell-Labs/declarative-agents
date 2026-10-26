// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package sprintgoal

import (
	"os"
	"path/filepath"
	"testing"
)

// Stories done says no story was done as None, None., or - None, as
// design.md's "or None" admits; none of them counts as a story (GH-63).
func TestStoriesDoneAcceptsEveryFormOfNone(t *testing.T) {
	for _, none := range []string{"None", "None.", "- None", "- None.", "none"} {
		text := "# Sprint end\n\n" + storiesHeading + "\n\n" + none + "\n"
		if p := CheckSummary(text); len(p) != 0 {
			t.Errorf("%q: %v", none, p)
		}
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, SummaryDir), 0o755)
		os.WriteFile(filepath.Join(dir, SummaryDir, "01.0.md"), []byte(text), 0o644)
		d, err := StoriesDone(dir)
		if err != nil || len(d.Release) != 0 || len(d.Last) != 0 {
			t.Errorf("%q read as stories done: %+v, %v", none, d, err)
		}
	}
	if p := CheckSummary("# Sprint end\n\n" + storiesHeading + "\n\n- st001\n"); len(p) != 1 {
		t.Errorf("an entry without its project = %v, want one problem", p)
	}
	if p := CheckSummary("# Sprint end\n\n" + storiesHeading + "\n\nnothing\n"); len(p) != 1 {
		t.Errorf("a section saying neither = %v, want one problem", p)
	}
}
