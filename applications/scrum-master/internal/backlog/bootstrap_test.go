// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
)

// The gate refuses a subsystem repository's plan before the system
// repository beside it declares the subsystem, so the merge audit, which
// runs the gate, refuses it as check does (design.md D15, GH-90); once the
// system declares it, the finding is gone.
func TestGateRefusesAPlanBeforeTheSystemDeclaresIt(t *testing.T) {
	parent := t.TempDir()
	member, system := filepath.Join(parent, "po-1"), filepath.Join(parent, "system")
	write := func(dir, p, body string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	epic, err := os.ReadFile(filepath.Join("..", "..", "testdata", "architecture", "docs", "specs", "epics", "ep001-publish-scorecards.md"))
	if err != nil {
		t.Fatal(err)
	}
	write(member, backlog.SubsystemSRD, "---\nid: srd001-subsystem\ntitle: Subsystem\n---\n")
	write(member, "docs/specs/epics/ep001-publish-scorecards.md", string(epic))
	write(system, "control/trackers.md", "bd\n")
	arch := func(component string) string {
		return "---\nid: architecture-system\ntitle: System\n---\n\n<!-- field: components -->\n## Components\n\n<!-- component: " + component + " -->\n### " + component + "\n\nA subsystem.\n"
	}
	refused := func() bool {
		t.Helper()
		r, err := backlog.Check(member)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range r.Findings {
			if f.Field == "bootstrapping_order" {
				return true
			}
		}
		return false
	}
	write(system, "docs/ARCHITECTURE.md", arch("po_2"))
	if !refused() {
		t.Fatal("a plan before the system declares the subsystem passed the gate")
	}
	write(system, "docs/ARCHITECTURE.md", arch("po_1"))
	if refused() {
		t.Fatal("a plan after the system declares the subsystem was refused")
	}
}
