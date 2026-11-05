// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package words

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/version"
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

// An agent older than the repository's release still runs its words, and the
// run names the drift once, on staleOut, however many words it runs, leaving
// each word's output as it was (GH-2899).
func TestAnOlderAgentIsNamedOnceAndTheWordStillRuns(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, install.VendorDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, install.ManifestFile), []byte(`{"release":"v0.20261103.4","files":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	saved, savedOut := version.Release, staleOut
	var warned bytes.Buffer
	version.Release, staleOut, staleOnce = "v0.20261103.2", &warned, sync.Once{}
	defer func() { version.Release, staleOut, staleOnce = saved, savedOut, sync.Once{} }()
	var out bytes.Buffer
	for range 2 {
		if code := RunIn(dir, []string{"install", "--list"}, nil, &out); code != ExitOK {
			t.Fatalf("exit %d: %s", code, out.String())
		}
	}
	if n := strings.Count(warned.String(), "this agent is v0.20261103.2"); n != 1 {
		t.Errorf("warned %d times: %q", n, warned.String())
	}
	if !strings.Contains(warned.String(), "install.sh v0.20261103.4") {
		t.Errorf("the warning names no fix: %q", warned.String())
	}
	if strings.Contains(out.String(), "warning") {
		t.Errorf("the warning reached the word's output: %q", out.String())
	}
}

// An agent older than the repository's release writes no skills, since the
// skills it holds are the older release's (GH-2899).
func TestAnOlderAgentWritesNoSkills(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, install.VendorDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, install.ManifestFile), []byte(`{"release":"v0.20261103.4","files":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	saved := version.Release
	version.Release = "v0.20261103.2"
	defer func() { version.Release = saved }()
	var out bytes.Buffer
	if code := RunIn(dir, []string{"install", "--agent", "claude", "--dir", dir}, nil, &out); code != ExitError {
		t.Fatalf("exit %d, want a refusal: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "no skills were written") {
		t.Errorf("the refusal = %q", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("skills were written: %v", err)
	}
}

// The words tell routing what a system checkout's subsystem SRDs declare, so a
// GitHub group's labels and milestones stay within them (GH-2903).
func TestRoutingReadsTheRegistry(t *testing.T) {
	dir := t.TempDir()
	srd := filepath.Join(dir, program.SRDDir, "srd020-team.md")
	if err := os.MkdirAll(filepath.Dir(srd), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nid: srd020-team\ntitle: Team\n---\n\n<!-- field: context | form: \"tree\" -->\n## Context\n\n- owner: Product Owner, team\n- scrum_repository: https://github.com/acme/plan.git\n\n" +
		"<!-- field: implementation_targets -->\n## Implementation targets\n\n| name | repository | path |\n| --- | --- | --- |\n| code | https://github.com/acme/code | / |\n"
	if err := os.WriteFile(srd, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := routing.Registered(dir)
	if !slices.Contains(got, "acme/plan") || !slices.Contains(got, "acme/code") {
		t.Fatalf("registered = %v, want the scrum and code repositories", got)
	}
	if slices.Contains(got, "acme/site") {
		t.Fatalf("registered = %v, naming a site repository nothing declares", got)
	}
	lineage := filepath.Join(dir, program.LineageFile)
	if err := os.MkdirAll(filepath.Dir(lineage), 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "---\nid: program\ntitle: Program\n---\n\n<!-- field: site_repository -->\n## Site repository\n\nhttps://github.com/acme/site.git\n"
	if err := os.WriteFile(lineage, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := routing.Registered(dir); !slices.Contains(got, "acme/site") {
		t.Fatalf("registered = %v, want the declared site repository", got)
	}
}
