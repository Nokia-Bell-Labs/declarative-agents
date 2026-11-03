// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// BacklogDir is the backlog word's folder: every issue the stories record
// (srd045 R4.4), across the program's repositories, open and closed,
// drawn from the timeline facts and never the tracker (srd048 R10).
const BacklogDir = "backlog"

// The backlog page filters the same record in the browser: repository,
// state, and who, with no library and no request of its own.
//
//go:embed templates/backlog.html.tmpl
var backlogHTML string

var backlogPage = template.Must(template.New("backlog").Parse(backlogHTML))

// backlogRow is one recorded issue as the backlog draws it.
type backlogRow struct {
	Ref        string   `json:"ref"`
	URL        string   `json:"url,omitempty"`
	Title      string   `json:"title,omitempty"`
	Repository string   `json:"repository"`
	Who        []string `json:"who,omitempty"`
	Opened     string   `json:"opened,omitempty"`
	Closed     string   `json:"closed,omitempty"`
	State      string   `json:"state"`
	Days       string   `json:"days,omitempty"`
	Projected  string   `json:"projected,omitempty"`
	Story      string   `json:"story"`
	StoryTitle string   `json:"story_title"`
	Epic       string   `json:"epic,omitempty"`
	EpicTitle  string   `json:"epic_title,omitempty"`
}

// BacklogPages are the backlog word's pages, drawn from the timeline
// facts the timeline word wrote into the reports checkout dir (srd048
// R10): the folder's index with an open and a closed table, the forge's
// view, and the embedded page that filters the same rows by repository,
// state, and who. Every fact comes from the stories' and epics' issues records, read
// from the checkouts; the word asks the tracker nothing.
func BacklogPages(dir string) (map[string][]byte, error) {
	b, err := os.ReadFile(filepath.Join(dir, Dir, "timeline", "facts.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s holds no timeline facts; run the site's timeline word first", dir)
	} else if err != nil {
		return nil, err
	}
	var f timeline.Facts
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("%s/timeline/facts.json: %w", Dir, err)
	}
	rows := backlogRows(f)
	index := backlogIndex(rows)
	data, err := json.Marshal(struct {
		Today string       `json:"today"`
		Rows  []backlogRow `json:"rows"`
	}{f.Today, rows})
	if err != nil {
		return nil, err
	}
	var page bytes.Buffer
	if err := backlogPage.Execute(&page, struct{ Title, Data string }{"Backlog", string(data)}); err != nil {
		return nil, err
	}
	return map[string][]byte{
		Dir + "/" + BacklogDir + "/index.md":     []byte(index),
		Dir + "/" + BacklogDir + "/backlog.html": page.Bytes(),
	}, nil
}

// backlogRows flattens the stories' issues records into rows, each with
// its story, its epic, its days so far or taken, and, open, the sprint
// end of its story's release as the projected time (srd048 R10).
func backlogRows(f timeline.Facts) []backlogRow {
	byID := map[string]timeline.Item{}
	for _, it := range f.Items {
		byID[it.ID] = it
	}
	release := map[string]timeline.Release{}
	for _, r := range f.Releases {
		release[r.Version] = r
	}
	var rows []backlogRow
	for _, it := range f.Items {
		if len(it.Ledger) == 0 {
			continue
		}
		epicID, epicTitle := "", ""
		for cur, ok := it, true; ok; cur, ok = byID[cur.Parent] {
			if cur.Kind == "epic" {
				epicID, epicTitle = cur.ID, cur.Title
				break
			}
			if cur.Parent == "" {
				break
			}
		}
		projected := ""
		if r, ok := release[it.Release]; ok && r.Sprint != nil && r.Sprint.End != "" {
			projected = "release " + r.Version + ", by " + r.Sprint.End
		}
		for _, e := range it.Ledger {
			repo, _, _ := strings.Cut(e.Ref, "#")
			row := backlogRow{Ref: e.Ref, URL: e.URL, Title: e.Title, Repository: repo, Who: e.Who,
				Opened: e.Opened, Closed: e.Closed, State: e.State,
				Epic: epicID, EpicTitle: epicTitle}
			// An issue naming the epic directly has no story to link.
			if it.Kind != "epic" {
				row.Story, row.StoryTitle = it.ID, it.Title
			}
			switch {
			case e.Closed != "":
				row.Days = daysBetween(e.Opened, e.Closed)
			default:
				row.Days = daysBetween(e.Opened, f.Today)
				row.Projected = projected
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].State != rows[j].State {
			return rows[i].State != "closed"
		}
		return rows[i].Ref < rows[j].Ref
	})
	return rows
}

