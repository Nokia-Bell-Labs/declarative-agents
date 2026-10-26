// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"flag"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// ReportsEnv names the reports repository's checkout when --dir does not,
// as the status-issues profile reads it (srd018 R1.3).
const ReportsEnv = reports.ReportsEnv

// reportsDir is the reports repository's checkout: --dir, else
// SCRUM_MASTER_REPORTS, else the checkout beside this one named for
// project, as repositories sit side by side under their owner. No program
// configuration names it (srd018 R1.3).
func reportsDir(dir, project string) (string, error) {
	if dir != "" {
		return dir, nil
	}
	if env := strings.TrimSpace(os.Getenv(ReportsEnv)); env != "" { //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
		return env, nil
	}
	if project != "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		return (&checkout.Code{Dir: wd}).Checkout(project), nil
	}
	return ".", nil
}

// word is one family of pages: its name, and the run that writes them into
// the reports checkout.
type word struct {
	name, summary string
	run           func(ctx *cli.Context, dir string, r *Result) error
}

// words are the site's words in the order the site profile's machine runs
// them (srd048 R1.1).
var words = []word{
	{"scaffold", "the scaffold word: the site's index when missing, and a page for each generated folder its word has not yet written",
		func(ctx *cli.Context, dir string, r *Result) error {
			s, err := Scaffold(dir, install.Version(), ctx.DryRun)
			r.Pages = append(r.Pages, s.Pages...)
			return err
		}},
	{"status", "the status word: the road map, story-point roll-up, architecture status, Sprint Goals, signals, and open issues, under site/status/ with their facts under site/data/status/",
		func(ctx *cli.Context, dir string, r *Result) error {
			system, err := SystemDir(dir)
			if err != nil {
				return err
			}
			var t tracker.Tracker
			if tr, err := ctx.Tracker(); err == nil {
				t = tr
			}
			in, err := LoadStatusInput(dir, system, t, time.Now().UTC())
			if err != nil {
				return err
			}
			pages, data, notes, err := StatusPages(in)
			if err != nil {
				return err
			}
			published, err := Publish(dir, "status", pages, install.Version(), ctx.DryRun)
			if err != nil {
				return err
			}
			facts, err := Publish(dir, "data/status", data, install.Version(), ctx.DryRun)
			r.Pages, r.Notes = append(append(r.Pages, published...), facts...), append(append(r.Notes, in.Sources.Notes...), notes...)
			return err
		}},
	{"timeline", "the timeline word: each item's planned and actual days on the sprint axis, now and as planned at each corpus tag, under site/timeline/",
		func(ctx *cli.Context, dir string, r *Result) error {
			system, err := SystemDir(dir)
			if err != nil {
				return err
			}
			// The timeline reads Sprint Goals and dated milestones from
			// the tracker when one is reachable, and says so when not.
			var t tracker.Tracker
			if tr, err := ctx.Tracker(); err == nil {
				t = tr
			}
			s, err := LoadSources(system, t, time.Now().UTC())
			if err != nil {
				return err
			}
			pages, _, err := TimelinePages(system, s)
			if err != nil {
				return err
			}
			published, err := Publish(dir, "timeline", pages, install.Version(), ctx.DryRun)
			r.Pages, r.Notes = append(r.Pages, published...), append(r.Notes, s.Notes...)
			return err
		}},
}

// ganttWord draws the gantt views from the timeline facts.
var ganttWord = word{"gantt", "the gantt word: the program's and each initiative's timeline as mermaid gantt charts under site/timeline/gantt/, drawn from the timeline facts",
	func(ctx *cli.Context, dir string, r *Result) error {
		pages, err := GanttPages(dir)
		if err != nil {
			return err
		}
		published, err := Publish(dir, GanttDir, pages, install.Version(), ctx.DryRun)
		r.Pages = append(r.Pages, published...)
		return err
	}}

// historyWord lists each requirement's changes between corpus tags.
var historyWord = word{"history", "the history word: the requirements and acceptance criteria added, reworded, retired, removed, or moved between consecutive corpus tags, each with its merge, under site/history/",
	func(ctx *cli.Context, dir string, r *Result) error {
		system, err := SystemDir(dir)
		if err != nil {
			return err
		}
		steps, err := History(system)
		if err != nil {
			return err
		}
		pages, docs, data := HistoryPages(steps)
		for _, p := range []struct {
			folder string
			pages  map[string][]byte
		}{{"history", pages}, {"history/docs", docs}, {"data/history", data}} {
			published, err := Publish(dir, p.folder, p.pages, install.Version(), ctx.DryRun)
			r.Pages = append(r.Pages, published...)
			if err != nil {
				return err
			}
		}
		return nil
	}}

