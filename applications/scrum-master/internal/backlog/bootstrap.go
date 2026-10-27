// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package backlog

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// Bootstrap is whether the system repository's ARCHITECTURE names this
// subsystem repository before its plan: a component whose id is the
// component name a subsystem SRD of the system declares for the repository,
// the one its component label carries, its dashes read as underscores; the
// repository's name, read the same way, is accepted too (GH-90).
type Bootstrap struct {
	Documents int    `json:"documents"`
	Subsystem string `json:"subsystem"`
	System    string `json:"system,omitempty"`
	Checked   bool   `json:"checked"`
	Named     bool   `json:"named"`
	Reason    string `json:"reason,omitempty"`
}

// Refused reports whether the order is broken: checked, and not named.
func (b *Bootstrap) Refused() bool { return b != nil && b.Checked && !b.Named }

// BootstrapOrder checks a subsystem repository's hierarchy documents
// against the system repository beside it, read from disk as the tracker's
// routing finds it.
func BootstrapOrder(dir string, documents int) *Bootstrap {
	name, origin := filepath.Base(checkout.MainCheckout(dir)), ""
	if out, err := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output(); err == nil && strings.TrimSpace(string(out)) != "" {
		origin = strings.TrimSpace(string(out))
		name = path.Base(program.ProjectPath(origin))
	}
	id := func(s string) string { return strings.ReplaceAll(s, "-", "_") }
	b := &Bootstrap{Documents: documents, Subsystem: id(name)}
	trackers, err := checkout.FindTrackers(dir)
	if err != nil || trackers == "" {
		b.Reason = "no system repository's checkout beside this one, so the order was not checked"
		return b
	}
	system := filepath.Dir(filepath.Dir(trackers))
	b.System, b.Checked = system, true
	repository := b.Subsystem
	if subs, err := program.SubsystemsIn(system); err == nil && origin != "" {
		if c, _, ok := subs.ComponentFor(origin); ok && c.Name != "" {
			b.Subsystem = id(c.Name)
		}
	}
	components, err := hierarchy.Components(system)
	switch {
	case err != nil:
		b.Checked, b.Reason = false, err.Error()
	case components[b.Subsystem] || components[repository]:
		b.Named = true
	default:
		b.Reason = fmt.Sprintf("%d hierarchy documents plan this subsystem, but %s names no component %s; add it to the system repository's ARCHITECTURE first (design.md D15)",
			documents, filepath.Join(filepath.Base(system), hierarchy.ArchitectureFile), b.Subsystem)
	}
	return b
}

// SubsystemSRD is the SRD onboard writes in a subsystem repository, which
// marks the repository a subsystem.
const SubsystemSRD = "docs/specs/software-requirements/srd001-subsystem.md"

// bootstrapFinding is the gate's bootstrapping-order finding for a
// subsystem repository whose plan comes before the system declares it
// (design.md D15, GH-90), so the merge audit refuses it as check does; none
// in any other repository or when the order holds or cannot be checked.
func bootstrapFinding(dir string, documents int) []hierarchy.Finding {
	if documents == 0 {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, SubsystemSRD)); err != nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(dir, program.PolicyFile)); err == nil {
		return nil
	}
	if b := BootstrapOrder(dir, documents); b.Refused() {
		return []hierarchy.Finding{{File: hierarchy.ArchitectureFile, Field: "bootstrapping_order", Message: b.Reason}}
	}
	return nil
}