// daysBetween is the days from one day to another as a reader reads it:
// under a day on the same day, else the count.
func daysBetween(from, to string) string {
	a, errA := time.Parse("2006-01-02", from)
	b, errB := time.Parse("2006-01-02", to)
	if errA != nil || errB != nil {
		return ""
	}
	d := int(b.Sub(a).Hours() / 24)
	switch {
	case d <= 0:
		return "under a day"
	case d == 1:
		return "1 day"
	}
	return fmt.Sprintf("%d days", d)
}

// backlogIndex is the folder's index: the embedded kanban board (GH-2855)
// above an open and a closed table, the forge's view of the same record, every
// issue linked on its tracker where its url opens in a browser and named
// without a link where it does not (R8.5), each with its story and epic.
func backlogIndex(rows []backlogRow) string {
	var b strings.Builder
	b.WriteString(generated("backlog", "the timeline facts") + "# Backlog\n\n")
	b.WriteString("Every issue the stories and epics record (srd045 R4.4), across the program's repositories, open and closed, read from the checkouts, never the tracker. The board's columns are the record: to do is open with nobody on it, in progress is open with someone, done is closed; it filters by repository, state, and who, and the tables are the forge's view of the same record.\n\n")
	b.WriteString("<iframe class=\"timeline\" src=\"backlog/backlog.html?embed=1\" title=\"The backlog board, filterable\"></iframe>\n")
	cell := func(r backlogRow) (issue, story, epic string) {
		issue = r.Ref
		if r.URL != "" {
			issue = fmt.Sprintf("[%s](%s)", r.Ref, r.URL)
		}
		story = "none"
		if r.Story != "" {
			story = fmt.Sprintf("[%s](../hierarchy/%s.md)", r.StoryTitle, r.Story)
		}
		if r.Epic != "" {
			epic = fmt.Sprintf("[%s](../hierarchy/%s.md)", r.EpicTitle, r.Epic)
		}
		return issue, story, epic
	}
	var open, closed []backlogRow
	for _, r := range rows {
		if r.Closed != "" {
			closed = append(closed, r)
		} else {
			open = append(open, r)
		}
	}
	b.WriteString("\n## Open\n\n")
	if len(open) == 0 {
		b.WriteString("No recorded issue is open.\n")
	} else {
		b.WriteString("| issue | title | who | opened | open for | projected | story | epic |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, r := range open {
			issue, story, epic := cell(r)
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", issue, r.Title, strings.Join(r.Who, ", "), r.Opened, r.Days, r.Projected, story, epic)
		}
	}
	b.WriteString("\n## Closed\n\n")
	if len(closed) == 0 {
		b.WriteString("No recorded issue has closed.\n")
	} else {
		b.WriteString("| issue | title | who | opened | closed | took | story | epic |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
		for _, r := range closed {
			issue, story, epic := cell(r)
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", issue, r.Title, strings.Join(r.Who, ", "), r.Opened, r.Closed, r.Days, story, epic)
		}
	}
	if len(rows) == 0 {
		b.WriteString("\nA story records its issues once scrum-master backlog record runs in its scrum repository (srd045 R4.4).\n")
	}
	return b.String()
}
