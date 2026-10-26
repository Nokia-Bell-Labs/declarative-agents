// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package backlog_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
)

// guide prints a shipped guide; with --with-request it follows the guide
// with the request read from its input, and refuses an empty one; an
// unknown guide names those there are (GH-59).
func TestGuidePrintsTheGuideAndTheRequest(t *testing.T) {
	plain, err := run("backlog", "guide", "--name", "plan-vs-architecture")
	if err != nil || !strings.Contains(plain, "ARCHITECTURE") {
		t.Fatalf("the guide = %q, %v", plain[:min(len(plain), 200)], err)
	}
	var out bytes.Buffer
	request := `{"parameters":{"title":"Add the scorer"}}`
	if err := cli.Default.RunIn([]string{"backlog", "guide", "--name", "plan-vs-architecture", "--with-request"}, strings.NewReader(request), &out, noTracker); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "Read the plan-vs-architecture guide below before acting") || !strings.Contains(got, strings.TrimSpace(plain)) ||
		!strings.HasSuffix(strings.TrimSpace(got), "The request:\n\n"+request) {
		t.Fatalf("guide with the request:\n%s", got)
	}
	if err := cli.Default.RunIn([]string{"backlog", "guide", "--name", "plan-vs-architecture", "--with-request"}, strings.NewReader(" \n"), &out, noTracker); err == nil {
		t.Fatal("an empty request was accepted")
	}
	if _, err := run("backlog", "guide", "--name", "nope"); err == nil || !strings.Contains(err.Error(), "plan-vs-architecture") {
		t.Fatalf("an unknown guide = %v, want the names there are", err)
	}
}
