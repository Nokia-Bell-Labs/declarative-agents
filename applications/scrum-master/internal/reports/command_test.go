// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// TestWeekGathersTheWeekOfDate gathers the week --date names, not the
// current one.
func TestWeekGathersTheWeekOfDate(t *testing.T) {
	var out bytes.Buffer
	err := cli.Default.Run([]string{"reports", "week", "--json", "--project", "g/po-1", "--user", "po-1", "--date", "2026-10-14"},
		&out, func() (tracker.Tracker, error) { return fake.New(), nil })
	if err != nil {
		t.Fatal(err)
	}
	var r WeekResult
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || r.Week != "2026-W42" || r.Since != "2026-10-12" {
		t.Fatalf("week = %+v, %v\n%s", r, err, out.String())
	}
}

// TestReportsInitInstallsSkills installs the skills for --agent in the
// reports checkout, as init and onboard do.
func TestReportsInitInstallsSkills(t *testing.T) {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q", "-b", "main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	var out bytes.Buffer
	err := cli.Default.Run([]string{"reports", "init", "--json", "--dir", dir, "--system", "g/system", "--agent", "claude"},
		&out, func() (tracker.Tracker, error) { return fake.New(), nil })
	if err != nil {
		t.Fatal(err)
	}
	var r struct {
		Files  []InitResult `json:"files"`
		Skills []struct {
			Path string `json:"path"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(out.Bytes(), &r); err != nil || len(r.Files) == 0 || len(r.Skills) == 0 {
		t.Fatalf("init = %+v, %v\n%s", r, err, out.String())
	}
	if _, err := os.Stat(r.Skills[0].Path); err != nil {
		t.Fatalf("skill %s not installed: %v", r.Skills[0].Path, err)
	}
}
