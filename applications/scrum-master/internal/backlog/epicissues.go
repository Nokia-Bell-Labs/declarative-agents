// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/people"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// EpicIssue is what the reconcile did for one epic.
type EpicIssue struct {
	Epic   string   `json:"epic"`
	Issue  string   `json:"issue,omitempty"`
	People []string `json:"people,omitempty"`
	// Action is filed, updated, closed, unchanged, or none, for an epic with no
	// issue that needs none.
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
}

// EpicIssuesResult is what scrum-master backlog epic-issues did.
type EpicIssuesResult struct {
	Project string      `json:"project"`
	Epics   []EpicIssue `json:"epics"`
	Commit  string      `json:"commit,omitempty"`
	DryRun  bool        `json:"dry_run,omitempty"`
}

// epicMarker ties an issue to its epic, so a run that filed one and did not get to
// write the document finds it again rather than filing another.
func epicMarker(id string) string { return "<!-- scrum-master epic: " + id + " -->" }

// epicBody is an open epic's issue: where the document is, who is on it, and that
// the document is what counts.
func epicBody(d *hierarchy.Doc, rel string, on []string) string {
	who := "No one is named on it yet."
	if len(on) > 0 {
		who = "People on it: " + strings.Join(on, ", ") + "."
	}
	return fmt.Sprintf("This issue mirrors the epic %s, `%s`, so people can follow it here. The document is what counts: its scope and progress are not edited on this issue, and it is never popped.\n\n%s\n\n%s\n", d.Title, rel, who, epicMarker(d.ID))
}

// epicsDone says which epics have all their story points done. A team's stories
// count toward an epic they serve, so with a team's checkout missing it says
// nothing and every epic stays open, rather than closing one on half its stories.
func epicsDone(root string, docs []*hierarchy.Doc, system string) map[string]bool {
	done, err := sprintgoal.StoriesDone(root)
	if err != nil {
		return nil
	}
	set := map[string]bool{}
	for id := range done.Release {
		set[id] = true
	}
	progress, _ := hierarchy.RollUp(docs, set)
	points := map[string]hierarchy.Progress{}
	for _, p := range progress {
		points[p.ID] = p
	}
	if system != "" {
		subs, err := program.SubsystemsIn(system)
		if err != nil {
			return nil
		}
		for _, sub := range subs.Teams(program.SystemControl(system)) {
			if _, ok := checkout.Locate(system, sub.ScrumRepository()); !ok {
				return nil
			}
		}
		teams, _ := TeamProgress(SiblingSources(root), docs)
		for id, p := range teams {
			sum := points[id]
			sum.Done += p.Done
			sum.Total += p.Total
			points[id] = sum
		}
	}
	out := map[string]bool{}
	for id, p := range points {
		out[id] = p.Total > 0 && p.Done >= p.Total
	}
	return out
}

// withEpicIssue is the document with its issue's two context items, after its
// owner.
func withEpicIssue(doc []byte, ref, url string) []byte {
	lines := strings.SplitAfter(string(doc), "\n")
	for n, l := range lines {
		if strings.HasPrefix(l, "- owner:") {
			items := "- issue: " + ref + "\n"
			if url != "" {
				items += "- issue_url: " + url + "\n"
			}
			return []byte(strings.Join(lines[:n+1], "") + items + strings.Join(lines[n+1:], ""))
		}
	}
	return doc
}

