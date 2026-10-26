// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
)

// areaChecks hold each document to its repository's area, the subsystem
// SRDs being the program's registry (srd045 R6.5, R6.6; GH-2726). In a team's
// repository, a story or feature names in architecture only components
// the subsystem owning the repository lists. In the system repository, a
// cross-team epic is the technical program manager's, not a Product
// Owner's, and spans more than one team's area: one whose components all
// belong to one subsystem belongs in that team's repository. Everything is
// read from disk, the system checkout's SRDs and policy, as the gate opens
// no tracker (srd045 R8.3); with no system checkout there is no registry
// and nothing to check.
func areaChecks(dir string, docs []*hierarchy.Doc) (findings []hierarchy.Finding, refuse bool, err error) {
	trackers, err := checkout.FindTrackers(dir)
	if err != nil || trackers == "" {
		return nil, false, nil
	}
	systemDir := filepath.Dir(filepath.Dir(trackers))
	subs, err := program.SubsystemsIn(systemDir)
	if err != nil {
		return nil, false, err
	}
	system := program.SystemControl(dir)
	teams := subs.Teams(system)
	if len(teams) == 0 {
		return nil, false, nil
	}
	if sameDir(dir, systemDir) {
		findings = systemAreaChecks(docs, teams)
	} else if sub, ok := repositorySubsystem(dir, teams); ok {
		findings = teamAreaChecks(docs, sub, teams)
	}
	data, err := policyData(dir)
	if err != nil || data == nil {
		return findings, false, err
	}
	checks, err := program.AreaChecksIn(data)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", program.PolicyFile, err)
	}
	return findings, checks == program.ArchitectureRefuse, nil
}

// teamAreaChecks warns for a story or feature that names a component the
// subsystem owning its repository does not list.
func teamAreaChecks(docs []*hierarchy.Doc, sub program.Subsystem, teams program.Subsystems) []hierarchy.Finding {
	var out []hierarchy.Finding
	for _, d := range docs {
		if d.Kind != hierarchy.Story && d.Kind != hierarchy.Feature {
			continue
		}
		for _, a := range d.Architecture {
			c := hierarchy.ComponentOf(a)
			if sub.Lists(c) {
				continue
			}
			msg := fmt.Sprintf("outside its area: names %s, which %s, the subsystem owning this repository, does not list", c, sub.ID)
			if owner, ok := teams.Owner(c); ok {
				msg += fmt.Sprintf("; %s lists it, so the work is that team's", owner.ID)
			}
			out = append(out, hierarchy.Finding{File: d.Path, Field: "architecture", Message: msg})
		}
	}
	return out
}

// systemAreaChecks warns for a system-repository epic a Product Owner owns,
// and for one whose components all belong to one team's subsystem.
func systemAreaChecks(docs []*hierarchy.Doc, teams program.Subsystems) []hierarchy.Finding {
	var out []hierarchy.Finding
	for _, d := range docs {
		if d.Kind != hierarchy.Epic {
			continue
		}
		if strings.Contains(strings.ToLower(d.Owner), "product owner") {
			out = append(out, hierarchy.Finding{File: d.Path, Field: "owner",
				Message: "a cross-team epic in the system repository is the technical program manager's; an epic one Product Owner owns lives in that team's repository"})
		}
		var owners []string
		for _, a := range d.Architecture {
			sub, ok := teams.Owner(hierarchy.ComponentOf(a))
			if !ok {
				owners = nil
				break
			}
			if !slices.Contains(owners, sub.ID) {
				owners = append(owners, sub.ID)
			}
		}
		if len(owners) == 1 {
			out = append(out, hierarchy.Finding{File: d.Path, Field: "architecture",
				Message: fmt.Sprintf("one team's epic: every component it names belongs to %s, so it belongs in that team's repository", owners[0])})
		}
	}
	return out
}

// repositorySubsystem is the team subsystem whose SRD lists the
// repository checked out at dir, by its origin's project path or, for an
// origin that is a local path or none, by its name.
func repositorySubsystem(dir string, teams program.Subsystems) (program.Subsystem, bool) {
	root := dir
	if out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
		root = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "-C", root, "config", "--get", "remote.origin.url").Output(); err == nil {
		if origin := strings.TrimSpace(string(out)); origin != "" && !strings.HasPrefix(origin, "/") {
			if sub, ok := teams.SubsystemFor(origin); ok {
				return sub, true
			}
		}
	}
	name := filepath.Base(root)
	for _, sub := range teams {
		for _, c := range sub.Components {
			if path.Base(program.ProjectPath(c.Repository)) == name {
				return sub, true
			}
		}
	}
	return program.Subsystem{}, false
}

// sameDir reports whether two paths are one directory.
func sameDir(a, b string) bool {
	canon := func(p string) string {
		abs, _ := filepath.Abs(p)
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			return real
		}
		return abs
	}
	return canon(a) == canon(b)
}
