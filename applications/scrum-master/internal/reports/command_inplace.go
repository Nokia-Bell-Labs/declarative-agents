// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// roster reads control/reports.md from the system repository; with none
// there, the roster is empty and the system named.
func roster(t tracker.Tracker, system, ref string) (Config, error) {
	cfg, err := LoadControl(t, system, ref)
	if errors.Is(err, tracker.ErrNotFound) {
		cfg = Config{System: system}
		cfg.defaults()
		return cfg, nil
	}
	return cfg, err
}

// day reads --date, empty being today.
func day(v string) (time.Time, error) {
	if v == "" {
		return time.Now(), nil
	}
	d, err := time.Parse(time.DateOnly, v)
	if err != nil {
		return d, fmt.Errorf("%w: --date %q is not YYYY-MM-DD", cli.ErrUsage, v)
	}
	return d, nil
}

func init() {
	var c struct{ control, ref, level, subsystem, project, from, to string }
	cli.Default.Register(&cli.Command{
		Group: "reports", Name: "collect", Summary: "collect a team's or the program's merged reports and rollups, and its open status issues, from the repositories where they were written",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&c.control, "control", "", "the system repository, whose subsystem SRDs name the program's repositories (required)")
			fs.StringVar(&c.ref, "ref", "main", "ref of the system repository's control files and subsystem SRDs")
			fs.StringVar(&c.level, "level", "", "team or program (required)")
			fs.StringVar(&c.subsystem, "subsystem", "", "the team's subsystem SRD id; empty is the one naming --project")
			fs.StringVar(&c.project, "project", "", "the repository the collection runs in, naming the team when --subsystem is empty")
			fs.StringVar(&c.from, "from", "", "first ISO week YYYY-Www; empty is this week")
			fs.StringVar(&c.to, "to", "", "last ISO week YYYY-Www; empty is --from")
		},
		Run: func(ctx *cli.Context) error {
			if c.control == "" || c.level == "" {
				return fmt.Errorf("%w: collect needs --control and --level", cli.ErrUsage)
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			cfg, err := roster(t, c.control, c.ref)
			if err != nil {
				return err
			}
			from, to := weeksOf(c.from, c.to)
			col, err := Collect(t, CollectInput{Config: cfg, System: c.control, Ref: c.ref, Level: c.level, Subsystem: c.subsystem, Project: c.project, From: from, To: to})
			if err != nil {
				return err
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%s %s, %s to %s: %d repositories\n", col.Level, col.Subsystem, col.From, col.To, len(col.Repositories))
			for _, f := range col.Reports {
				fmt.Fprintf(&b, "  report  %s %s %s (%s)\n", f.Repository, f.Path, short(f.Commit), f.Source)
			}
			for _, f := range col.Rollups {
				fmt.Fprintf(&b, "  rollup  %s %s %s (%s)\n", f.Repository, f.Path, short(f.Commit), f.Source)
			}
			for _, o := range col.Open {
				fmt.Fprintf(&b, "  open    %s %s#%d %s %s due %s\n", o.Kind, o.Repository, o.IID, o.Name, o.Week, o.DueDate)
			}
			for _, u := range col.Unread {
				fmt.Fprintf(&b, "  unread  %s: %s\n", u.Repository, u.Reason)
			}
			return ctx.Emit(col, b.String())
		},
	})

	var p struct{ project, user, date, control, ref string }
	cli.Default.Register(&cli.Command{
		Group: "reports", Name: "previous", Summary: "a person's last report before this week: its blockers and what they committed to next",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&p.project, "project", "", "the repository the person reports in (required)")
			fs.StringVar(&p.user, "user", "", "tracker username; empty is the caller")
			fs.StringVar(&p.date, "date", "", "a day YYYY-MM-DD of this week; empty is today")
			fs.StringVar(&p.control, "control", "", "the system repository, whose roster may name a central reports repository still read")
			fs.StringVar(&p.ref, "ref", "main", "ref of the system repository's control files")
		},
		Run: func(ctx *cli.Context) error {
			if p.project == "" {
				return fmt.Errorf("%w: previous needs --project", cli.ErrUsage)
			}
			d, err := day(p.date)
			if err != nil {
				return err
			}
			t, err := ctx.Tracker()
			if err != nil {
				return err
			}
			user := p.user
			if user == "" {
				me, err := tracker.UserIn(t, p.project)
				if err != nil {
					return err
				}
				user = me.Username
			}
			cfg := Config{}
			if p.control != "" {
				if cfg, err = roster(t, p.control, p.ref); err != nil {
					return err
				}
			}
			prev, err := PreviousReport(t, cfg, p.project, user, d)
			if err != nil {
				return err
			}
			var b strings.Builder
			if !prev.Found {
				fmt.Fprintf(&b, "no report by %s in %s before %s\n", user, p.project, ISOWeek(d))
			} else {
				fmt.Fprintf(&b, "%s %s %s, %s\n", prev.Repository, prev.Path, short(prev.Commit), prev.Week)
				for _, n := range prev.Next {
					fmt.Fprintf(&b, "  next     %s\n", n)
				}
				for _, e := range prev.Blockers {
					fmt.Fprintf(&b, "  blocker  %s (waiting on %s since %s)\n", e["blocker"], e["waiting-on"], e["since"])
				}
			}
			return ctx.Emit(prev, b.String())
		},
	})

	var ph struct{ user, level, date, dir string }
	cli.Default.Register(&cli.Command{
		Group: "reports", Name: "path", Summary: "where this week's report or rollup goes, as its status issue names it",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&ph.user, "user", "", "the person whose report it is (required without --level)")
			fs.StringVar(&ph.level, "level", "", "a rollup's level instead: program, or the team's subsystem SRD id")
			fs.StringVar(&ph.date, "date", "", "a day YYYY-MM-DD of the week; empty is today")
			fs.StringVar(&ph.dir, "dir", ".", "the checkout the report is written in: a central reports repository's, holding reports.md, names its path and branch")
		},
		Run: func(ctx *cli.Context) error {
			if ph.user == "" && ph.level == "" {
				return fmt.Errorf("%w: path needs --user or --level", cli.ErrUsage)
			}
			d, err := day(ph.date)
			if err != nil {
				return err
			}
			_, err = os.Stat(filepath.Join(ph.dir, ConfigFile))
			sp := PathOf(ph.user, ph.level, d, err == nil)
			if sp.Branch == "" {
				return ctx.Emit(sp, fmt.Sprintf("%s, on the branch pop makes for the status issue\n", sp.Path))
			}
			return ctx.Emit(sp, fmt.Sprintf("%s on branch %s\n", sp.Path, sp.Branch))
		},
	})
}

