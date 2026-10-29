// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// ReconcileEntry is one repository the reconcile swept: ok with nothing to
// fix, filed with the issue holding its findings, open when an issue
// already is, or failed with the reason (GH-2822).
type ReconcileEntry struct {
	Project  string   `json:"project"`
	Action   string   `json:"action"` // ok, filed, open, failed
	Issue    string   `json:"issue,omitempty"`
	Findings []string `json:"findings,omitempty"`
	Detail   string   `json:"detail,omitempty"`
}

// ReconcileResult is the reconcile word's report over the registry and the
// program's group.
type ReconcileResult struct {
	Repositories []ReconcileEntry `json:"repositories"`
	// Notes say what the sweep could not do, a tracking system that cannot
	// list the group's projects among them.
	Notes  []string `json:"notes,omitempty"`
	DryRun bool     `json:"dry_run"`
}

// Reconcile sweeps the program from the system checkout at dir (GH-2822):
// it reads the registry, the subsystem SRDs, and checks every registered
// repository against the forge and against what its documents declare —
// a URL the forge no longer serves or redirects, a checkout whose origin
// names another project, a parent or initiative pinned to what is not
// there, and the repository's doctor drift, run dry. Where the tracking
// system's host lists the group's projects, one the registry does not name
// is a finding too, since membership is declared, never inferred. Each
// repository with findings gets one issue in the system repository, never
// a second while one is open; the word changes no repository and never
// creates, moves, or deletes one on the forge — the infrastructure tooling
// owns that. A dry run reports every finding and files nothing.
func Reconcile(t tracker.Tracker, dir string, dryRun bool) (ReconcileResult, error) {
	r := ReconcileResult{DryRun: dryRun, Repositories: []ReconcileEntry{}}
	root, err := toplevel(dir)
	if err != nil {
		return r, fmt.Errorf("reconcile needs the system checkout: %w", err)
	}
	origin := checkout.Origin(root)
	if !checkout.IsForge(origin) {
		return r, fmt.Errorf("%s has no forge origin to name the registered repositories by", root)
	}
	system := checkout.ProjectPath(origin)
	subs, err := program.SubsystemsIn(root)
	if err != nil {
		return r, err
	}
	if len(subs) == 0 {
		return r, fmt.Errorf("the registry names no repositories: %s holds no subsystem SRD under %s", root, program.SRDDir)
	}
	policy, err := program.LoadPolicy(t, system, "main")
	if err != nil {
		return r, err
	}
	teams := subs.Teams(system)
	systemDocs := map[string]bool{}
	if docs, err := hierarchy.Load(root); err == nil {
		for _, d := range docs {
			systemDocs[d.ID] = true
		}
	}
	// The subsystem each repository belongs to, for its SRD and initiative.
	owner := map[string]program.Subsystem{}
	for _, sub := range teams {
		for _, p := range sub.Repositories() {
			owner[p] = sub
		}
	}
	repos := registered(teams, system)
	known := []string{program.ProjectPath(system)}
	for _, repo := range repos {
		known = append(known, repo.project)
	}
	open := openReconcileIssues(t, system)
	for _, repo := range repos {
		findings := reconcileRepository(root, system, repo, owner[repo.project], known, systemDocs)
		r.Repositories = append(r.Repositories, reconcileEntry(t, system, repo.project, findings, open, policy, dryRun))
	}
	// The group sweep: projects the registry does not name, where the host
	// lists them (design.md D7: membership is declared, not inferred).
	group := path.Dir(program.ProjectPath(system))
	if gl, ok := tracker.For(t, system).(tracker.GroupLister); ok && group != "" && group != "." {
		projects, err := gl.GroupProjects(group)
		if err != nil {
			r.Notes = append(r.Notes, fmt.Sprintf("%s: the group's projects are not listed, so unregistered repositories are not checked: %v", group, err))
		} else if findings := unregistered(projects, known); len(findings) > 0 {
			r.Repositories = append(r.Repositories, reconcileEntry(t, system, group, findings, open, policy, dryRun))
		}
	} else if !ok {
		r.Notes = append(r.Notes, "the tracking system does not list a group's projects, so unregistered repositories are not checked")
	}
	return r, nil
}

