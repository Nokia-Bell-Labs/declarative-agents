// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/repofiles"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// TeamSource is one team repository whose documents roll up to this one's:
// its name, which its documents carry as <name>/<id> — its project path
// below the program's group (GH-2811) — and a directory holding its docs/
// to read as a checkout's.
type TeamSource struct {
	Name       string `json:"name"`
	Repository string `json:"repository,omitempty"`
	Dir        string `json:"-"`
	Source     string `json:"source"`
	// Initiative is the initiative document the repository's SRD declares
	// with its initiative item: the repository holds that initiative's
	// epics and features, which attach under it (GH-2811).
	Initiative string `json:"initiative,omitempty"`
}

// SourceSibling names a team read from a checkout beside this one, as
// every reader did before the registry (GH-89).
const SourceSibling = "sibling"

// SiblingSources are the checkouts beside dir.
func SiblingSources(dir string) []TeamSource {
	var out []TeamSource
	for _, sib := range checkout.Siblings(dir) {
		out = append(out, TeamSource{Name: filepath.Base(sib), Dir: sib, Source: SourceSibling})
	}
	return out
}

// teamFile is a file a team repository's roll-up reads: its hierarchy
// documents, its ARCHITECTURE, and its sprint-end summaries.
func teamFile(p string) bool {
	return strings.HasSuffix(p, ".md") && (strings.HasPrefix(p, "docs/specs/") || p == "docs/ARCHITECTURE.md" || strings.HasPrefix(p, "docs/sprint-ends/"))
}

// RegistrySources reads every team repository the system repository's
// subsystem SRDs name, the program's registry, into a directory under
// scratch each: from its host with no checkout where the tracking system
// serves files, else from a checkout beside this one. A repository it
// cannot read is in unread, never skipped silently (srd045 R3.4; GH-2726).
// With no registry, it reads none and says so with ok false.
func RegistrySources(t tracker.Tracker, system, ref, scratch string) (sources []TeamSource, unread []repofiles.Unread, ok bool, err error) {
	if t == nil || system == "" {
		return nil, nil, false, nil
	}
	teams, err := program.Teams(t, system, ref)
	if err != nil || len(teams) == 0 {
		return nil, nil, false, err
	}
	// A team that names a scrum repository plans there alone; one that
	// does not plans in its component repositories (GH-2738).
	var repos []string
	urls, initiatives := map[string]string{}, map[string]string{}
	for _, sub := range teams {
		for _, r := range sub.PlanningRepositories() {
			if r != "" && !slices.Contains(repos, r) {
				repos = append(repos, r)
				urls[r] = repositoryURL(sub, r)
				initiatives[r] = sub.Initiative
			}
		}
	}
	group := path.Dir(program.ProjectPath(system))
	for _, repo := range repos {
		files, reason := repofiles.Read(t, repo, "docs", teamFile)
		if reason != "" {
			unread = append(unread, repofiles.Unread{Repository: repo, Reason: reason})
			continue
		}
		dir, err := repofiles.Materialize(scratch, files)
		if err != nil {
			return sources, unread, true, err
		}
		// A copy carries its repository as its origin, so a reader that
		// links a document to its forge from its checkout's origin links
		// this one to the team's repository (GH-2726).
		if err := exec.Command("git", "init", "-q", dir).Run(); err == nil {
			exec.Command("git", "-C", dir, "remote", "add", "origin", urls[repo]).Run()
		}
		src := repofiles.SourceCheckout
		if len(files) > 0 {
			src = files[0].Source
		}
		sources = append(sources, TeamSource{Name: program.RepositoryName(group, repo), Repository: repo, Dir: dir, Source: src, Initiative: initiatives[repo]})
	}
	return sources, unread, true, nil
}

// repositoryURL is how the SRD writes one of a team's repositories, so a
// copy's origin links the forge as the SRD does.
func repositoryURL(sub program.Subsystem, repo string) string {
	if sub.Scrum != "" && program.ProjectPath(sub.Scrum) == repo {
		return sub.Scrum
	}
	for _, c := range sub.Components {
		if program.ProjectPath(c.Repository) == repo {
			return c.Repository
		}
	}
	return repo
}

// Sources are the team repositories whose documents roll up to the
// system checkout at dir: those the registry names, read through t, or,
// with no tracker or no registry, the checkouts beside dir.
func Sources(t tracker.Tracker, dir, ref, scratch string) ([]TeamSource, []repofiles.Unread, error) {
	sources, unread, ok, err := RegistrySources(t, program.SystemControl(dir), ref, scratch)
	if err != nil || ok {
		return sources, unread, err
	}
	return SiblingSources(dir), nil, nil
}
