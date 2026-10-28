// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
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
	for _, repo := range registered(subs.Teams(system), system) {
		r.Repositories = append(r.Repositories, oneCheckout(r.Root, root, system, repo, cloneURL(origin, system, repo), dryRun))
	}
	return r, nil
}

// registeredRepo is one registry entry: the repository's project path and
// how its SRD writes it.
type registeredRepo struct{ project, written string }

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
	var dir string
	cli.Default.Register(&cli.Command{
		Group:   "checkouts",
		Summary: "clone every registered repository at its group path beside the system checkout, and fast-forward the ones already there (GH-2812)",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&dir, "dir", "", "the system repository's checkout (default: the git root)")
		},
		Run: func(ctx *cli.Context) error {
			d := dir
			if d == "" {
				var err error
				if d, err = gitRoot(); err != nil {
					return err
				}
			}
			r, err := Checkouts(d, ctx.DryRun)
			if err != nil {
				return err
			}
			return ctx.Emit(r, r.Human())
		},
	})
}
