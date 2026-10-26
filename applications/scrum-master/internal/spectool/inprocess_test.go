// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spectool

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// In an agent that linked the program, Run runs it in process in the
// repository's directory, with no release laid out, and reports its lines
// and its failure as the built tool's (GH-33).
func TestRunUsesTheLinkedProgram(t *testing.T) {
	dir := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	before, _ := os.Getwd()
	var gotArgs []string
	var gotDir string
	prev := InProcess
	InProcess = func(args []string, stdout, stderr io.Writer) int {
		gotArgs = args
		gotDir, _ = os.Getwd()
		fmt.Fprintln(stdout, "docs/ARCHITECTURE.md:15: heading-id-mismatch: x")
		fmt.Fprintln(stderr, "spectool: 1 problem(s) in 3 document(s)")
		return 1
	}
	defer func() { InProcess = prev }()
	lines, err := Run(dir, "check", "all")
	if strings.Join(gotArgs, " ") != "check all" || gotDir != dir {
		t.Fatalf("ran %v in %s, want check all in %s", gotArgs, gotDir, dir)
	}
	if len(lines) != 1 || err == nil || err.Error() != "1 problem(s) in 3 document(s)" {
		t.Fatalf("lines %v err %v", lines, err)
	}
	if now, _ := os.Getwd(); now != before {
		t.Fatalf("the working directory is %s, want %s back", now, before)
	}
}