// weeksOf defaults an empty first week to this week and an empty last to
// the first.
func weeksOf(from, to string) (string, string) {
	if from == "" {
		from = ISOWeek(time.Now())
	}
	if to == "" {
		to = from
	}
	return from, to
}

func short(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

// statusInPlace is status-issues against the roster in the system
// repository: each person's issue where they report, then the team and
// program rollup issues.
func statusInPlace(ctx *cli.Context, system, ref, date string) error {
	t, err := ctx.Tracker()
	if err != nil {
		return err
	}
	cfg, err := LoadControl(t, system, ref)
	if errors.Is(err, tracker.ErrNotFound) {
		return fmt.Errorf("%s holds no %s: the roster of who reports, and where, lives there (GH-2725)", system, ControlFile)
	}
	if err != nil {
		return err
	}
	p, err := program.LoadPolicy(t, system, ref)
	if err != nil {
		return err
	}
	now, err := day(date)
	if err != nil {
		return err
	}
	lead := ""
	if me, err := tracker.UserIn(t, system); err == nil {
		lead = me.Username
	}
	res, err := StatusIssuesInPlace(t, RollupInput{Config: cfg, System: system, Ref: ref, Policy: p, Lead: lead, Now: now, DryRun: ctx.DryRun})
	if err != nil {
		return err
	}
	var b strings.Builder
	for _, r := range res {
		fmt.Fprintf(&b, "%-8s %-14s %s %s#%d due %s\n", r.Action, r.Kind, r.User, r.Project, r.IID, r.DueDate)
	}
	return ctx.Emit(res, b.String())
}

// collection is a level's collection: read from a file reports collect
// wrote, or collected now.
func collection(t tracker.Tracker, cfg Config, system, ref, level, subsystem, project, file, from, to string) (*Collection, error) {
	if file != "" {
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		var col Collection
		if err := json.Unmarshal(b, &col); err != nil {
			return nil, fmt.Errorf("%s: %w", file, err)
		}
		return &col, nil
	}
	if level == "" {
		return nil, fmt.Errorf("%w: aggregate with --control needs --level or --collected", cli.ErrUsage)
	}
	col, err := Collect(t, CollectInput{Config: cfg, System: system, Ref: ref, Level: level, Subsystem: subsystem, Project: project, From: from, To: to})
	if err != nil {
		return nil, err
	}
	return &col, nil
}
