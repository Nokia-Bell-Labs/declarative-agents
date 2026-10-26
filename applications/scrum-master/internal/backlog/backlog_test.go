// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	_ "github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/commands"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

const fixtures = "../../testdata/hierarchy"

func noTracker() (tracker.Tracker, error) { return nil, errors.New("backlog reads no tracker") }

func run(args ...string) (string, error) {
	var out bytes.Buffer
	err := cli.Default.Run(args, &out, noTracker)
	return out.String(), err
}

func TestListNamesEveryDocument(t *testing.T) {
	entries, err := backlog.List(filepath.Join(fixtures, "valid"), "")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, e := range entries {
		kinds[e.Kind]++
		if e.ID == "" || e.Title == "" || e.Status == "" || !strings.HasPrefix(e.Path, "docs/specs/") {
			t.Errorf("incomplete entry: %+v", e)
		}
		if e.Kind == "story" && (e.Points == "" || e.Rank == "") {
			t.Errorf("story without points or rank: %+v", e)
		}
		if e.Kind != "story" && (e.Points != "" || e.Rank != "") {
			t.Errorf("non-story with points or rank: %+v", e)
		}
	}
	want := map[string]int{"theme": 1, "initiative": 2, "epic": 1, "feature": 1, "story": 4}
	if !reflect.DeepEqual(kinds, want) {
		t.Errorf("kinds %v, want %v", kinds, want)
	}
}

func TestListKeepsOneKind(t *testing.T) {
	out, err := run("backlog", "list", "--dir", filepath.Join(fixtures, "valid"), "--kind", "story", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var entries []backlog.Entry
	if err := json.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if len(entries) != 4 {
		t.Fatalf("%d stories, want 4", len(entries))
	}
	for _, e := range entries {
		if e.Kind != "story" || e.Parent == "" {
			t.Errorf("entry %+v", e)
		}
	}
	if _, err := run("backlog", "list", "--dir", fixtures, "--kind", "task"); !errors.Is(err, cli.ErrUsage) {
		t.Errorf("unknown kind: %v, want a usage error", err)
	}
}

func TestListEmptyRepository(t *testing.T) {
	out, err := run("backlog", "list", "--dir", t.TempDir(), "--json")
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Errorf("empty repository: %q, %v", out, err)
	}
}

func TestCheckPassesValid(t *testing.T) {
	out, err := run("backlog", "check", "--dir", filepath.Join(fixtures, "valid"))
	if err != nil || !strings.HasPrefix(out, "hierarchy gate: pass, ") {
		t.Errorf("valid fixture: %q, %v", out, err)
	}
}

// TestCheckIsTheGate proves check reports exactly hierarchy.Validate's
// findings, the function mage audit's gate calls, on every malformed case,
// and fails each one.
func TestCheckIsTheGate(t *testing.T) {
	cases, err := os.ReadDir(filepath.Join(fixtures, "malformed"))
	if err != nil || len(cases) == 0 {
		t.Fatalf("no malformed fixtures: %v", err)
	}
	for _, c := range cases {
		dir := filepath.Join(fixtures, "malformed", c.Name())
		docs, err := hierarchy.Load(dir)
		if err != nil {
			t.Fatal(err)
		}
		var gate []string
		for _, f := range hierarchy.Validate(docs) {
			rel, _ := filepath.Rel(dir, f.File)
			gate = append(gate, rel+": "+f.Field+": "+f.Message)
		}
		out, err := run("backlog", "check", "--dir", dir, "--json")
		if err == nil {
			t.Errorf("%s: check passed a malformed repository", c.Name())
		}
		var r backlog.Report
		if jerr := json.Unmarshal([]byte(out), &r); jerr != nil {
			t.Fatalf("%s: %v: %s", c.Name(), jerr, out)
		}
		var got []string
		for _, f := range r.Findings {
			got = append(got, f.String())
		}
		if len(got) == 0 || !reflect.DeepEqual(got, gate) {
			t.Errorf("%s: check %v, gate %v", c.Name(), got, gate)
		}
	}
}

func TestCheckWritesNothing(t *testing.T) {
	dir := filepath.Join(fixtures, "valid")
	before := snapshot(t, dir)
	if _, err := run("backlog", "check", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := run("backlog", "list", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Error("check or list changed the repository")
	}
}

func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		files[p] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// backlog list carries each document's horizontal links, so a reader of
// --json sees what a story is blocked by or depends on (GH-20).
func TestListCarriesTheLinks(t *testing.T) {
	entries, err := backlog.List(fixtures+"/valid", "story")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.ID == "st002-store-scores" {
			if len(e.BlockedBy) == 0 {
				t.Fatalf("st002's blocked_by is lost: %+v", e)
			}
			return
		}
	}
	t.Fatalf("no st002 in %+v", entries)
}
