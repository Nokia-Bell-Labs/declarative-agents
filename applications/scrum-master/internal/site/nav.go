// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// Sidebar is the site's navigator, which the app shell loads on every page
// (GH-127).
const Sidebar = Dir + "/_sidebar.md"

// Repo is one of the program's repositories: its name and its page on the
// forge, empty when its origin is none a browser opens.
type Repo struct{ Name, URL string }

// Repos are the program's repositories with a checkout beside the reports
// checkout: the system's first, then each sibling the system declares as a
// component repository, then the reports repository, each named as its
// origin names it, else by its main checkout's folder. A sibling the program
// does not declare is someone else's and is left out (GH-141).
func Repos(system, reports string) []Repo {
	// The checkouts compare as absolute paths: the reports checkout is
	// often ".", which no sibling's path equals as written.
	if abs, err := filepath.Abs(reports); err == nil {
		reports = abs
	}
	declared, _ := Projects(system)
	dirs := []string{system}
	for _, sib := range checkout.Siblings(system) {
		if containsDir([]string{reports}, sib) || declares(declared, sib) {
			dirs = append(dirs, sib)
		}
	}
	if !containsDir(dirs, reports) {
		dirs = append(dirs, reports)
	}
	var out []Repo
	for _, d := range dirs {
		if exec.Command("git", "-C", d, "rev-parse", "--git-dir").Run() != nil {
			continue
		}
		name := program.ProjectPath(originOf(d))
		if name == "" || strings.HasPrefix(originOf(d), "/") {
			name = filepath.Base(mainCheckout(d))
		}
		out = append(out, Repo{Name: name, URL: originWeb(d)})
	}
	return out
}

// declares reports whether the program declares the checkout's repository:
// its origin's project path is a declared one, or, for an origin that is a
// local path or none, its repository's name is a declared one's.
func declares(projects []string, dir string) bool {
	origin := originOf(dir)
	if p := program.ProjectPath(origin); p != "" && !strings.HasPrefix(origin, "/") {
		return slices.Contains(projects, p)
	}
	name := filepath.Base(mainCheckout(dir))
	if origin != "" {
		name = strings.TrimSuffix(filepath.Base(origin), ".git")
	}
	for _, p := range projects {
		if p != "" && path.Base(p) == name {
			return true
		}
	}
	return false
}

func originOf(dir string) string {
	b, _ := exec.Command("git", "-C", dir, "config", "--get", "remote.origin.url").Output()
	return strings.TrimSpace(string(b))
}

// mainCheckout is the main checkout of dir's repository, dir itself when it
// is one or git cannot say: a worktree is named by the repository it is of.
func mainCheckout(dir string) string {
	b, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return dir
	}
	common := strings.TrimSpace(string(b))
	if filepath.Base(common) != ".git" {
		return dir
	}
	return filepath.Dir(common)
}

func containsDir(dirs []string, d string) bool {
	canon := func(p string) string {
		abs, _ := filepath.Abs(p)
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			return real
		}
		return abs
	}
	want := canon(d)
	for _, x := range dirs {
		if canon(x) == want {
			return true
		}
	}
	return false
}

// sidebarPage is the navigator: the site's own pages, the timeline and its
// sprints, the plan's tree, every theme to every story, so search reaches
// each, the program's scrums, and its repositories on the forge. Links are
// site-rooted, the routes the shell serves.
func sidebarPage(nodes map[string]*Node, repos []Repo, sprints []sprint, teams []timeline.Team, parent *timeline.ProgramRef) string {
	var b strings.Builder
	b.WriteString(generated("hierarchy", "the timeline facts and the checkouts beside the reports checkout"))
	// A program that is a team of a larger one says so first, linking the
	// parent's site (GH-2740).
	if parent != nil {
		if parent.Site != "" {
			fmt.Fprintf(&b, "- [%sPart of %s](%s)\n", `<span class="kb kb-program" title="Program">P</span> `, parent.Name, parent.Site)
		} else {
			fmt.Fprintf(&b, "- %sPart of %s\n", `<span class="kb kb-program" title="Program">P</span> `, parent.Name)
		}
	}
	b.WriteString("- [Overview](/overview.md)\n- [About](/index.md)\n")
	b.WriteString("- Program\n  - [Status](/status/index.md)\n  - [Road map](/status/road-map.md)\n  - [Requirement history](/history/index.md)\n  - [Corpus](/corpus/index.md)\n")
	// The timeline opens into the program's sprints, each with its page
	// (GH-139).
	b.WriteString("- [Timeline](/timeline/index.md)\n")
	if len(sprints) > 0 {
		b.WriteString("  - [Sprints](/" + SprintsDir + "/index.md)\n")
		for _, sp := range sprints {
			fmt.Fprintf(&b, "    - [%s](/%s)\n", sp.Title, sp.Page)
		}
	}
	b.WriteString("- [Plan](/" + HierarchyDir + "/index.md)\n")
	var walk func(id string, depth int)
	walk = func(id string, depth int) {
		n := nodes[id]
		// The link's title is the document's own: the shell titles the
		// browser's tab with it rather than with the chip's letter.
		fmt.Fprintf(&b, "%s- [%s%s](/%s %q)\n", strings.Repeat("  ", depth), navBadge(n.Kind), n.Title, n.Page, n.Title)
		for _, c := range n.Children {
			walk(c, depth+1)
		}
	}
	for _, id := range sortedNodes(nodes) {
		if _, ok := nodes[nodes[id].Parent]; !ok {
			walk(id, 1)
		}
	}
	if len(teams) > 0 {
		b.WriteString("- [Scrums](/" + ScrumsDir + "/index.md)\n")
		for _, t := range teams {
			chip := `<span class="kb kb-scrum" title="Scrum">S</span> `
			if t.Program {
				chip = `<span class="kb kb-program" title="Program">P</span> `
			}
			fmt.Fprintf(&b, "  - [%s%s](/%s/%s.md %q)\n", chip, t.Name, ScrumsDir, t.Name, t.Name)
		}
	}
	if len(repos) > 0 {
		b.WriteString("- Repositories\n")
		for _, r := range repos {
			if r.URL != "" {
				fmt.Fprintf(&b, "  - [%s](%s)\n", r.Name, r.URL)
			} else {
				fmt.Fprintf(&b, "  - %s\n", r.Name)
			}
		}
	}
	return b.String()
}
