// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// The timeline page draws the plan itself, as SVG, each item as what it
// is (srd048 R5.3, GH-136): releases as lines, initiatives with their due
// dates, epics as bars, stories as blocks in their sprints, Sprint Goals
// as flags, actual and as-planned spans beside the plan. It carries no
// library; its only request fetches a tag's facts beside it, the first
// time the reader compares with that tag's plan (GH-2859).
//
//go:embed templates/timeline.html.tmpl
var timelineHTML string

var timelinePage = template.Must(template.New("timeline").Parse(timelineHTML))

// chartItem is one item as the page draws it: the facts it needs, trimmed.
type chartItem struct {
	ID         string         `json:"id"`
	Kind       string         `json:"kind"`
	Title      string         `json:"title"`
	Parent     string         `json:"parent,omitempty"`
	Status     string         `json:"status"`
	Release    string         `json:"release,omitempty"`
	Planned    *timeline.Span `json:"planned,omitempty"`
	Actual     *timeline.Span `json:"actual,omitempty"`
	Points     int            `json:"points,omitempty"`
	PointsDone int            `json:"points_done,omitempty"`
	Rank       string         `json:"rank,omitempty"`
	URL        string         `json:"url,omitempty"`
	Selects    []string       `json:"selects,omitempty"`
	DependsOn  []string       `json:"depends_on,omitempty"`
	Due        []timeline.Due `json:"due,omitempty"`
}

// chartPlan names a corpus tag's plan: its label and the facts file
// beside the page. The page inlines no tag's spans; it fetches the file
// the first time the reader compares with that plan, so the page stays
// one size however many tags the program has (GH-2859). The fetched
// items' planned spans draw as a ghost behind the plan now, so slippage
// shows.
type chartPlan struct {
	Label string `json:"label"`
	File  string `json:"file"`
}

// chartData is everything the page draws.
type chartData struct {
	Today    string             `json:"today"`
	Origin   string             `json:"origin"`
	Releases []timeline.Release `json:"releases"`
	Items    []chartItem        `json:"items"`
	Plans    []chartPlan        `json:"plans"`
}

// TimelineHTML is the timeline page (srd048 R5.3): the plan now inlined,
// and each corpus tag's plan named, newest first, fetched beside the page
// only when the reader compares with it (GH-2859).
func TimelineHTML(now timeline.Facts, plans []chartPlan) ([]byte, error) {
	d := chartData{Today: now.Today, Origin: sprintOrigin(now), Releases: now.Releases, Items: []chartItem{}, Plans: plans}
	if d.Releases == nil {
		d.Releases = []timeline.Release{}
	}
	if d.Plans == nil {
		d.Plans = []chartPlan{}
	}
	for _, it := range now.Items {
		d.Items = append(d.Items, chartItem{ID: it.ID, Kind: it.Kind, Title: it.Title, Parent: it.Parent, Status: it.Status, Release: it.Release,
			Planned: it.Planned, Actual: it.Actual, Points: it.Points, PointsDone: it.PointsDone, Rank: it.Rank, URL: it.URL,
			Selects: it.Selects, DependsOn: it.DependsOn, Due: it.Due})
	}
	// json.Marshal escapes <, >, and &, so no fact can close the script
	// block it sits in.
	data, err := json.Marshal(d)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	err = timelinePage.Execute(&b, struct{ Title, Data string }{"Program timeline", string(data)})
	return b.Bytes(), err
}

// timelineHTMLPage reads the timeline facts in the reports checkout and
// renders the page: the working tree's facts inlined, and each tag's file
// named for the page to fetch when the reader compares with its plan —
// read here only for its tag, never inlined (GH-2859).
func timelineHTMLPage(dir string) ([]byte, error) {
	var now timeline.Facts
	b, err := os.ReadFile(filepath.Join(dir, Dir, "timeline", "facts.json"))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &now); err != nil {
		return nil, err
	}
	var plans []chartPlan
	matches, _ := filepath.Glob(filepath.Join(dir, Dir, "timeline", "facts@*.json"))
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			return nil, err
		}
		var f struct {
			AsOf string `json:"as_of"`
		}
		if err := json.Unmarshal(b, &f); err != nil {
			return nil, fmt.Errorf("%s: %w", m, err)
		}
		label := f.AsOf
		if label == "" {
			label = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "facts@"), ".json")
		}
		// The page sits under site/timeline/gantt/, the facts beside its
		// folder.
		plans = append(plans, chartPlan{Label: label, File: "../" + filepath.Base(m)})
	}
	sort.Slice(plans, func(i, j int) bool { return plans[i].Label > plans[j].Label })
	return TimelineHTML(now, plans)
}

// sprintOrigin is the start of the program's earliest release sprint, the
// boundary the page's sprint columns align to (GH-135); empty with no
// release sprint, when the columns start where the chart does.
func sprintOrigin(f timeline.Facts) string {
	origin := ""
	for _, r := range f.Releases {
		if r.Sprint != nil && r.Sprint.Start != "" && (origin == "" || r.Sprint.Start < origin) {
			origin = r.Sprint.Start
		}
	}
	return origin
}