// ReconcileEpicIssues gives each open epic in the checkout at dir one issue in the
// repository holding it, as a mirror (srd045 R2.7): filed type::tracking with the
// document recording it, its body and assignees following control/people.md, and
// closed when the epic is done or retired. A second run changes nothing. The people
// are its assignees where the tracking system holds several; beads names them in
// the body alone.
func ReconcileEpicIssues(t tracker.Tracker, dir string, commit, dryRun bool) (EpicIssuesResult, error) {
	r := EpicIssuesResult{Epics: []EpicIssue{}, DryRun: dryRun}
	root, err := toplevelOf(dir)
	if err != nil {
		return r, err
	}
	r.Project = program.ProjectPath(checkout.Origin(root))
	if r.Project == "" {
		return r, fmt.Errorf("%s has no origin naming its project, so its epics' issues cannot be found", root)
	}
	docs, err := hierarchy.Load(root)
	if err != nil {
		return r, err
	}
	var c people.Config
	name := func(id string) string { return id }
	system := ""
	if trackers, err := checkout.FindTrackers(root); err == nil && trackers != "" {
		system = filepath.Dir(filepath.Dir(trackers))
		if c, err = people.Load(system); err != nil {
			return r, err
		}
		if !sameDir(system, root) {
			group := path.Dir(program.ProjectPath(checkout.Origin(system)))
			team := program.RepositoryName(group, r.Project)
			name = func(id string) string { return team + "/" + id }
			system = ""
		}
	}
	done := epicsDone(root, docs, system)
	existing, err := t.ListIssues(r.Project, tracker.IssueQuery{State: "all", Labels: []string{"type::tracking"}})
	if err != nil {
		return r, err
	}
	var changed []string
	for _, d := range docs {
		if d.Kind != hierarchy.Epic {
			continue
		}
		rel, err := filepath.Rel(root, d.Path)
		if err != nil {
			return r, err
		}
		rel = filepath.ToSlash(rel)
		var on []string
		for _, p := range c.People {
			if slices.Contains(p.Epics, name(d.ID)) {
				on = append(on, p.Username)
			}
		}
		e := EpicIssue{Epic: d.ID, Issue: d.Issue, People: on, Action: "none"}
		open := d.Status == "agreed" && !done[d.ID]
		iid := 0
		if d.Issue != "" {
			if _, n, ok := strings.Cut(d.Issue, "#"); ok {
				iid, _ = strconv.Atoi(n)
			}
			if iid == 0 {
				return r, fmt.Errorf("%s: its issue item %q is not project#iid", rel, d.Issue)
			}
		} else if i := slices.IndexFunc(existing, func(i tracker.Issue) bool { return strings.Contains(i.Description, epicMarker(d.ID)) }); i >= 0 {
			// A run filed it and stopped before writing the document.
			iid = existing[i].IID
			e.Issue = fmt.Sprintf("%s#%d", r.Project, iid)
			if !dryRun {
				if err := recordEpicIssue(d.Path, e.Issue, existing[i].WebURL); err != nil {
					return r, err
				}
				changed = append(changed, rel)
			}
			e.Note = "found the issue an earlier run filed"
		} else if open {
			e.Action = "filed"
			if !dryRun {
				i, err := t.CreateIssue(r.Project, tracker.NewIssue{Title: "Epic: " + d.Title, Description: epicBody(d, rel, on), Labels: program.Labels("management", "tracking", "", "")})
				if err != nil {
					return r, fmt.Errorf("%s: %w", d.ID, err)
				}
				iid = i.IID
				e.Issue = fmt.Sprintf("%s#%d", r.Project, iid)
				if err := recordEpicIssue(d.Path, e.Issue, i.WebURL); err != nil {
					return r, err
				}
				changed = append(changed, rel)
				e.Note = setEpicAssignees(t, r.Project, iid, on)
			}
			r.Epics = append(r.Epics, e)
			continue
		}
		if iid == 0 {
			r.Epics = append(r.Epics, e)
			continue
		}
		i, err := t.GetIssue(r.Project, iid)
		if err != nil {
			return r, fmt.Errorf("%s: %w", d.ID, err)
		}
		switch {
		case !open && i.State != "closed":
			e.Action = "closed"
			if !dryRun {
				state := "close"
				if err := t.CreateNote(r.Project, iid, fmt.Sprintf("The epic %s is %s, so this mirror closes.", d.ID, map[bool]string{true: "retired", false: "done"}[d.Status == "retired"])); err != nil {
					return r, err
				}
				if _, err := t.UpdateIssue(r.Project, iid, tracker.IssueUpdate{State: &state}); err != nil {
					return r, err
				}
			}
		case !open:
			e.Action = "unchanged"
		default:
			e.Action = "unchanged"
			body := epicBody(d, rel, on)
			_, several := tracker.For(t, r.Project).(tracker.MultiAssigner)
			differs := several && !slices.Equal(sorted(i.Assignees), sorted(on))
			// A forge may trim a body, as GitLab does its last newline.
			rewrite := strings.TrimSpace(i.Description) != strings.TrimSpace(body)
			if rewrite || differs {
				e.Action = "updated"
				if !dryRun {
					if rewrite {
						if _, err := t.UpdateIssue(r.Project, iid, tracker.IssueUpdate{Description: &body}); err != nil {
							return r, err
						}
					}
					if differs {
						e.Note = setEpicAssignees(t, r.Project, iid, on)
					}
				}
			}
		}
		r.Epics = append(r.Epics, e)
	}
	if commit && !dryRun && len(changed) > 0 {
		args := append([]string{"-C", root, "add", "--"}, changed...)
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			return r, fmt.Errorf("add the epics: %v: %s", err, bytes.TrimSpace(out))
		}
		msg := "The epics record their issues (scrum-master backlog epic-issues)"
		if out, err := exec.Command("git", "-C", root, "commit", "-q", "-m", msg).CombinedOutput(); err != nil {
			return r, fmt.Errorf("commit the epics: %v: %s", err, bytes.TrimSpace(out))
		}
		sha, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
		if err != nil {
			return r, err
		}
		r.Commit = strings.TrimSpace(string(sha))
	}
	return r, nil
}

