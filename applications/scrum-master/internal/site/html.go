// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"bytes"
	_ "embed"
	"encoding/json"
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
// library and makes no request.
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

// chartPlan is a corpus tag's plan: each item's planned span then, which
// the page draws as a ghost behind the plan now, so slippage shows.
type chartPlan struct {
	Label   string                    `json:"label"`
	Planned map[string]*timeline.Span `json:"planned"`
}

// chartData is everything the page draws.
type chartData struct {
	Today    string             `json:"today"`
	Origin   string             `json:"origin"`
	Releases []timeline.Release `json:"releases"`
	Items    []chartItem        `json:"items"`
	Plans    []chartPlan        `json:"plans"`
}

// TimelineHTML is the self-contained timeline page (srd048 R5.3): the plan
// now, and each corpus tag's, newest first, for the page to draw.
func TimelineHTML(now timeline.Facts, tags map[string]timeline.Facts) ([]byte, error) {
	d := chartData{Today: now.Today, Origin: sprintOrigin(now), Releases: now.Releases, Items: []chartItem{}, Plans: []chartPlan{}}
	if d.Releases == nil {
		d.Releases = []timeline.Release{}
	}
	for _, it := range now.Items {
		d.Items = append(d.Items, chartItem{ID: it.ID, Kind: it.Kind, Title: it.Title, Parent: it.Parent, Status: it.Status, Release: it.Release,
			Planned: it.Planned, Actual: it.Actual, Points: it.Points, PointsDone: it.PointsDone, Rank: it.Rank, URL: it.URL,
			Selects: it.Selects, DependsOn: it.DependsOn, Due: it.Due})
	}
	names := make([]string, 0, len(tags))
	for tag := range tags {
		names = append(names, tag)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, tag := range names {
		p := chartPlan{Label: tag, Planned: map[string]*timeline.Span{}}
		for _, it := range tags[tag].Items {
			if it.Planned != nil {
				p.Planned[it.ID] = it.Planned
			}
		}
		d.Plans = append(d.Plans, p)
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
// renders the page.
func timelineHTMLPage(dir string) ([]byte, error) {
	read := func(name string) (timeline.Facts, error) {
		var f timeline.Facts
		b, err := os.ReadFile(filepath.Join(dir, Dir, "timeline", name))
		if err != nil {
			return f, err
		}
		return f, json.Unmarshal(b, &f)
	}
	now, err := read("facts.json")
	if err != nil {
		return nil, err
	}
	tags := map[string]timeline.Facts{}
	matches, _ := filepath.Glob(filepath.Join(dir, Dir, "timeline", "facts@*.json"))
	for _, m := range matches {
		f, err := read(filepath.Base(m))
		if err != nil {
			return nil, err
		}
		tag := f.AsOf
		if tag == "" {
			tag = strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), "facts@"), ".json")
		}
		tags[tag] = f
	}
	return TimelineHTML(now, tags)
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
