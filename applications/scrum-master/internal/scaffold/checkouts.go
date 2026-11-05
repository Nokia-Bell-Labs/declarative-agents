// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
)

// CheckoutEntry is one registered repository the checkouts word handled:
// cloned at its group path, updated in place, kept as it is with the
// reason, or failed with the reason (GH-2812).
type CheckoutEntry struct {
	Project string `json:"project"`
	Action  string `json:"action"` // cloned, updated, kept, failed
	Path    string `json:"path,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// CheckoutsResult is the checkouts word's report over the registry.
type CheckoutsResult struct {
	Root         string          `json:"root"`
	Repositories []CheckoutEntry `json:"repositories"`
	DryRun       bool            `json:"dry_run"`
}

// Checkouts lays the program's checkouts down from the system checkout at
// dir (GH-2812): it reads the registry, the subsystem SRDs, and clones
// each registered repository that has no checkout at its group path under
// the folder whose layout the system checkout mirrors, the level Locate
// searches (GH-2803). One already checked out is fetched and
// fast-forwarded on its default branch; one on another branch, or holding
// changes git status shows, is kept with the reason. A repository that
// cannot be reached fails alone, and the run goes on. A dry run reports
// what it would do and touches nothing.
func Checkouts(dir string, dryRun bool) (CheckoutsResult, error) {
	r := CheckoutsResult{DryRun: dryRun}
	root, err := toplevel(dir)
	if err != nil {
		return r, fmt.Errorf("checkouts needs the system checkout: %w", err)
	}
	origin := checkout.Origin(root)
	if !checkout.IsForge(origin) {
		return r, fmt.Errorf("%s has no forge origin to derive the registered repositories' URLs from", root)
	}
	system := checkout.ProjectPath(origin)
	subs, err := program.SubsystemsIn(root)
	if err != nil {
		return r, err
	}
	if len(subs) == 0 {
		return r, fmt.Errorf("the registry names no repositories: %s holds no subsystem SRD under %s", root, program.SRDDir)
	}
	r.Root, r.Repositories = registryRoot(root, system), []CheckoutEntry{}
	repos := registered(subs.Teams(system), system)
	if site, ok := siteRepository(root, repos); ok {
		repos = append(repos, site)
	}
	for _, repo := range repos {
		r.Repositories = append(r.Repositories, oneCheckout(r.Root, root, system, repo, cloneURL(origin, system, repo), dryRun))
	}
	return r, nil
}

// FindSystem is the system repository's checkout for the clone at dir, cloned beside
// it when there is none (srd043 R6.4): dir's own, the one beside it, or the project
// system names, else the one DiscoverSystem finds through the forge. A clone of any
// of a program's repositories is enough to lay the program down.
func FindSystem(dir, system string, discover func(dir string) ([]string, error), dryRun bool) (string, *CheckoutEntry, error) {
	root, err := toplevel(dir)
	if err != nil {
		return "", nil, fmt.Errorf("checkouts needs a checkout of one of the program's repositories: %w", err)
	}
	if file, err := checkout.FindTrackers(root); err == nil && file != "" {
		return filepath.Dir(filepath.Dir(file)), nil, nil
	}
	origin := checkout.Origin(root)
	if !checkout.IsForge(origin) {
		return "", nil, fmt.Errorf("%s has no forge origin to find its program through; check the system repository out beside it", root)
	}
	if system == "" {
		found, err := discover(root)
		if err != nil {
			return "", nil, err
		}
		switch len(found) {
		case 0:
			return "", nil, fmt.Errorf("no repository in the groups of %s holds %s; name the system repository with --system or SCRUM_MASTER_SYSTEM", checkout.ProjectPath(origin), checkout.TrackersFile)
		case 1:
			system = found[0]
		default:
			return "", nil, fmt.Errorf("more than one repository holds %s: %s; name the program's with --system", checkout.TrackersFile, strings.Join(found, ", "))
		}
	}
	project := checkout.ProjectPath(system)
	dest := filepath.Join(filepath.Dir(root), path.Base(project))
	url := strings.Replace(origin, checkout.ProjectPath(origin), project, 1)
	e := &CheckoutEntry{Project: project, Action: "cloned", Path: dest}
	if dryRun {
		e.Detail = "would clone " + url
		return "", e, nil
	}
	if _, err := os.Stat(dest); err == nil {
		return "", nil, fmt.Errorf("%s exists and is not a checkout of %s", dest, project)
	}
	if out, err := exec.Command("git", "clone", "-q", url, dest).CombinedOutput(); err != nil {
		return "", nil, fmt.Errorf("clone %s: %v: %s", url, err, strings.TrimSpace(string(out)))
	}
	if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(checkout.TrackersFile))); err != nil {
		return "", nil, fmt.Errorf("%s holds no %s, so it is no system repository", project, checkout.TrackersFile)
	}
	return dest, e, nil
}

// registeredRepo is one registry entry: the repository's project path and
// how its SRD writes it.
type registeredRepo struct{ project, written string }

// siteRepository is the repository the system checkout at root names as its
// site's home in control/program.md, when it does and repos does not already
// hold it: the one declaration of a program's reports repository, which the
// registry does not list.
func siteRepository(root string, repos []registeredRepo) (registeredRepo, bool) {
	l, err := program.LineageIn(root)
	if err != nil || l.SiteRepository == "" {
		return registeredRepo{}, false
	}
	p := checkout.ProjectPath(l.SiteRepository)
	if slices.ContainsFunc(repos, func(x registeredRepo) bool { return x.project == p }) {
		return registeredRepo{}, false
	}
	return registeredRepo{project: p, written: l.SiteRepository}, true
}

// registered lists every repository the subsystem SRDs name, each once,
// the system repository left out: each team's scrum repository and every
// component row (GH-2812).
func registered(teams program.Subsystems, system string) []registeredRepo {
	var out []registeredRepo
	seen := []string{checkout.ProjectPath(system)}
	add := func(written string) {
		p := checkout.ProjectPath(written)
		if p != "" && !slices.Contains(seen, p) {
			seen = append(seen, p)
			out = append(out, registeredRepo{project: p, written: written})
		}
	}
	for _, sub := range teams {
		if sub.Scrum != "" {
			add(sub.Scrum)
		}
		for _, c := range sub.Components {
			add(c.Repository)
		}
	}
	return out
}

// registryRoot is the folder the program's checkouts are laid down under:
// the one the system checkout's own path mirrors, found by stripping the
// longest trailing part of its project path from its path; the folder
// holding it when its folder matches none, as for a renamed checkout.
func registryRoot(root, system string) string {
	parts := strings.Split(checkout.ProjectPath(system), "/")
	for i := range parts {
		tail := filepath.Join(parts[i:]...)
		if rest, ok := strings.CutSuffix(root, string(filepath.Separator)+tail); ok {
			return rest
		}
	}
	return filepath.Dir(root)
}

// destination is where a registered repository is cloned: its project path
// below the registry root, trimmed of the leading group segments the
// system checkout's own path does not carry, so a program mirrored from
// its group down clones below the group, and one checked out side by side
// clones beside the system checkout.
func destination(registryRoot, root, system, project string) string {
	carried := strings.Trim(strings.TrimPrefix(root, registryRoot), string(filepath.Separator))
	sysParts := strings.Split(checkout.ProjectPath(system), "/")
	p := checkout.ProjectPath(project)
	if dropped := len(sysParts) - len(strings.Split(filepath.ToSlash(carried), "/")); dropped > 0 {
		if prefix := strings.Join(sysParts[:dropped], "/") + "/"; strings.HasPrefix(p, prefix) {
			p = strings.TrimPrefix(p, prefix)
		}
	}
	return filepath.Join(registryRoot, filepath.FromSlash(p))
}

// cloneURL is where a registered repository is cloned from: as the SRD
// writes it when that is a URL a clone takes, else the system checkout's
// origin with the system's project path swapped for the repository's, so
// the clone uses the same scheme and host the person already uses.
func cloneURL(origin, system string, repo registeredRepo) string {
	w := repo.written
	if strings.Contains(w, "://") || strings.HasPrefix(w, "/") || strings.HasPrefix(w, ".") {
		return w
	}
	if host, p, ok := strings.Cut(w, ":"); ok && !strings.Contains(host, "/") && p != "" {
		return w // the scp form user@host:group/repo
	}
	if u := strings.Replace(origin, checkout.ProjectPath(system), repo.project, 1); u != origin {
		return u
	}
	return w
}

// oneCheckout clones, updates, or keeps one registered repository.
func oneCheckout(registryRoot, root, system string, repo registeredRepo, url string, dryRun bool) CheckoutEntry {
	e := CheckoutEntry{Project: repo.project}
	if dir, ok := checkout.Locate(root, repo.project); ok {
		return updateCheckout(e, dir, dryRun)
	}
	dest := destination(registryRoot, root, system, repo.project)
	e.Path = dest
	if _, err := os.Stat(dest); err == nil {
		e.Action, e.Detail = "failed", dest+" exists and is not a checkout of "+repo.project
		return e
	}
	if dryRun {
		e.Action, e.Detail = "cloned", "would clone "+url
		return e
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		e.Action, e.Detail = "failed", err.Error()
		return e
	}
	if out, err := exec.Command("git", "clone", "-q", url, dest).CombinedOutput(); err != nil {
		e.Action, e.Detail = "failed", strings.TrimSpace(string(out))
		if e.Detail == "" {
			e.Detail = err.Error()
		}
		return e
	}
	e.Action = "cloned"
	return e
}

// updateCheckout fetches and fast-forwards a checkout on its default
// branch; one on another branch, or holding changes, is kept with the
// reason.
func updateCheckout(e CheckoutEntry, dir string, dryRun bool) CheckoutEntry {
	e.Path = dir
	git := func(args ...string) (string, error) {
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	if status, err := git("status", "--porcelain"); err != nil || status != "" {
		e.Action, e.Detail = "kept", "holds changes git status shows"
		if err != nil {
			e.Detail = "git status failed: " + status
		}
		return e
	}
	def := "main"
	if head, err := git("symbolic-ref", "--short", "refs/remotes/origin/HEAD"); err == nil {
		def = strings.TrimPrefix(head, "origin/")
	}
	if cur, _ := git("branch", "--show-current"); cur != def {
		e.Action, e.Detail = "kept", "on branch "+cur
		return e
	}
	if dryRun {
		e.Action, e.Detail = "updated", "would fetch and fast-forward "+def
		return e
	}
	if out, err := git("fetch", "-q", "origin"); err != nil {
		e.Action, e.Detail = "failed", "fetch: "+out
		return e
	}
	count, _ := git("rev-list", "--count", "HEAD..origin/"+def)
	if out, err := git("merge", "--ff-only", "-q", "origin/"+def); err != nil {
		e.Action, e.Detail = "failed", "fast-forward: "+out
		return e
	}
	e.Action = "updated"
	if count == "" || count == "0" {
		e.Detail = "already up to date"
	} else {
		e.Detail = count + " commits fast-forwarded"
	}
	return e
}

// toplevel is the root of the checkout holding dir.
func toplevel(dir string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %s", dir, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Human is the checkouts word's text output: one line per repository and
// the counts.
func (r CheckoutsResult) Human() string {
	var b strings.Builder
	counts := map[string]int{}
	for _, e := range r.Repositories {
		counts[e.Action]++
		fmt.Fprintf(&b, "%-7s %s", e.Action, e.Project)
		switch {
		case e.Action == "cloned" && !r.DryRun:
			fmt.Fprintf(&b, "  -> %s", e.Path)
		case e.Detail != "":
			fmt.Fprintf(&b, "  (%s)", e.Detail)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "checkouts: %d cloned, %d updated, %d kept, %d failed", counts["cloned"], counts["updated"], counts["kept"], counts["failed"])
	if r.DryRun {
		b.WriteString(" (dry run)")
	}
	return b.String()
}

func init() {
	var dir, system string
	cli.Default.Register(&cli.Command{
		Group:   "checkouts",
		Summary: "clone every registered repository at its group path beside the system checkout, and fast-forward the ones already there; from a clone of any of them, find and clone the system repository first (GH-2812)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&dir, "dir", "", "a checkout of one of the program's repositories (default: the git root)")
			fs.StringVar(&system, "system", "", "the system repository's project path, when it cannot be found through the forge")
		},
		Run: func(ctx *cli.Context) error {
			d := dir
			if d == "" {
				var err error
				if d, err = gitRoot(); err != nil {
					return err
				}
			}
			named := namedSystem(system)
			found, cloned, err := FindSystem(d, named, routing.DiscoverSystem, ctx.DryRun)
			if err != nil {
				return err
			}
			if found == "" { // a dry run with the system repository still to clone
				r := CheckoutsResult{DryRun: true, Repositories: []CheckoutEntry{*cloned}}
				return ctx.Emit(r, r.Human())
			}
			if named == "" && routing.OnAccount(d) {
				if err := declaredBy(found, d, cloned == nil); err != nil {
					if cloned != nil {
						os.RemoveAll(found) //nolint:errcheck // The clone this run just made.
					}
					return err
				}
			}
			r, err := Checkouts(found, ctx.DryRun)
			if cloned != nil {
				r.Repositories = append([]CheckoutEntry{*cloned}, r.Repositories...)
			}
			if err != nil {
				return err
			}
			return ctx.Emit(r, r.Human())
		},
	})
}

// namedSystem is the system repository a run names: --system, else
// SCRUM_MASTER_SYSTEM, as the words that read the trackers document take it
// (GH-2904).
func namedSystem(flag string) string {
	if flag != "" {
		return flag
	}
	return strings.TrimSpace(os.Getenv("SCRUM_MASTER_SYSTEM")) //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
}

// declaredBy refuses a clone the program at system does not declare, where the
// forge found that program only by the account they share: a group is a whole
// account there, so an unrelated repository would otherwise lay the program down
// beside it (GH-2904). The system repository's own clone is declared, and naming
// the system repository says the person means it. So is the site repository
// control/program.md names, and a site repository with the system already checked
// out beside it, beside being how its README says to run the site (GH-2915).
func declaredBy(system, dir string, beside bool) error {
	root, err := toplevel(dir)
	if err != nil {
		return err
	}
	own, project := checkout.ProjectPath(checkout.Origin(root)), checkout.ProjectPath(checkout.Origin(system))
	if own == project {
		return nil
	}
	subs, err := program.SubsystemsIn(system)
	if err != nil {
		return err
	}
	repos := registered(subs.Teams(project), project)
	if site, ok := siteRepository(system, repos); ok {
		repos = append(repos, site)
	}
	if slices.ContainsFunc(repos, func(x registeredRepo) bool { return x.project == own }) {
		return nil
	}
	isSite := reports.IsSite(root)
	if isSite && beside {
		return nil
	}
	reason := fmt.Sprintf("%s does not declare %s, and this forge's group is a whole account, so finding it there says nothing about this clone; name the system repository with --system or SCRUM_MASTER_SYSTEM to lay it down anyway", project, own)
	if isSite {
		reason += fmt.Sprintf(", or name this repository as site_repository in %s's %s", project, program.LineageFile)
	}
	return errors.New(reason)
}