func sorted(names []string) []string {
	out := slices.Clone(names)
	slices.Sort(out)
	return out
}

// setEpicAssignees makes the people the issue's assignees where the tracking system
// holds several; what stops it is a note, since the body names them anyway.
func setEpicAssignees(t tracker.Tracker, project string, iid int, on []string) string {
	m, ok := tracker.For(t, project).(tracker.MultiAssigner)
	if !ok {
		return ""
	}
	if err := m.SetAssignees(project, iid, on); err != nil {
		return "could not assign " + strings.Join(on, ", ") + ": " + err.Error()
	}
	return ""
}

func recordEpicIssue(file, ref, url string) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if strings.HasPrefix(url, "beads:") {
		url = ""
	}
	next := withEpicIssue(b, ref, url)
	if bytes.Equal(next, b) {
		return fmt.Errorf("%s has no owner item to record its issue after", file)
	}
	return os.WriteFile(file, next, 0o644)
}

func init() {
	var a struct {
		dir    string
		commit bool
	}
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "epic-issues", Summary: "give each open epic one issue mirroring it, and close it when the epic is done or retired",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&a.dir, "dir", ".", "a checkout of the repository holding the epics")
			fs.BoolVar(&a.commit, "commit", false, "commit the epics that record an issue on the current branch")
		},
		Run: func(ctx *cli.Context) error {
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			r, err := ReconcileEpicIssues(t, a.dir, a.commit, ctx.DryRun)
			if err != nil {
				return err
			}
			var h strings.Builder
			for _, e := range r.Epics {
				action := e.Action
				if r.DryRun && action != "unchanged" && action != "none" {
					action = "would be " + action
				}
				fmt.Fprintf(&h, "%-10s %s %s\n", action, e.Epic, e.Issue)
				if e.Note != "" {
					fmt.Fprintf(&h, "           %s\n", e.Note)
				}
			}
			if len(r.Epics) == 0 {
				fmt.Fprintf(&h, "%s holds no epics\n", r.Project)
			}
			return ctx.Emit(r, h.String())
		},
	})
}
