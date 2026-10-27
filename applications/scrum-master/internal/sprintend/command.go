// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package sprintend

import (
	"errors"
	"flag"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	var a struct{ project, reportsDir, from, to, dir, artifact, control, ref string }
	cli.Default.Register(&cli.Command{
		Group: "sprint-end", Name: "facts", Summary: "the period's aggregate, each Sprint Goal's ledger and reported status, and the recommended version",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&a.project, "project", "", "the repository the sprint-end issue is in (required)")
			fs.StringVar(&a.reportsDir, "reports", "", "a checkout of a central reports repository, read when the roster in the system repository names no reporter; else the reports are read where they were written, and the facts say which (GH-2725, GH-2775)")
			fs.StringVar(&a.from, "from", "", "the period's first week, YYYY-Www (required)")
			fs.StringVar(&a.to, "to", "", "the period's last week, YYYY-Www (required)")
			fs.StringVar(&a.dir, "dir", ".", "the repository's checkout, for its release tags")
			fs.StringVar(&a.artifact, "artifact", "", "the artifact the repository releases (required)")
			fs.StringVar(&a.control, "control", "", "program repository holding control/policy.md; empty is the system repository beside the checkout, else the defaults")
			fs.StringVar(&a.ref, "ref", "main", "ref of the control files")
		},
		Run: func(ctx *cli.Context) error {
			if a.project == "" || a.from == "" || a.to == "" || a.artifact == "" {
				return fmt.Errorf("%w: facts needs --project, --from, --to, and --artifact", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			p := program.Defaults()
			control := program.ControlFor(a.control, a.dir, "")
			if control != "" {
				if p, err = program.LoadPolicy(t, control, a.ref); err != nil {
					return err
				}
			}
			agg, err := periodAggregate(t, p, control, a.ref, a.reportsDir, a.project, a.from, a.to)
			if err != nil {
				return err
			}
			tags, err := release.RepoTags(a.dir)
			if err != nil {
				return err
			}
			in := Input{Tracker: t, Project: a.project, Aggregate: agg, Releases: p.Releases, Artifact: a.artifact, Tags: tags, Dir: a.dir}
			if a.control != "" && program.ProjectPath(a.control) == program.ProjectPath(a.project) {
				subs, err := program.LoadSubsystems(t, a.control, a.ref)
				if err != nil && !errors.Is(err, tracker.ErrNotFound) {
					return err
				}
				in.Teams = subs.Repositories()
			}
			f, err := MakeFacts(in)
			if err != nil {
				return err
			}
			var h strings.Builder
			fmt.Fprintf(&h, "%s, %s to %s: %d reports, %d signals; recommended version %s: %s\n", f.Project, a.from, a.to, agg.Reports, len(agg.Signals), orNothing(f.Recommended), f.Next.Reason)
			fmt.Fprintf(&h, "  reports read %s\n", agg.Source)
			for _, o := range f.SprintGoals {
				// Reported complete is not met: only track-goals decides met
				// (srd038 R2.2), so the line names which one holds.
				outcome := o.Outcome
				if outcome == "reported complete" {
					outcome += ", not yet met"
				}
				fmt.Fprintf(&h, "  %s %s (%s): %s, reported %q, %s\n", o.Issue, strings.Join(o.Serves, ","), o.Release, o.State, o.Reported, outcome)
			}
			return ctx.Emit(f, h.String())
		},
	})

	var st struct{ dir, release, done, project string }
	cli.Default.Register(&cli.Command{
		Group: "sprint-end", Name: "stories", Summary: "record which of the Sprint Backlog's stories the owner reports done in the sprint-end summary",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&st.dir, "dir", ".", "the sprint-end issue's worktree")
			fs.StringVar(&st.release, "release", "", "the release whose summary records them (required)")
			fs.StringVar(&st.done, "done", "", "the done stories, project#story or a story id of --project, comma-separated; empty records none")
			fs.StringVar(&st.project, "project", "", "the project a bare story id belongs to")
		},
		Run: func(ctx *cli.Context) error {
			if st.release == "" {
				return fmt.Errorf("%w: stories needs --release", cli.ErrUsage)
			}
			ids := []string{}
			for _, id := range strings.Split(st.done, ",") {
				if id = strings.TrimSpace(id); id == "" {
					continue
				}
				if !strings.Contains(id, "#") {
					if st.project == "" {
						return fmt.Errorf("%w: story %q names no project; give project#story or --project", cli.ErrUsage, id)
					}
					id = st.project + "#" + id
				}
				ids = append(ids, id)
			}
			path := filepath.Join(st.dir, sprintgoal.SummaryDir, st.release+".md")
			changed, err := sprintgoal.RecordStoriesDone(path, ids, ctx.DryRun)
			if err != nil {
				return err
			}
			res := struct {
				Path    string   `json:"path"`
				Done    []string `json:"done"`
				Changed bool     `json:"changed"`
			}{path, ids, changed}
			return ctx.Emit(res, fmt.Sprintf("%s: %d stories done, changed=%v", path, len(ids), changed))
		},
	})
}

func orNothing(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

// periodAggregate is the period's aggregate for the sprint end of project:
// its team's, or for the system repository the program's (GH-116), from
// the source reports.ChooseSource names (GH-2775). In place it reads the
// reports where they were written, collected for the team's subsystem or
// the program (GH-2725); else the central reports checkout reportsDir.
func periodAggregate(t tracker.Tracker, p program.Policy, control, ref, reportsDir, project, from, to string) (reports.Aggregation, error) {
	if control == "" && !reports.IsCentral(reportsDir) {
		return reports.Aggregation{}, fmt.Errorf("%w: facts needs --control or --reports to read the period's reports", cli.ErrUsage)
	}
	s, err := reports.ChooseSource(t, control, ref, reportsDir)
	if err != nil {
		return reports.Aggregation{}, err
	}
	var in reports.AggregateInput
	if s.InPlace {
		level := "team"
		if program.ProjectPath(project) == program.ProjectPath(control) {
			level = reports.ProgramLevel
		}
		col, err := reports.Collect(t, reports.CollectInput{Config: s.Config, System: control, Ref: ref, Level: level, Project: project, From: from, To: to})
		if err != nil {
			return reports.Aggregation{}, err
		}
		in = reports.AggregateInput{Config: s.Config, Policy: p, From: from, To: to, Collection: &col}
	} else {
		team := ""
		if program.ProjectPath(project) != program.ProjectPath(s.Config.System) {
			team = project
		}
		in = reports.AggregateInput{Dir: s.Central, Config: s.Config, Policy: p, From: from, To: to, Project: team}
	}
	a, err := reports.Aggregate(t, in)
	if err != nil {
		return a, err
	}
	in.Collection = nil
	s.Explain(t, &a, in)
	return a, nil
}
