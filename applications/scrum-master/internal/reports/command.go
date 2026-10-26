// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

func init() {
	var initArgs struct {
		dir, system, reminder string
		members, agents       []string
	}
	cli.Default.Register(&cli.Command{
		Group: "reports", Name: "init", Summary: "scaffold a reports repository in a local checkout",
		Flags: func(fs *flag.FlagSet) {
			initArgs.members, initArgs.agents = nil, nil
			fs.StringVar(&initArgs.dir, "dir", ".", "local checkout of the reports repository")
			fs.StringVar(&initArgs.system, "system", "", "system repository project path")
			fs.StringVar(&initArgs.reminder, "reminder", DefaultReminderWeekday, "reminder weekday")
			fs.Func("member", "username=project of a person who reports (repeatable)", func(v string) error {
				initArgs.members = append(initArgs.members, v)
				return nil
			})
			// One flag for every member, as a profile's one parameter passes
			// them (GH-37).
			fs.Func("members", "username=project of each person who reports, comma-separated", func(v string) error {
				for _, m := range strings.Split(v, ",") {
					if m = strings.TrimSpace(m); m != "" {
						initArgs.members = append(initArgs.members, m)
					}
				}
				return nil
			})
			fs.Func("agent", "install the skills for cursor, claude, copilot, or all (repeatable)", func(v string) error {
				initArgs.agents = append(initArgs.agents, v)
				return nil
			})
		},
		Run: func(ctx *cli.Context) error {
			cfg := Config{System: initArgs.system, ReminderWeekday: initArgs.reminder}
			for _, m := range initArgs.members {
				user, project, ok := strings.Cut(m, "=")
				if !ok || user == "" || project == "" {
					return fmt.Errorf("%w: --member %q is not username=project", cli.ErrUsage, m)
				}
				cfg.Team = append(cfg.Team, Member{user, project})
			}
			if _, err := Weekday(cfg.ReminderWeekday); err != nil {
				return fmt.Errorf("%w: %v", cli.ErrUsage, err)
			}
			var agents []string
			if len(initArgs.agents) > 0 {
				var err error
				if agents, err = install.ParseAgents(initArgs.agents); err != nil {
					return fmt.Errorf("%w: %v", cli.ErrUsage, err)
				}
			}
			res, err := Init(initArgs.dir, cfg, time.Now(), ctx.DryRun)
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, r := range res {
				fmt.Fprintf(&b, "%-8s %s\n", r.Action, r.Path)
			}
			out := struct {
				Files  []InitResult     `json:"files"`
				Skills []install.Result `json:"skills,omitempty"`
			}{Files: res}
			if len(agents) > 0 {
				skills, err := install.EmbeddedSkills()
				if err != nil {
					return err
				}
				if out.Skills, err = install.Install(initArgs.dir, agents, skills, install.Version(), false, ctx.DryRun); err != nil {
					return err
				}
				fmt.Fprintf(&b, "skills: %d files for the named agents\n", len(out.Skills))
			}
			return ctx.Emit(out, b.String())
		},
	})

	var statusArgs struct{ dir, reports, ref, date, control string }
	cli.Default.Register(&cli.Command{
		Group: "reports", Name: "status-issues", Summary: "leave one weekly status issue per listed person for the coming week",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&statusArgs.dir, "dir", ".", "local checkout of the reports repository holding reports.md")
			fs.StringVar(&statusArgs.reports, "reports", "", "the reports repository's project path (required)")
			fs.StringVar(&statusArgs.ref, "ref", "main", "ref of the system repository's control files and subsystem SRDs")
			fs.StringVar(&statusArgs.date, "date", "", "run as of this day YYYY-MM-DD; empty is today")
			fs.StringVar(&statusArgs.control, "control", "", "the system repository holding control/reports.md: file each person's status issue where they report, and the team and program rollup issues (GH-2725)")
		},
		Run: func(ctx *cli.Context) error {
			if statusArgs.control != "" {
				return statusInPlace(ctx, statusArgs.control, statusArgs.ref, statusArgs.date)
			}
			if statusArgs.reports == "" {
				statusArgs.reports = strings.TrimSpace(os.Getenv(ReportsEnv)) //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
			}
			if statusArgs.reports == "" {
				return fmt.Errorf("%w: status-issues needs --control, or --reports for a central reports repository", cli.ErrUsage)
			}
			cfg, err := LoadConfig(statusArgs.dir)
			if err != nil {
				return err
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			p, err := program.LoadPolicy(t, cfg.System, statusArgs.ref)
			if err != nil {
				return err
			}
			now := time.Now()
			if statusArgs.date != "" {
				if now, err = time.Parse("2006-01-02", statusArgs.date); err != nil {
					return fmt.Errorf("%w: --date %q is not YYYY-MM-DD", cli.ErrUsage, statusArgs.date)
				}
			}
			res, err := StatusIssues(t, StatusInput{Config: cfg, Reports: statusArgs.reports, Policy: p, Ref: statusArgs.ref, Now: now, DryRun: ctx.DryRun})
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, r := range res {
				fmt.Fprintf(&b, "%-8s %s %s#%d due %s\n", r.Action, r.User, r.Project, r.IID, r.DueDate)
			}
			return ctx.Emit(res, b.String())
		},
	})

	var weekArgs struct{ project, user, date, control, ref string }
	cli.Default.Register(&cli.Command{
		Group: "reports", Name: "week", Summary: "the open Sprint Goals and a person's activity this week",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&weekArgs.project, "project", "", "project path (required)")
			fs.StringVar(&weekArgs.user, "user", "", "tracker username; empty is the caller")
			fs.StringVar(&weekArgs.date, "date", "", "a day YYYY-MM-DD of the week to gather; empty is today")
			fs.StringVar(&weekArgs.control, "control", "", "the system repository, whose subsystem SRDs name the teams whose Sprint Goals bound a project holding none of its own")
			fs.StringVar(&weekArgs.ref, "ref", "main", "ref of the control files")
		},
		Run: func(ctx *cli.Context) error {
			if weekArgs.project == "" {
				return fmt.Errorf("%w: week needs --project", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			user := weekArgs.user
			if user == "" {
				me, err := tracker.UserIn(t, weekArgs.project)
				if err != nil {
					return err
				}
				user = me.Username
			}
			day := time.Now()
			if weekArgs.date != "" {
				if day, err = time.Parse(time.DateOnly, weekArgs.date); err != nil {
					return fmt.Errorf("%w: --date %q is not YYYY-MM-DD", cli.ErrUsage, weekArgs.date)
				}
			}
			var teams []string
			if weekArgs.control != "" {
				subs, err := program.LoadSubsystems(t, weekArgs.control, weekArgs.ref)
				if err != nil && !errors.Is(err, tracker.ErrNotFound) {
					return err
				}
				teams = subs.Repositories()
			}
			r, err := Week(t, weekArgs.project, user, day, teams)
			if err != nil {
				return err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%s, %s since %s\n", r.Project, r.User, r.Since)
			for _, o := range r.SprintGoals {
				fmt.Fprintf(&b, "  Sprint Goal %s due %s: %s\n", o.Ref, o.DueDate, o.Title)
			}
			fmt.Fprintf(&b, "  %d merged, %d closed\n", len(r.Merged), len(r.Closed))
			return ctx.Emit(r, b.String())
		},
	})

	var aggArgs struct {
		dir, from, to, ref                            string
		control, level, subsystem, project, collected string
		push                                          bool
	}
	cli.Default.Register(&cli.Command{
		Group: "reports", Name: "aggregate", Summary: "aggregate a week or a range of reports into signals by component",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&aggArgs.dir, "dir", ".", "local checkout of the reports repository")
			fs.StringVar(&aggArgs.from, "from", "", "first ISO week YYYY-Www; empty is this week")
			fs.StringVar(&aggArgs.to, "to", "", "last ISO week YYYY-Www; empty is --from")
			fs.StringVar(&aggArgs.ref, "ref", "main", "ref of the system repository's control files")
			fs.BoolVar(&aggArgs.push, "push", false, "file each actionable signal as an issue in the system repository")
			fs.StringVar(&aggArgs.control, "control", "", "the system repository holding control/reports.md: aggregate a level's reports collected where they were written, not --dir's (GH-2725)")
			fs.StringVar(&aggArgs.level, "level", "", "with --control, team or program")
			fs.StringVar(&aggArgs.subsystem, "subsystem", "", "with --level team, the team's subsystem SRD id; empty is the one naming --project")
			fs.StringVar(&aggArgs.project, "project", "", "with --level team, the repository the aggregate runs in")
			fs.StringVar(&aggArgs.collected, "collected", "", "with --control, a file holding reports collect's JSON to aggregate instead of collecting again")
		},
		Run: func(ctx *cli.Context) error {
			from, to := weeksOf(aggArgs.from, aggArgs.to)
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			var cfg Config
			var col *Collection
			if aggArgs.control != "" {
				if cfg, err = roster(t, aggArgs.control, aggArgs.ref); err != nil {
					return err
				}
				if col, err = collection(t, cfg, aggArgs.control, aggArgs.ref, aggArgs.level, aggArgs.subsystem, aggArgs.project, aggArgs.collected, from, to); err != nil {
					return err
				}
				from, to = col.From, col.To
			} else if cfg, err = LoadConfig(aggArgs.dir); err != nil {
				return err
			}
			p, err := program.LoadPolicy(t, cfg.System, aggArgs.ref)
			if err != nil {
				return err
			}
			a, err := Aggregate(t, AggregateInput{Dir: aggArgs.dir, Config: cfg, Policy: p, From: from, To: to, Collection: col})
			if err != nil {
				return err
			}
			if aggArgs.push {
				if cfg.System == "" {
					return fmt.Errorf("%w: --push needs --control, or system in reports.md", cli.ErrUsage)
				}
				if a.Pushed, err = PushSignals(t, cfg.System, a.Signals, p, ctx.DryRun); err != nil {
					return err
				}
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%d report(s), %s to %s\n", a.Reports, a.From, a.To)
			for _, s := range a.Signals {
				fmt.Fprintf(&b, "  %-24s %s\n", s.Kind, s.Detail)
			}
			for _, pr := range a.Progress {
				fmt.Fprintf(&b, "  progress %s (%s): %s\n", pr.SprintGoal, pr.Component, pr.State)
			}
			for _, pr := range a.Pushed {
				line := fmt.Sprintf("  pushed %s: %s #%d", pr.Key, pr.Action, pr.IID)
				// An issue filed now says how admission went, as scrum-master issue
				// file does (srd018 R3.6).
				if pr.Action == "filed" && pr.Admitted {
					// A signal issue is a management issue: admission
					// clears it, but only a data issue moves to ready, so it
					// stays open until someone takes it (srd041; GH-20).
					line += ", admitted; a management issue, it stays open until someone takes it"
				} else if pr.Action == "filed" {
					line += ", needs-triage: " + strings.Join(pr.Reasons, "; ")
				}
				fmt.Fprintln(&b, line)
			}
			return ctx.Emit(a, b.String())
		},
	})

	var validateDir string
	cli.Default.Register(&cli.Command{
		Group: "reports", Name: "validate", Summary: "check reports against the report format",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&validateDir, "dir", ".", "local checkout of the reports repository; reports to check may follow as arguments")
		},
		Run: func(ctx *cli.Context) error {
			checked, findings, err := Validate(validateDir, ctx.Flags.Args())
			if err != nil {
				return err
			}
			var b strings.Builder
			for _, f := range findings {
				fmt.Fprintf(&b, "%s:\n  %s\n", f.Path, strings.Join(f.Problems, "\n  "))
			}
			fmt.Fprintf(&b, "%d report(s) checked, %d invalid", checked, len(findings))
			if err := ctx.Emit(map[string]any{"checked": checked, "findings": findings}, b.String()); err != nil {
				return err
			}
			if len(findings) > 0 {
				return fmt.Errorf("%d report(s) break the report format", len(findings))
			}
			return nil
		},
	})
}