// reconcileRepository gathers one registered repository's findings, each a
// line naming the drift and where its fix lands.
func reconcileRepository(root, system string, repo registeredRepo, sub program.Subsystem, known []string, systemDocs map[string]bool) []string {
	var findings []string
	srd := sub.ID
	if srd == "" {
		srd = "its subsystem SRD"
	}
	// The forge: unreachable, or redirected as a moved GitLab project is.
	// A redirect to the project's own path, as normalizing a URL without
	// .git does, moves nothing.
	url := cloneURL(checkout.Origin(root), system, repo)
	if reason, moved := remoteState(root, url); moved != "" && moved != repo.project {
		findings = append(findings, fmt.Sprintf("the forge redirects %s to %s; update the row in %s", repo.project, moved, srd))
	} else if reason != "" {
		findings = append(findings, fmt.Sprintf("unreachable at %s (%s); update the row in %s, or restore the repository — the forge is the infrastructure tooling's", url, reason, srd))
	}
	dir, ok := checkout.Locate(root, repo.project)
	if !ok {
		// The lookup matches by origin, so a checkout sitting at the
		// project's group path with another origin is the sharper finding:
		// the registry and the folder disagree about what lives there.
		dest := destination(registryRoot(root, system), root, system, repo.project)
		if co := checkout.ProjectPath(checkout.Origin(dest)); co != "" && co != repo.project {
			findings = append(findings, fmt.Sprintf("the checkout at %s has origin %s, not the registered %s; update the row in %s, or point the checkout at the registered repository", dest, co, repo.project, srd))
			return findings
		}
		findings = append(findings, "no checkout at its group path; scrum-master checkouts lays it down (GH-2812)")
		return findings
	}
	if co := checkout.ProjectPath(checkout.Origin(dir)); co != "" && co != repo.project {
		findings = append(findings, fmt.Sprintf("the checkout at %s has origin %s, not the registered %s; update the row in %s, or point the checkout at the registered repository", dir, co, repo.project, srd))
	}
	// The initiative item names a document the plan holds (srd048 R8.6).
	if sub.Initiative != "" && !systemDocs[sub.Initiative] {
		findings = append(findings, fmt.Sprintf("%s's initiative item names %s, which the system repository does not hold; fix the item or add the document", srd, sub.Initiative))
	}
	findings = append(findings, pinFindings(root, dir, known)...)
	// What doctor would repair or report, run dry (srd043 R4).
	if d, err := Doctor(DoctorOptions{Dir: dir, Version: install.Version(), DryRun: true}); err == nil {
		for _, line := range d.Drift() {
			findings = append(findings, "doctor: "+line+"; run the upgrade profile there")
		}
	}
	return findings
}

// pinFindings checks a checkout's documents' pins: a parent pinned by URL
// to a project the registry does not name, or to a document the pinned
// repository's checkout does not hold (GH-2738, GH-2811).
func pinFindings(root, dir string, known []string) []string {
	var findings []string
	docs, err := hierarchy.Load(dir)
	if err != nil {
		return findings
	}
	for _, d := range docs {
		if !strings.Contains(d.Parent, "://") {
			continue
		}
		ref, err := links.Parse(d.Parent)
		if err != nil {
			continue
		}
		if !slices.Contains(known, ref.Project) {
			findings = append(findings, fmt.Sprintf("%s's parent pins %s, which the registry does not name; register the repository, or repin the parent", d.ID, ref.Project))
			continue
		}
		target, ok := checkout.Locate(root, ref.Project)
		if !ok {
			continue // the missing checkout is its own finding
		}
		if _, err := os.Stat(filepath.Join(target, filepath.FromSlash(ref.Path))); err != nil {
			findings = append(findings, fmt.Sprintf("%s's parent pins %s, which %s's checkout does not hold; fix the pin or add the document", d.ID, ref.Path, ref.Project))
		}
	}
	return findings
}

// unregistered is the group-sweep finding: each project under the group
// the registry does not name.
func unregistered(projects, known []string) []string {
	var findings []string
	for _, p := range projects {
		if !slices.Contains(known, checkout.ProjectPath(p)) {
			findings = append(findings, fmt.Sprintf("%s is in the group but the registry does not name it; onboard it where it joins the program — one that is not the program's, the reports repository among them, needs no row", p))
		}
	}
	return findings
}

// redirectPattern is git's warning when an https remote redirects, as a
// moved or renamed GitLab project does.
var redirectPattern = regexp.MustCompile(`redirecting to (\S+)`)

