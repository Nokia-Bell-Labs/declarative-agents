// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package plan

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// BacklogStory is one story of a team's Product Backlog.
type BacklogStory struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Points int    `json:"points"`
	Rank   string `json:"rank,omitempty"`
	Parent string `json:"parent,omitempty"`
}

// Backlog is the team's Product Backlog at Sprint Planning (srd031 R6.4,
// srd045 R5): its agreed stories that are not done, by rank, the team's
// velocity, and the stories that fit it.
type Backlog struct {
	Stories []BacklogStory `json:"stories"`
	// Velocity is the story points of the stories the newest sprint-end
	// summary records as done; Measured is false when no summary records
	// any, and the skill asks for an assumed velocity (srd045 R5.3).
	Velocity    int      `json:"velocity"`
	Measured    bool     `json:"measured"`
	Recommended []string `json:"recommended"`
}

// MakeBacklog reads the stories and sprint-end summaries of the team's
// checkout at dir. assumed is the velocity the Product Owner gave when the
// team has none measured; zero recommends nothing.
func MakeBacklog(dir string, assumed int) (Backlog, error) {
	docs, err := hierarchy.Load(dir)
	if err != nil {
		return Backlog{}, err
	}
	done, err := sprintgoal.StoriesDone(dir)
	if err != nil {
		return Backlog{}, err
	}
	points := map[string]int{}
	var stories []*hierarchy.Doc
	for _, d := range docs {
		if d.Kind != hierarchy.Story {
			continue
		}
		points[d.ID] = d.PointsValue()
		if d.Status == "agreed" && done.Release[d.ID] == "" {
			stories = append(stories, d)
		}
	}
	sort.SliceStable(stories, func(i, j int) bool {
		if a, b := stories[i].RankValue(), stories[j].RankValue(); a != b {
			return a < b
		}
		return stories[i].ID < stories[j].ID
	})
	b := Backlog{Stories: []BacklogStory{}, Recommended: []string{}}
	for _, s := range stories {
		b.Stories = append(b.Stories, BacklogStory{ID: s.ID, Title: s.Title, Points: s.PointsValue(), Rank: s.Rank, Parent: s.Parent})
	}
	if done.Latest != "" {
		b.Measured = true
		for _, id := range done.Last {
			b.Velocity += points[id]
		}
	} else {
		b.Velocity = assumed
	}
	sum := 0
	for _, s := range b.Stories {
		if sum+s.Points > b.Velocity {
			break
		}
		sum += s.Points
		b.Recommended = append(b.Recommended, s.ID)
	}
	return b, nil
}

// BacklogResult is what scrum-master plan backlog reports.
type BacklogResult struct {
	Issue   string   `json:"issue"`
	Stories []string `json:"stories"`
	Action  string   `json:"action"` // updated or unchanged
	Planned bool     `json:"planned_event_posted"`
	DryRun  bool     `json:"dry_run"`
}

// PushBacklog writes the chosen stories, with their points, into the
// Sprint Backlog section of the team's Sprint Goal for the release in
// project and posts a planned event naming them (srd031 R6.6, srd034
// R3.5). A second push with the same stories changes nothing; a dry run
// writes nothing.
func PushBacklog(t tracker.Tracker, project, release, dir string, chosen []string, dryRun bool) (BacklogResult, error) {
	r := BacklogResult{Stories: chosen, DryRun: dryRun}
	if len(chosen) == 0 {
		return r, fmt.Errorf("choose at least one story")
	}
	docs, err := hierarchy.Load(dir)
	if err != nil {
		return r, err
	}
	byID := map[string]*hierarchy.Doc{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	var lines strings.Builder
	for _, id := range chosen {
		d, ok := byID[id]
		if !ok || d.Kind != hierarchy.Story || d.Status != "agreed" {
			return r, fmt.Errorf("%s is not an agreed story in %s", id, dir)
		}
		fmt.Fprintf(&lines, "- %s %s (%d points)\n", id, d.Title, d.PointsValue())
	}
	goal, err := findSprintGoal(t, project, release)
	if err != nil {
		return r, err
	}
	r.Issue = fmt.Sprintf("%s#%d", project, goal.IID)
	body, err := withBacklog(goal.Description, lines.String())
	if err != nil {
		return r, fmt.Errorf("%s: %w", r.Issue, err)
	}
	r.Action = "unchanged"
	if body != goal.Description {
		r.Action = "updated"
		if !dryRun {
			if _, err := t.UpdateIssue(project, goal.IID, tracker.IssueUpdate{Description: &body}); err != nil {
				return r, err
			}
		}
	}
	var cites []string
	for _, c := range links.ParseCites(goal.Description) {
		cites = append(cites, c.URL)
	}
	e, text := events.PlannedStories(release, cites, chosen, environmentOf(goal.Description))
	r.Planned, err = events.Post(t, project, goal.IID, e, events.PlannedStoriesRequired, text, dryRun)
	return r, err
}

// findSprintGoal returns the open Sprint Goal tasks filed in project for the
// release; an objective issue has no Sprint Backlog, so it is not one.
func findSprintGoal(t tracker.Tracker, project, release string) (tracker.Issue, error) {
	issues, err := t.ListIssues(project, tracker.IssueQuery{State: "opened", Labels: []string{"plane::management"}})
	if err != nil {
		return tracker.Issue{}, err
	}
	for _, i := range issues {
		if m, ok := sprintgoal.Parse(i.Description); ok && !m.Legacy && m.Release == release {
			return i, nil
		}
	}
	return tracker.Issue{}, fmt.Errorf("no open Sprint Goal for release %s in %s; tasks files it first (srd022)", release, project)
}

// withBacklog replaces the Sprint Backlog section's lines, between its
// heading and the next heading or the marker.
func withBacklog(description, lines string) (string, error) {
	const heading = "## Sprint Backlog\n"
	i := strings.Index(description, heading)
	if i < 0 {
		return "", fmt.Errorf("the Sprint Goal has no Sprint Backlog section")
	}
	start := i + len(heading)
	rest := description[start:]
	end := len(rest)
	for _, stop := range []string{"\n## ", "\n<!-- scrum-master goals "} {
		if j := strings.Index(rest, stop); j >= 0 && j < end {
			end = j + 1
		}
	}
	return description[:start] + "\n" + lines + "\n" + strings.TrimLeft(rest[end:], "\n"), nil
}

// environmentOf reads the Sprint Goal's Environment line.
func environmentOf(description string) string {
	for _, line := range strings.Split(description, "\n") {
		if env, ok := strings.CutPrefix(strings.TrimSpace(line), "Environment:"); ok {
			return strings.TrimSpace(env)
		}
	}
	return ""
}

// hasStories reports whether the checkout at dir holds any story.
func hasStories(dir string) bool {
	docs, err := hierarchy.Load(dir)
	return err == nil && slices.ContainsFunc(docs, func(d *hierarchy.Doc) bool { return d.Kind == hierarchy.Story })
}
