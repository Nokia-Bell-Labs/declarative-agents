// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package plan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
)

// A release's milestone is dated in the group named, else the program group
// control/trackers.md records, else the system project's parent group
// (srd031 R4.2, GH-103).
func TestTheMilestoneGroup(t *testing.T) {
	dir := t.TempDir()
	if g := programGroup("", dir, "acme-labs/aeop"); g != "acme-labs" {
		t.Fatalf("with no group recorded: %q", g)
	}
	os.MkdirAll(filepath.Join(dir, "control"), 0o755)
	os.WriteFile(filepath.Join(dir, routing.File), routing.Document(routing.Config{Group: "aeop", Default: routing.System{Name: routing.GitHub}}), 0o644)
	if g := programGroup("", dir, "acme-labs/aeop"); g != "aeop" {
		t.Fatalf("with aeop recorded: %q", g)
	}
	if g := programGroup("other", dir, "acme-labs/aeop"); g != "other" {
		t.Fatalf("with a group named: %q", g)
	}
}
