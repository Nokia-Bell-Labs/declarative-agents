// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The application roots at its go.mod, not the monorepo's git toplevel, so
// every gate and rig path stays inside applications/scrum-master (GH-190).
func TestRepositoryRootIsTheApplication(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}
	if _, serr := os.Stat(filepath.Join(root, "go.mod")); serr != nil || filepath.Base(root) == "magefiles" {
		t.Fatalf("root = %q, want the directory holding go.mod: %v", root, serr)
	}
	if _, serr := os.Stat(filepath.Join(root, "plugin", "agents")); serr != nil {
		t.Fatalf("root %q does not hold the application's profiles: %v", root, serr)
	}
}

// TestAuditFailsOnMissingIndexedDocument proves the acceptance criterion of
// GH-39: a corpus whose SPECIFICATIONS.md index names a document that does
// not exist fails the corpus audit with index-broken-path. It runs the
// sibling agent on a copy of this corpus with docs/VISION.md removed; it
// builds the agent, so it runs only when SCRUM_MASTER_AUDIT_FIXTURE is set.
func TestAuditFailsOnMissingIndexedDocument(t *testing.T) {
	if os.Getenv("SCRUM_MASTER_AUDIT_FIXTURE") == "" {
		t.Skip("set SCRUM_MASTER_AUDIT_FIXTURE to build the sibling agent and run the fixture")
	}
	work := t.TempDir()
	if err := runStreaming(work, "cp", "-R", mustAbs(t, filepath.Join("..", "docs")), "."); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(work, "docs", "VISION.md")); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	capture := func(dir, name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		cmd.Stdout = &out
		cmd.Stderr = &out
		return cmd.Run()
	}
	err := runCorpusAudit(work, capture)
	if err == nil {
		t.Fatal("the audit passed a corpus whose index names a missing document")
	}
	if !strings.Contains(out.String(), "index-broken-path") {
		t.Fatalf("the audit failed without an index-broken-path finding:\n%s", out.String())
	}
}

// Inside the monorepo the audit runs agent-core's gates there, and a gate's
// failure fails the audit; outside it there is nothing to run (GH-112).
func TestAuditRunsTheRuntimeGates(t *testing.T) {
	mono := t.TempDir()
	root := filepath.Join(mono, "applications", "scrum-master")
	writeFile(t, filepath.Join(mono, "agent-core", "go.mod"), "module x\n")
	var ran []string
	record := func(dir, name string, args ...string) error {
		ran = append(ran, dir+": "+name+" "+strings.Join(args, " "))
		return nil
	}
	if err := runRuntimeGates(root, record); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 1 || !strings.HasPrefix(ran[0], filepath.Join(root, "..", "..", "agent-core")+": go test") || !strings.Contains(ran[0], "./internal/gostyle/") {
		t.Fatalf("ran %q", ran)
	}
	failing := func(string, string, ...string) error { return errors.New("exit status 1") }
	if err := runRuntimeGates(root, failing); err == nil || !strings.Contains(err.Error(), "agent-core gates") {
		t.Fatalf("a failing gate gave %v", err)
	}
	ran = nil
	if err := runRuntimeGates(t.TempDir(), record); err != nil || len(ran) != 0 {
		t.Fatalf("outside the monorepo: ran %q, %v", ran, err)
	}
}
