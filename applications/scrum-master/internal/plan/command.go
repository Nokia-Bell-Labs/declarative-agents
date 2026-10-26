// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package plan

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/goals"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/release"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/routing"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// programGroup is the group a release's milestone is dated in: the one
// named, else the program group the system checkout's control/trackers.md
// records, else the system project's parent group (srd031 R4.2, GH-103).
func programGroup(named, dir, system string) string {
	if named != "" {
		return named
	}
	if g := routing.RecordedGroup(dir); g != "" {
		return g
	}
	return path.Dir(program.ProjectPath(system))
}

func init() {
	var a struct{ dir, system, aggregate, rollup, artifact, control, ref string }
	var velocity int
	common := func(fs *flag.FlagSet) {
		fs.StringVar(&a.dir, "dir", ".", "the system repository's checkout")
		fs.StringVar(&a.system, "system", "", "system repository path, for slipped objectives and the last milestone")
		fs.StringVar(&a.aggregate, "aggregate", "", "the period's scrum-master reports aggregate --json output, for slipped objectives")
		fs.StringVar(&a.rollup, "rollup", "", "the period's program rollup, for slipped objectives; empty with no --aggregate is the latest under --dir's status/rollups (GH-2725)")
		fs.StringVar(&a.artifact, "artifact", "corpus", "the artifact the system repository releases its plan as")
		fs.StringVar(&a.control, "control", "", "program repository holding control/policy.md; defaults to --system")
		fs.StringVar(&a.ref, "ref", "main", "ref of the control files")
	}
	candidates := func(ctx *cli.Context) (Candidates, tracker.Tracker, error) {
		in := Input{Dir: a.dir, System: a.system, Artifact: a.artifact, Now: time.Now(), Releases: program.Defaults().Releases}
		if a.system != "" {
			t, err := ctx.Tracker()
			if err != nil {
				return Candidates{}, nil, err
			}
			in.Tracker = t
			control := a.control
			if control == "" {
				control = a.system
			}
			p, err := program.LoadPolicy(t, control, a.ref)
			if err != nil {
				return Candidates{}, nil, err
			}
			in.Releases = p.Releases
		}
		if a.aggregate != "" {
			b, err := os.ReadFile(a.aggregate)
			if err != nil {
				return Candidates{}, nil, err
			}
			var agg reports.Aggregation
			if err := json.Unmarshal(b, &agg); err != nil {
				return Candidates{}, nil, fmt.Errorf("%s: %w", a.aggregate, err)
			}
			in.Aggregate = &agg
		} else if path := a.rollup; path != "" || reports.LatestProgramRollup(a.dir) != "" {
			if path == "" {
				path = reports.LatestProgramRollup(a.dir)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return Candidates{}, nil, err
			}
			r, err := reports.ParseRollup(path, b)
			if err != nil {
				return Candidates{}, nil, err
			}
			agg := r.Aggregation()
			in.Aggregate = &agg
		}
		tags, err := release.RepoTags(a.dir)
		if err != nil {
			return Candidates{}, nil, err
		}
		in.Tags = tags
		c, err := MakeCandidates(in)
		if err == nil && teamRepository(a.dir) {
			// A team's repository does not date or tag the system's
			// release: its sprint is the system's period (srd031 R6.4). Its
			// release is the system's open one, never the 01.0 init laid
			// down in its own road map (GH-141).
			c.EndDate, c.Tag = "", ""
			c.Version, c.Latest = openRelease(in.Tracker, a.system), ""
		}
		if err == nil && hasStories(a.dir) {
			b, berr := MakeBacklog(a.dir, velocity)
			if berr != nil {
				return c, in.Tracker, berr
			}
			c.Backlog = &b
		}
		return c, in.Tracker, err
	}

	cli.Default.Register(&cli.Command{
		Group: "plan", Name: "candidates", Summary: "rank the initiative items or requirements the next release could prove, and in a team's repository its Product Backlog, with the recommended version, end date, and tag",
		Flags: func(fs *flag.FlagSet) {
			common(fs)
			fs.IntVar(&velocity, "velocity", 0, "the team's assumed velocity in story points, when no sprint-end summary records done stories")
		},
		Run: func(ctx *cli.Context) error {
			c, _, err := candidates(ctx)
			if err != nil {
				return err
			}
			var h strings.Builder
			// A team repository plans a sprint from its backlog and has no
			// release of its own to name: print only what is known (GH-20).
			if c.Version != "" {
				line := "next release " + c.Version
				if c.EndDate != "" {
					line += ", ending " + c.EndDate
				}
				if c.Tag != "" {
					line += ", tagged " + c.Tag
				}
				h.WriteString(line + "\n")
			}
			for _, it := range c.Items {
				flag := ""
				if it.Slipped {
					flag = " (slipped)"
				}
				fmt.Fprintf(&h, "  %s%s due %s: %s\n", it.ID, flag, it.Due, it.Text)
			}
			for _, it := range c.WithoutEvidence {
				fmt.Fprintf(&h, "  held back %s: missing %s\n", it.ID, strings.Join(it.Missing, " and "))
			}
			if b := c.Backlog; b != nil {
				fmt.Fprintln(&h, backlogLine(*b))
				for _, st := range b.Stories {
					fmt.Fprintf(&h, "  %s (%d points, rank %s): %s\n", st.ID, st.Points, st.Rank, st.Title)
				}
			}
			return ctx.Emit(c, h.String())
		},
	})

	var pu struct{ version, end, group string }
	cli.Default.Register(&cli.Command{
		Group: "plan", Name: "push", Summary: "date the milestone and sprint issue, post planned events, and file the period's sprint-end issues",
		Flags: func(fs *flag.FlagSet) {
			common(fs)
			fs.StringVar(&pu.version, "version", "", "the road-map release to push (required)")
			fs.StringVar(&pu.end, "end", "", "the period's end date, YYYY-MM-DD (required)")
			fs.StringVar(&pu.group, "group", "", "the program group the milestone lives on; defaults to the program group control/trackers.md records, else the system repository's group")
		},
		Run: func(ctx *cli.Context) error {
			if a.system == "" || pu.version == "" || pu.end == "" {
				return fmt.Errorf("%w: push needs --system, --version, and --end", cli.ErrUsage)
			}
			if err := program.CheckProject("system", a.system); err != nil {
				return fmt.Errorf("%w: %v", cli.ErrUsage, err)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			control := a.control
			if control == "" {
				control = a.system
			}
			p, err := program.LoadPolicy(t, control, a.ref)
			if err != nil {
				return err
			}
			group := programGroup(pu.group, a.dir, a.system)
			web, err := goals.WebBase(a.dir)
			if err != nil {
				return err
			}
			res, err := Push(t, PushInput{Dir: a.dir, System: a.system, Group: group, Version: pu.version, End: pu.end,
				WebBase: web, Policy: p, Ref: a.ref, DryRun: ctx.DryRun})
			if err != nil {
				return err
			}
			var h strings.Builder
			fmt.Fprintf(&h, "milestone %s: %s\n%s\n", res.Release.Milestone, res.Milestone, issueLine("sprint issue", res.Sprint))
			for _, s := range res.SprintEnds {
				fmt.Fprintln(&h, issueLine("sprint-end issue", s))
			}
			for _, e := range res.Planned {
				fmt.Fprintf(&h, "planned event on %s: posted=%v\n", e.Issue, e.Posted)
			}
			for _, s := range res.Skipped {
				fmt.Fprintf(&h, "skipped %s\n", s)
			}
			return ctx.Emit(res, h.String())
		},
	})

	pushWordInput := func(ctx *cli.Context, version, end, group string) (tracker.Tracker, PushInput, error) {
		if a.system == "" || version == "" || end == "" {
			return nil, PushInput{}, fmt.Errorf("%w: the push words need --system, --version, and --end", cli.ErrUsage)
		}
		// The plan-push profile's words take the system from
		// SCRUM_MASTER_SYSTEM; a checkout path there is refused as the
		// one-command push refuses it (D1, GH-93).
		if err := program.CheckProject("system", a.system); err != nil {
			return nil, PushInput{}, err
		}
		t, err := ctx.Tracker()
		if err != nil {
			return nil, PushInput{}, err
		}
		control := a.control
		if control == "" {
			control = a.system
		}
		p, err := program.LoadPolicy(t, control, a.ref)
		if err != nil {
			return nil, PushInput{}, err
		}
		group = programGroup(group, a.dir, a.system)
		web, err := goals.WebBase(a.dir)
		if err != nil {
			return nil, PushInput{}, err
		}
		return t, PushInput{Dir: a.dir, System: a.system, Group: group, Version: version, End: end,
			WebBase: web, Policy: p, Ref: a.ref, DryRun: ctx.DryRun}, nil
	}

	var pw struct{ version, end, group string }
	pushWordFlags := func(fs *flag.FlagSet) {
		common(fs)
		fs.StringVar(&pw.version, "version", "", "the road-map release (required)")
		fs.StringVar(&pw.end, "end", "", "the period's end date, YYYY-MM-DD (required)")
		fs.StringVar(&pw.group, "group", "", "the program group the milestone lives on; defaults to the program group control/trackers.md records, else the system repository's group")
	}
	cli.Default.Register(&cli.Command{
		Group: "plan", Name: "milestone", Summary: "the milestone word: date the release's milestone in every group it serves (GH-206)",
		Flags: pushWordFlags,
		Run: func(ctx *cli.Context) error {
			t, in, err := pushWordInput(ctx, pw.version, pw.end, pw.group)
			if err != nil {
				return err
			}
			status, groups, err := PushMilestone(t, in)
			if err != nil {
				return err
			}
			r := struct {
				Version   string   `json:"version"`
				Milestone string   `json:"milestone"`
				Groups    []string `json:"milestone_groups,omitempty"`
				DryRun    bool     `json:"dry_run"`
			}{in.Version, status, groups, ctx.DryRun}
			return ctx.Emit(r, fmt.Sprintf("milestone %s in %s", status, strings.Join(groups, ", ")))
		},
	})
	cli.Default.Register(&cli.Command{
		Group: "plan", Name: "sprint-issue", Summary: "the sprint word: the release's one sprint issue with its exit checklist, upserted by marker (GH-206)",
		Flags: pushWordFlags,
		Run: func(ctx *cli.Context) error {
			t, in, err := pushWordInput(ctx, pw.version, pw.end, pw.group)
			if err != nil {
				return err
			}
			action, err := PushSprintIssue(t, in)
			if err != nil {
				return err
			}
			return ctx.Emit(action, fmt.Sprintf("sprint issue %s#%d: %s", action.Project, action.IID, action.Action))
		},
	})
	cli.Default.Register(&cli.Command{
		Group: "plan", Name: "sprint-ends", Summary: "the sprint-ends word: one sprint-end issue per releasing repository, upserted by marker (GH-206)",
		Flags: pushWordFlags,
		Run: func(ctx *cli.Context) error {
			t, in, err := pushWordInput(ctx, pw.version, pw.end, pw.group)
			if err != nil {
				return err
			}
			actions, skipped, err := PushSprintEnds(t, in)
			if err != nil {
				return err
			}
			r := struct {
				SprintEnds []IssueAction `json:"sprint_end_issues"`
				Skipped    []string      `json:"skipped,omitempty"`
				DryRun     bool          `json:"dry_run"`
			}{actions, skipped, ctx.DryRun}
			var h strings.Builder
			for _, s := range actions {
				fmt.Fprintln(&h, issueLine("sprint-end issue", s))
			}
			for _, s := range skipped {
				fmt.Fprintf(&h, "skipped %s\n", s)
			}
			return ctx.Emit(r, h.String())
		},
	})
	cli.Default.Register(&cli.Command{
		Group: "plan", Name: "events", Summary: "the events word: a planned event on each open Sprint Goal the release moves, posted once (GH-206)",
		Flags: pushWordFlags,
		Run: func(ctx *cli.Context) error {
			t, in, err := pushWordInput(ctx, pw.version, pw.end, pw.group)
			if err != nil {
				return err
			}
			planned, err := PushEvents(t, in)
			if err != nil {
				return err
			}
			r := struct {
				Planned []PlannedOnIssue `json:"planned_events"`
				DryRun  bool             `json:"dry_run"`
			}{planned, ctx.DryRun}
			var h strings.Builder
			for _, e := range planned {
				fmt.Fprintf(&h, "planned event on %s: posted=%v\n", e.Issue, e.Posted)
			}
			if len(planned) == 0 {
				h.WriteString("no open Sprint Goal needed a planned event\n")
			}
			return ctx.Emit(r, h.String())
		},
	})

	var w struct{ version, milestone, tag, items, issue string }
	cli.Default.Register(&cli.Command{
		Group: "plan", Name: "write", Summary: "write the chosen release into docs/road-map.md in the checkout",
		Flags: func(fs *flag.FlagSet) {
			common(fs)
			fs.StringVar(&w.version, "version", "", "the release's version, such as 01.2 (required)")
			fs.StringVar(&w.milestone, "milestone", "", "the release's milestone; defaults to the version")
			fs.StringVar(&w.tag, "tag", "", "the tag that publishes the plan (required)")
			fs.StringVar(&w.items, "items", "", "comma-separated objective items, objNNN-short-name#Rn.m (required)")
			fs.StringVar(&w.issue, "issue", "", "planning issue as project#iid, to comment on items held back")
		},
		Run: func(ctx *cli.Context) error {
			if w.version == "" || w.tag == "" || w.items == "" {
				return fmt.Errorf("%w: write needs --version, --tag, and --items", cli.ErrUsage)
			}
			c, t, err := candidates(ctx)
			if err != nil {
				return err
			}
			r := Release{Version: w.version, Milestone: w.milestone, Tag: w.tag, Items: strings.Split(w.items, ",")}
			if r.Milestone == "" {
				r.Milestone = r.Version
			}
			var project string
			var iid int
			if w.issue != "" {
				if project, iid, err = release.ParseIssueRef(w.issue); err != nil {
					return fmt.Errorf("%w: %v", cli.ErrUsage, err)
				}
				if t == nil {
					if t, err = ctx.Tracker(); err != nil {
						return err
					}
				}
			}
			if ctx.DryRun {
				t = nil
			}
			res, err := WriteRelease(a.dir, r, c, t, project, iid)
			if err != nil {
				return err
			}
			h := fmt.Sprintf("%s release %s in %s: %s\n", res.Action, r.Version, res.Path, strings.Join(res.Release.Items, ", "))
			for _, x := range res.Returned {
				h += fmt.Sprintf("returned %s: missing %s\n", x.ID, strings.Join(x.Missing, " and "))
			}
			return ctx.Emit(res, h)
		},
	})

	var bl struct{ project, release, stories string }
	cli.Default.Register(&cli.Command{
		Group: "plan", Name: "backlog", Summary: "write the stories chosen at Sprint Planning onto the team's Sprint Goal and post a planned event naming them",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&a.dir, "dir", ".", "the team repository's checkout, holding its stories")
			fs.StringVar(&bl.project, "project", "", "the team's repository, where tasks filed its Sprint Goal (required)")
			fs.StringVar(&bl.release, "release", "", "the release the Sprint Goal counts toward (required)")
			fs.StringVar(&bl.stories, "stories", "", "the chosen stories' ids, comma-separated (required)")
		},
		Run: func(ctx *cli.Context) error {
			if bl.project == "" || bl.release == "" || bl.stories == "" {
				return fmt.Errorf("%w: backlog needs --project, --release, and --stories", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			var chosen []string
			for _, id := range strings.Split(bl.stories, ",") {
				if id = strings.TrimSpace(id); id != "" {
					chosen = append(chosen, id)
				}
			}
			r, err := PushBacklog(t, bl.project, bl.release, a.dir, chosen, ctx.DryRun)
			if err != nil {
				return err
			}
			return ctx.Emit(r, fmt.Sprintf("%s: %s, %d stories, planned event posted=%v", r.Issue, r.Action, len(r.Stories), r.Planned))
		},
	})
}

// issueLine is one issue push filed or found: project#iid once it has a
// number, and the repository it goes to when a dry run has not filed it.
func issueLine(kind string, a IssueAction) string {
	if a.IID == 0 {
		return fmt.Sprintf("%s in %s: %s, not yet numbered", kind, a.Project, a.Action)
	}
	return fmt.Sprintf("%s %s#%d: %s", kind, a.Project, a.IID, a.Action)
}

// backlogLine heads the Product Backlog: the velocity and the stories it
// recommends, or that there is no velocity to recommend from.
func backlogLine(b Backlog) string {
	if b.Velocity == 0 {
		return "Product Backlog, no velocity to recommend from: no sprint-end summary records a story done; pass --velocity"
	}
	if len(b.Recommended) == 0 {
		return fmt.Sprintf("Product Backlog, velocity %d (measured %v), no story fits", b.Velocity, b.Measured)
	}
	return fmt.Sprintf("Product Backlog, velocity %d (measured %v), recommended %s", b.Velocity, b.Measured, strings.Join(b.Recommended, ", "))
}