// remoteState asks the forge about one repository's URL: reason is why it
// cannot be reached, empty when it can; moved is the project path a
// redirect names, empty without one.
func remoteState(root, url string) (reason, moved string) {
	cmd := exec.Command("git", "-C", root, "ls-remote", "--heads", url)
	out, err := cmd.CombinedOutput()
	if m := redirectPattern.FindSubmatch(out); m != nil {
		return "", checkout.ProjectPath(strings.TrimSuffix(string(m[1]), "/info/refs"))
	}
	if err != nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return strings.TrimSpace(lines[len(lines)-1]), ""
	}
	return "", ""
}

// reconcileMarker names the repository an issue reconciles, so a rerun
// finds it rather than filing a second.
func reconcileMarker(project string) string {
	return "<!-- scrum-master reconcile " + project + " -->"
}

// openReconcileIssues maps each project with an open reconcile issue to it.
func openReconcileIssues(t tracker.Tracker, system string) map[string]string {
	out := map[string]string{}
	issues, err := t.ListIssues(system, tracker.IssueQuery{State: "opened"})
	if err != nil {
		return out
	}
	for _, i := range issues {
		if m := regexp.MustCompile(`<!-- scrum-master reconcile (\S+) -->`).FindStringSubmatch(i.Description); m != nil {
			out[m[1]] = fmt.Sprintf("%s#%d", system, i.IID)
		}
	}
	return out
}

// reconcileEntry reports one repository: ok without findings, the open
// issue when one is, the filed issue otherwise, and what a dry run would
// file.
func reconcileEntry(t tracker.Tracker, system, project string, findings []string, open map[string]string, policy program.Policy, dryRun bool) ReconcileEntry {
	e := ReconcileEntry{Project: project, Findings: findings}
	if len(findings) == 0 {
		e.Action = "ok"
		return e
	}
	if issue, ok := open[project]; ok {
		e.Action, e.Issue, e.Detail = "open", issue, "already filed"
		return e
	}
	if dryRun {
		e.Action, e.Detail = "filed", fmt.Sprintf("would file %d finding(s)", len(findings))
		return e
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The program's reconcile found drift between the registry, the forge, and %s (GH-2822). The forge itself is the infrastructure tooling's: nothing here creates, moves, or deletes a repository.\n\n## Requirements\n\n", project)
	for i, f := range findings {
		fmt.Fprintf(&b, "%d. %s.\n", i+1, strings.TrimSuffix(f, "."))
	}
	fmt.Fprintf(&b, "\n## Acceptance Criteria\n\n- Every requirement above is resolved, and scrum-master reconcile reports %s ok (test: scrum-master reconcile --dry-run; environment: local)\n\n%s\n", project, reconcileMarker(project))
	in := tracker.NewIssue{
		Title:       "Reconcile " + project,
		Description: b.String(),
		Labels:      append(program.Labels("management", "", "", ""), program.Op(program.OpReconcile)),
	}
	res, err := program.File(t, system, in, policy, false)
	if err != nil {
		e.Action, e.Detail = "failed", err.Error()
		return e
	}
	e.Action, e.Issue = "filed", fmt.Sprintf("%s#%d", system, res.IID)
	return e
}

// Human is the reconcile word's text output: one line per repository, the
// notes, and the counts.
func (r ReconcileResult) Human() string {
	var b strings.Builder
	counts := map[string]int{}
	for _, e := range r.Repositories {
		counts[e.Action]++
		fmt.Fprintf(&b, "%-7s %s", e.Action, e.Project)
		switch {
		case e.Issue != "":
			fmt.Fprintf(&b, "  %s (%d finding(s))", e.Issue, len(e.Findings))
		case e.Detail != "":
			fmt.Fprintf(&b, "  (%s)", e.Detail)
		}
		b.WriteString("\n")
		for _, f := range e.Findings {
			fmt.Fprintf(&b, "        - %s\n", f)
		}
	}
	for _, n := range r.Notes {
		b.WriteString("note: " + n + "\n")
	}
	fmt.Fprintf(&b, "reconcile: %d ok, %d filed, %d open, %d failed", counts["ok"], counts["filed"], counts["open"], counts["failed"])
	if r.DryRun {
		b.WriteString(" (dry run)")
	}
	return b.String()
}

func init() {
	var dir string
	cli.Default.Register(&cli.Command{
		Group:   "reconcile",
		Summary: "sweep the program for drift between the registry, the forge, and the checkouts, and file one issue per repository with findings (GH-2822)",
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
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := Reconcile(t, d, ctx.DryRun)
			if err != nil {
				return err
			}
			return ctx.Emit(r, r.Human())
		},
	})
}