// corpusWord copies the system repository's documents into the site.
var corpusWord = word{"corpus", "the corpus word: the system repository's documents under site/corpus/, links fixed to resolve in the site or at the forge",
	func(ctx *cli.Context, dir string, r *Result) error {
		system, err := SystemDir(dir)
		if err != nil {
			return err
		}
		// An encrypted corpus stays out of an unencrypted site: one page
		// says why, and publishing it alone prunes any copied before
		// (GH-115).
		if why := withheld(system, dir); why != "" {
			r.Notes = append(r.Notes, "corpus: "+why)
			page := generated("corpus", "the system repository's documents") + "# Corpus\n\nNo document is copied here: " + why + ".\n"
			published, err := PublishTree(dir, CorpusDir, map[string][]byte{Dir + "/" + CorpusDir + "/index.md": []byte(page)}, install.Version(), ctx.DryRun)
			r.Pages = append(r.Pages, published...)
			return err
		}
		steps, err := HistorySteps(dir)
		if err != nil {
			return err
		}
		pages, err := CorpusPages(system, steps)
		if err != nil {
			return err
		}
		published, err := PublishTree(dir, CorpusDir, pages, install.Version(), ctx.DryRun)
		r.Pages = append(r.Pages, published...)
		return err
	}}

// hierarchyWord writes a page per planning document, with its links up and
// down and the Sprint Goals and issues that serve it (GH-126), and the
// site's navigator over them (GH-127).
var hierarchyWord = word{"hierarchy", "the hierarchy word: a page per theme, initiative, epic, feature, and story under site/hierarchy/, linking its parent, its children, the Sprint Goals and issues that serve it, and its source, drawn from the timeline facts",
	func(ctx *cli.Context, dir string, r *Result) error {
		system, err := SystemDir(dir)
		if err != nil {
			return err
		}
		pages, data, sprints, sidebar, err := HierarchyPages(dir, Repos(system, dir))
		if err != nil {
			return err
		}
		published, err := PublishTree(dir, HierarchyDir, pages, install.Version(), ctx.DryRun)
		r.Pages = append(r.Pages, published...)
		if err != nil {
			return err
		}
		sprintPages, err := Publish(dir, SprintsDir, sprints, install.Version(), ctx.DryRun)
		r.Pages = append(r.Pages, sprintPages...)
		if err != nil {
			return err
		}
		nav, err := Write(dir, Sidebar, sidebar, install.Version(), ctx.DryRun)
		r.Pages = append(r.Pages, nav)
		if err != nil {
			return err
		}
		facts, err := Publish(dir, "data/hierarchy", data, install.Version(), ctx.DryRun)
		r.Pages = append(r.Pages, facts...)
		return err
	}}

// overviewWord writes the program's home: the interactive timeline, a tile
// per initiative, and the releases (GH-128).
var overviewWord = word{"overview", "the overview word: the program's home page, site/overview.md, with the interactive timeline, a tile per initiative, and the releases, drawn from the timeline facts",
	func(ctx *cli.Context, dir string, r *Result) error {
		system, err := SystemDir(dir)
		if err != nil {
			return err
		}
		name := "Program overview"
		if repos := Repos(system, dir); len(repos) > 0 {
			name = repos[0].Name
		}
		page, err := OverviewPage(dir, name)
		if err != nil {
			return err
		}
		p, err := Write(dir, Overview, page, install.Version(), ctx.DryRun)
		r.Pages = append(r.Pages, p)
		return err
	}}

func init() {
	words = append(words, ganttWord, historyWord, corpusWord, hierarchyWord, overviewWord)
	var dir, project string
	flags := func(fs *flag.FlagSet) {
		fs.StringVar(&dir, "dir", "", "the reports repository's checkout (default: "+ReportsEnv+", else the checkout beside this one named for --project)")
		fs.StringVar(&project, "project", "", "the reports repository's project path, whose checkout sits beside this one")
	}
	run := func(ws []word) func(ctx *cli.Context) error {
		return func(ctx *cli.Context) error {
			d, err := reportsDir(dir, project)
			if err != nil {
				return err
			}
			r := Result{Dir: d, Pages: []Page{}, DryRun: ctx.DryRun}
			for _, w := range ws {
				if err := w.run(ctx, d, &r); err != nil {
					return err
				}
			}
			// Words that read the same sources say the same thing once.
			seen := map[string]bool{}
			r.Notes = slices.DeleteFunc(r.Notes, func(n string) bool { defer func() { seen[n] = true }(); return seen[n] })
			return ctx.Emit(r, Human(r))
		}
	}
	// The one-shot run: every word in the order the site profile's machine
	// runs them.
	cli.Default.Register(&cli.Command{
		Group: "site", Summary: "write the generated status and timeline site into the reports repository: every word in order",
		Flags: flags, Run: run(words),
	})
	for _, w := range words {
		cli.Default.Register(&cli.Command{Group: "site", Name: w.name, Summary: w.summary, Flags: flags, Run: run([]word{w})})
	}
}
