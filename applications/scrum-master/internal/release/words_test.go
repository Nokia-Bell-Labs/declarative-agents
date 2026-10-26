// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package release

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// A gate whose command is a command line of words runs in process, so a
// release needs no words binary installed (GH-32).
func TestGateRunsAWordsCommandInProcess(t *testing.T) {
	var got []string
	prev := program.Words
	program.Words = func(dir string, args []string, out io.Writer) int {
		got = append([]string{dir}, args...)
		io.WriteString(out, "hierarchy gate: pass")
		return 0
	}
	defer func() { program.Words = prev }()
	var out bytes.Buffer
	if err := ShellExec("/repo", program.BacklogAudit, &out); err != nil || strings.Join(got, " ") != "/repo backlog check" || out.String() != "hierarchy gate: pass" {
		t.Fatalf("ran %v, out %q, err %v", got, out.String(), err)
	}
}
