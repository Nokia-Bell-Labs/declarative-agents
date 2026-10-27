// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"errors"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// InitResult is one file init considered.
type InitResult struct {
	Path   string `json:"path"`
	Action string `json:"action"` // created, kept
}

const deckTemplate = `# {title}

<!-- The release skill renders the deck from this template: one slide per
outline section of an approved summary, up to the slide limit (srd018 R4). -->
`

// readme is the reports repository's README: what it holds, and how the
// site in it is read and refreshed (srd018 R1.1, srd048 R7.2).
const readme = `# Reports

The program's reports repository: each person's weekly status report under
reports/, the report format in schema/, the sprint-end summaries in
summaries/, the release deck's template in deck/, and the program's site
in site/.

## The site

site/ holds the program's status, timeline, and requirement history, and
its corpus, as Markdown pages the site skill generates from the system
repository and the tracker. Open site/index.md on the forge: GitHub and
GitLab render the pages and their mermaid gantt charts as they are, so a
reader needs access to this repository and nothing else, and can answer
what exists, what is being built and when it lands, and what is planned
beyond, without git. Every generated page closes with a stamp; a page you
edit is kept, never replaced. site/index.md is yours to edit.

To refresh it, run the site skill from a checkout of this repository with
the system repository checked out beside it, and land the change like any
other: file the issue, pop it, and merge it. The site never commits.

In CI, a scheduled job on the release image runs the site profile over
this checkout and the system's beside it, then files and merges the
change through the same issue workflow:

    agent --profile "$SCRUM_MASTER_HOME/agents/site/profile.yaml" \
      --core-root "$SCRUM_MASTER_HOME/agent-core" --directory .

A static site generator (Docsify, Hugo, or Starlight) can later read the
same files; none is needed to read them.
`

// siteReadme is the README of a repository that is the program's site's
// home only: the reports are written where the work is (GH-2725).
const siteReadme = `# Site

The program's site: its status, timeline, requirement history, and corpus,
as Markdown pages the site skill generates from the system repository and
the tracker, in site/, and the release deck's template in deck/. Weekly
reports and rollups are not here: each is written and merged in the
repository where its author worked, under status/, and collected from
there (the roster is the system repository's control/reports.md).

Open site/index.md on the forge: GitHub and GitLab render the pages and
their mermaid gantt charts as they are. Every generated page closes with a
stamp; a page you edit is kept, never replaced. site/index.md is yours to
edit.

To refresh it, run the site skill from a checkout of this repository with
the system repository checked out beside it, and land the change like any
other: file the issue, pop it, and merge it. The site never commits.
`

// SiteIndexStub is the site's front page until the site skill first runs,
// which replaces it then, as it does not replace an index the program
// wrote (GH-107).
const SiteIndexStub = `# Program site

The site skill writes the program's status, timeline, requirement history,
and corpus here; run it to fill this site. This page is yours to edit.
`

// Init scaffolds the repository the program's site lives in, in a local
// checkout (srd018 R1): its README, the site's front page, and a deck
// template. A program whose roster is in the system repository writes its
// reports where the work is, so that is all (GH-2725): inPlace, its roster
// naming a reporter, or no member given and no reports.md here. A central
// reports repository also gets the configuration, the current quarter's
// reports directory, the report schema, and a summaries directory, as
// before. It never overwrites a file that exists; a dry run writes nothing.
func Init(dir string, cfg Config, now time.Time, inPlace, dryRun bool) ([]InitResult, error) {
	_, err := os.Stat(filepath.Join(dir, ConfigFile))
	if inPlace || (len(cfg.Team) == 0 && errors.Is(err, fs.ErrNotExist)) {
		return write(dir, []struct{ path, content string }{
			{filepath.Join("deck", "template.md"), deckTemplate},
			{"README.md", siteReadme},
			{filepath.Join("site", "index.md"), SiteIndexStub},
		}, dryRun)
	}
	cfg.defaults()
	conf, err := corpus.FormatConfig("reports", "Reports configuration", cfg)
	if err != nil {
		return nil, err
	}
	example := RenderWith("po-1", ISOWeek(now), []Entry{
		{"sprint_goal": "group/product-owners/po-1#4", "component": "grader", "status": "at-risk", "complete": "no", "problems": "the scoring API returns stale results"},
	}, []Entry{
		{"blocker": "stale results from the scoring API", "component": "grader", "since": now.Format("2006-01-02"), "waiting-on": "scoring-api"},
	}, []string{"the grader scores the first task end to end"}, []string{"score a second task once the scoring API is fixed"})
	files := []struct{ path, content string }{
		{ConfigFile, string(conf)},
		{filepath.Join("reports", Quarter(now), ".gitkeep"), ""},
		{filepath.Join("schema", "report.md"), example + "\n<!-- The report format (config format report): one file per person per ISO week at\nreports/<YYYY-Qn>/<YYYY-Www>-<username>.md, with the sections Work, Blockers, Impact, and Next; the reports-validate\nskill checks it, and refuses a report written in place, under status/, without Impact or Next (GH-2778). -->\n"},
		{filepath.Join("summaries", ".gitkeep"), ""},
		{filepath.Join("deck", "template.md"), deckTemplate},
		{"README.md", readme},
		{filepath.Join("site", "index.md"), SiteIndexStub},
	}
	return write(dir, files, dryRun)
}

// write creates each file that does not exist and keeps each that does.
func write(dir string, files []struct{ path, content string }, dryRun bool) ([]InitResult, error) {
	var out []InitResult
	for _, f := range files {
		p := filepath.Join(dir, f.path)
		r := InitResult{Path: f.path, Action: "kept"}
		if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
			r.Action = "created"
			if !dryRun {
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return out, err
				}
				if err := os.WriteFile(p, []byte(f.content), 0o644); err != nil {
					return out, err
				}
			}
		} else if err != nil {
			return out, err
		}
		out = append(out, r)
	}
	return out, nil
}
