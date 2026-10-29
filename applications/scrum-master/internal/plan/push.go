// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package plan

import (
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/goals"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/objectives"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// SprintEndLabels are a sprint-end issue's labels besides its component: a
// recurring data issue whose work is a release (srd038 R1.1).
var SprintEndLabels = []string{"plane::data", "type::recurring", "kind::release"}

// PushInput is one scrum-master plan push: the release the road map in Dir holds,
// dated by End, pushed to the system repository and the repositories its
// subsystem SRDs declare.
type PushInput struct {
	Dir     string
	System  string
	Group   string // the program group the milestone lives on
	Version string
	End     string // the milestone's end date, YYYY-MM-DD
	WebBase string
	Policy  program.Policy
	Ref     string
	DryRun  bool
}

// PushResult is what scrum-master plan push did.
type PushResult struct {
	Release   Release `json:"release"`
	Milestone string  `json:"milestone"` // created, updated, unchanged
	// MilestoneGroups are the groups the milestone was written under: the
	// program group first, then each releasing repository's group outside
	// it, so its issues find the milestone.
	MilestoneGroups []string         `json:"milestone_groups,omitempty"`
	Sprint          IssueAction      `json:"sprint_issue"`
	SprintEnds      []IssueAction    `json:"sprint_end_issues"`
	Planned         []PlannedOnIssue `json:"planned_events"`
	Skipped         []string         `json:"skipped,omitempty"`
}

// IssueAction is one issue push filed, updated, or found unchanged.
type IssueAction struct {
	Project  string   `json:"project"`
	IID      int      `json:"iid,omitempty"`
	Action   string   `json:"action"` // filed, updated, unchanged
	Admitted bool     `json:"admitted"`
	Reasons  []string `json:"reasons,omitempty"`
}

// PlannedOnIssue is a planned event push posted, or found already posted.
type PlannedOnIssue struct {
	Issue  string `json:"issue"`
	Posted bool   `json:"posted"`
}

var weekRange = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// pushPlan is what every push word loads from the road map and the system
// repository: the release, its milestone, the objective groups, and the
// releasing repositories (GH-206).
type pushPlan struct {
	r      Release
	m      tracker.Milestone
	docs   []*objectives.Document
	groups []group
	repos  []string
	// ends are the repositories that get a sprint-end issue: the scrum and
	// component repositories the registry names, less an initiative's, which
	// releases nothing (GH-2824).
	ends    []string
	skipped []string
}

// loadPushPlan reads the road map in in.Dir and the system repository's
// subsystem SRDs, so each word recomputes its scope and a re-run updates
// and creates nothing.
func loadPushPlan(t tracker.Tracker, in PushInput) (pushPlan, error) {
	var p pushPlan
	if !weekRange.MatchString(in.End) {
		return p, fmt.Errorf("--end must be YYYY-MM-DD, not %q", in.End)
	}
	end, _ := time.Parse(time.DateOnly, in.End)
	start := periodStart(end)
	releases, err := readRoadMap(in.Dir)
	if err != nil {
		return p, err
	}
	i := slices.IndexFunc(releases, func(r goals.Release) bool { return r.Version == in.Version })
	if i < 0 {
		return p, fmt.Errorf("%s names no release %s; write it with scrum-master plan write first", RoadMap, in.Version)
	}
	p.r = Release{Version: in.Version, Items: releases[i].Objectives, Milestone: releases[i].Milestone, Tag: releases[i].Tag}
	if p.r.Milestone == "" || p.r.Tag == "" {
		return p, fmt.Errorf("release %s has no milestone or tag in release.publication", p.r.Version)
	}
	if p.docs, err = objectives.LoadAll(in.Dir); err != nil {
		return p, err
	}
	p.groups = groupsOf(p.r.Items, p.docs)
	p.m = tracker.Milestone{Title: p.r.Milestone, StartDate: start.Format(time.DateOnly), DueDate: in.End,
		Description: fmt.Sprintf("Release %s, %s to %s.", p.r.Version, isoWeek(start), isoWeek(end))}
	subs, err := program.LoadSubsystems(t, in.System, in.Ref)
	if err != nil {
		return p, err
	}
	p.repos = subs.Repositories()
	ends := slices.Clone(p.repos)
	sort.Strings(ends)
	for _, repo := range slices.Compact(ends) {
		if sub, ok := subs.SubsystemFor(repo); ok && sub.Initiative != "" {
			p.skipped = append(p.skipped, repo+": an initiative's repository; it releases nothing (GH-2811)")
			continue
		}
		p.ends = append(p.ends, repo)
	}
	if !slices.Contains(p.repos, program.ProjectPath(in.System)) {
		p.skipped = append(p.skipped, in.System+": no subsystem SRD declares it as a component, so it has no sprint-end issue")
	}
	return p, nil
}

// ensureProgramMilestone dates the milestone in the program group.
func ensureProgramMilestone(t tracker.Tracker, in PushInput, p pushPlan) (status string, groups []string, err error) {
	if in.DryRun {
		return "dry-run", nil, nil
	}
	if status, err = t.EnsureMilestone(in.Group, p.m); err != nil {
		return "", nil, err
	}
	return status, []string{in.Group}, nil
}

// ensureOtherMilestones dates the milestone in each releasing repository's
// group outside the program group, so its issues find it (srd031 R4.2).
func ensureOtherMilestones(t tracker.Tracker, in PushInput, p pushPlan, have []string) ([]string, error) {
	if in.DryRun {
		return have, nil
	}
	for _, repo := range p.repos {
		group := path.Dir(repo)
		if group == "." || group == in.Group || strings.HasPrefix(group, in.Group+"/") || slices.Contains(have, group) {
			continue
		}
		if _, err := t.EnsureMilestone(group, p.m); err != nil {
			return have, err
		}
		have = append(have, group)
	}
	return have, nil
}

// PushMilestone is the milestone word (GH-206, srd031 R4.7): the release's
// dated milestone in the program group and each releasing repository's
// group outside it.
func PushMilestone(t tracker.Tracker, in PushInput) (status string, groups []string, err error) {
	p, err := loadPushPlan(t, in)
	if err != nil {
		return "", nil, err
	}
	status, groups, err = ensureProgramMilestone(t, in, p)
	if err != nil {
		return "", nil, err
	}
	groups, err = ensureOtherMilestones(t, in, p, groups)
	return status, groups, err
}

// PushSprintIssue is the sprint-issue word (GH-206, srd031 R4.7): the
// release's one sprint issue with its exit checklist, upserted by marker.
func PushSprintIssue(t tracker.Tracker, in PushInput) (IssueAction, error) {
	p, err := loadPushPlan(t, in)
	if err != nil {
		return IssueAction{}, err
	}
	return pushSprint(t, in, p.r, p.groups)
}

// PushSprintEnds is the sprint-ends word (GH-206, srd031 R4.7): one
// sprint-end issue per releasing repository, each upserted by marker. An
// initiative's repository releases nothing, so it gets none and is reported
// skipped (GH-2824).
func PushSprintEnds(t tracker.Tracker, in PushInput) ([]IssueAction, []string, error) {
	p, err := loadPushPlan(t, in)
	if err != nil {
		return nil, nil, err
	}
	actions, err := pushSprintEnds(t, in, p)
	return actions, p.skipped, err
}

func pushSprintEnds(t tracker.Tracker, in PushInput, p pushPlan) ([]IssueAction, error) {
	out := []IssueAction{}
	for _, repo := range p.ends {
		a, err := pushSprintEnd(t, in, p.r, repo)
		if err != nil {
			return out, err
		}
		out = append(out, a)
	}
	return out, nil
}

// PushEvents is the events word (GH-206, srd031 R4.7): a planned event on
// each open Sprint Goal the release moves, posted once.
func PushEvents(t tracker.Tracker, in PushInput) ([]PlannedOnIssue, error) {
	p, err := loadPushPlan(t, in)
	if err != nil {
		return nil, err
	}
	return postPlanned(t, in, p.r, p.repos, p.docs, p.groups)
}

// Push dates the release's milestone, sets the sprint issue's checklist,
// posts a planned event on each open objective issue the release moves,
// and files one sprint-end issue per releasing repository, each found by
// its marker so a second push updates and creates nothing (srd031 R4.2,
// R4.3, R4.6; srd038 R1.1; srd001 R7). It is the four words of the
// plan-push profile as one command, in the one order the words run
// (GH-206, srd031 R4.7).
func Push(t tracker.Tracker, in PushInput) (PushResult, error) {
	var res PushResult
	p, err := loadPushPlan(t, in)
	if err != nil {
		return res, err
	}
	res.Release = p.r
	if res.Milestone, res.MilestoneGroups, err = ensureProgramMilestone(t, in, p); err != nil {
		return res, err
	}
	if res.Sprint, err = pushSprint(t, in, p.r, p.groups); err != nil {
		return res, err
	}
	if res.MilestoneGroups, err = ensureOtherMilestones(t, in, p, res.MilestoneGroups); err != nil {
		return res, err
	}
	res.Skipped = p.skipped
	if res.SprintEnds, err = pushSprintEnds(t, in, p); err != nil {
		return res, err
	}
	if res.Planned, err = postPlanned(t, in, p.r, p.repos, p.docs, p.groups); err != nil {
		return res, err
	}
	return res, nil
}

// group is one objective of the release and the items it places.
type group struct {
	key, title, docPath, environment string
	items                            []string
}

func groupsOf(items []string, docs []*objectives.Document) []group {
	var out []group
	for _, id := range items {
		key, item, _ := strings.Cut(id, "#")
		gid, _, _ := strings.Cut(item, ".")
		k := key + "#" + gid
		if j := slices.IndexFunc(out, func(g group) bool { return g.key == k }); j >= 0 {
			out[j].items = append(out[j].items, item)
			continue
		}
		g := group{key: k, environment: "dev", items: []string{item}}
		for _, d := range docs {
			if d.ID == key {
				g.title, g.docPath = d.Requirements[gid].Title, filepath.ToSlash(filepath.Join(objectives.DirFor(d.ID), filepath.Base(d.Path)))
				if c := d.CriteriaFor(gid); len(c) > 0 && c[0].Environment != "" {
					g.environment = c[0].Environment
				}
			}
		}
		out = append(out, g)
	}
	return out
}

func sprintMarker(version string) string {
	return fmt.Sprintf("<!-- scrum-master plan sprint release=%s -->", version)
}
func sprintEndMarker(version string) string {
	return fmt.Sprintf("<!-- scrum-master plan sprint-end release=%s -->", version)
}

var checked = regexp.MustCompile(`(?m)^- \[x\] (\S+?):`)

// pushSprint keeps one sprint issue for the release with one checklist item
// per objective; an item a person ticked stays ticked (srd001 R7.2, R7.4).
func pushSprint(t tracker.Tracker, in PushInput, r Release, groups []group) (IssueAction, error) {
	a := IssueAction{Project: in.System}
	existing, err := find(t, in.System, "type::sprint", sprintMarker(r.Version))
	if err != nil {
		return a, err
	}
	ticked := map[string]bool{}
	if existing != nil {
		for _, m := range checked.FindAllStringSubmatch(existing.Description, -1) {
			ticked[m[1]] = true
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "The sprint for release %s, ending %s. The program lead ticks each objective at the period's end; no script ticks or closes this issue (srd001 R7.4).\n\n## Exit checklist\n\n", r.Version, in.End)
	for _, g := range groups {
		box := " "
		if ticked[g.key] {
			box = "x"
		}
		fmt.Fprintf(&b, "- [%s] %s: %s\n", box, g.key, g.title)
	}
	b.WriteString("\n" + sprintMarker(r.Version) + "\n")
	return upsert(t, in, a, existing, tracker.NewIssue{Title: "Sprint " + r.Version, Description: b.String(),
		Labels: []string{"plane::management", "type::sprint"}, DueDate: in.End, Milestone: r.Milestone})
}

// pushSprintEnd keeps one sprint-end issue for the release in repo.
func pushSprintEnd(t tracker.Tracker, in PushInput, r Release, repo string) (IssueAction, error) {
	a := IssueAction{Project: repo}
	existing, err := find(t, repo, "type::recurring", sprintEndMarker(r.Version))
	if err != nil {
		return a, err
	}
	l, err := program.Derive(t, repo, in.System, in.Ref, nil, program.Labelling{Plane: "data", Kind: "release"})
	if err != nil {
		return a, err
	}
	labels := append([]string{"type::recurring"}, program.Labels(l.Plane, "", l.Kind, l.Component)...)
	return upsert(t, in, a, existing, tracker.NewIssue{Title: "Recurring: sprint end " + r.Version,
		Description: SprintEndBody(r.Version, in.End), Labels: labels, DueDate: in.End, Milestone: r.Milestone})
}

// SprintEndBody is the sprint-end issue's description: its Requirements are
// the steps srd038 R1.2 lists, in order.
func SprintEndBody(version, end string) string {
	return fmt.Sprintf(`The end of release %[1]s's period, due %[2]s (srd038 R1).

## Requirements

1. Run the sprint-end skill in this repository's checkout; its facts step gives the period's aggregate, each objective issue's ledger and reported status, and the recommended version.
2. Model step (combine): draft the period's summary from the period's reports and that output.
3. Ask the owner whether anything in the summary, the objectives, or the release should change, and apply what the owner names in the worktree.
4. Commit the summary at docs/sprint-ends/%[1]s.md and merge it with the merge skill, listing in closes each objective the owner reports complete; the merge closes this issue.
5. Ask the owner to confirm the release and its version. With no confirmation, or nothing to release, stop: the merge alone closes this issue.
6. Once the summary merges, run the release skill on main with this issue as the release issue; it gates and tags.

## Acceptance Criteria

- The summary merges, and a confirmed release is tagged on main after a passing gate (test: integration_sprint_end; environment: dev)

## Recurrence

Once per period. The plan-push skill, publishing the next release, files the next sprint-end issue.

## Previous Runs

- none

%[3]s
`, version, end, sprintEndMarker(version))
}

// find returns the open issue in project carrying marker, if any.
func find(t tracker.Tracker, project, label, marker string) (*tracker.Issue, error) {
	issues, err := t.ListIssues(project, tracker.IssueQuery{State: "opened", Labels: []string{label}})
	if err != nil {
		return nil, err
	}
	for i := range issues {
		if strings.Contains(issues[i].Description, marker) {
			return &issues[i], nil
		}
	}
	return nil, nil
}

// upsert files in when there is no existing issue, else updates its
// description and due date when they differ.
func upsert(t tracker.Tracker, in PushInput, a IssueAction, existing *tracker.Issue, n tracker.NewIssue) (IssueAction, error) {
	if existing == nil {
		n.Labels = append(n.Labels, program.Op(program.OpPlan))
		r, err := program.File(t, a.Project, n, in.Policy, in.DryRun)
		if err != nil {
			return a, err
		}
		a.Action, a.IID, a.Admitted, a.Reasons = "filed", r.IID, r.Lint.Admitted, r.Lint.Reasons
		return a, nil
	}
	a.IID, a.Admitted = existing.IID, true
	// A tracker may store a description without its trailing newline, so
	// the comparison ignores the whitespace around it.
	if strings.TrimSpace(existing.Description) == strings.TrimSpace(n.Description) && existing.DueDate == n.DueDate {
		a.Action = "unchanged"
		return a, nil
	}
	a.Action = "updated"
	if in.DryRun {
		return a, nil
	}
	_, err := t.UpdateIssue(a.Project, existing.IID, tracker.IssueUpdate{Description: &n.Description, DueDate: &n.DueDate})
	return a, err
}

// postPlanned posts a planned event on each open Sprint Goal, or objective
// issue read as one, that serves an outcome of the release but was filed for
// another release (srd031 R4.3, srd006 R8). A goal that cites its items
// counts only those of the release's items it cites, so two goals serving
// one outcome do not each count the other's (GH-92).
func postPlanned(t tracker.Tracker, in PushInput, r Release, repos []string, docs []*objectives.Document, groups []group) ([]PlannedOnIssue, error) {
	// None planned prints [], not null (GH-2781).
	out := []PlannedOnIssue{}
	for _, repo := range repos {
		issues, err := t.ListIssues(repo, tracker.IssueQuery{State: "opened", Labels: []string{"plane::management"}})
		if err != nil {
			return out, err
		}
		for _, issue := range issues {
			m, ok := sprintgoal.Parse(issue.Description)
			if !ok || m.Release == r.Version {
				continue
			}
			var cites []string
			environment := ""
			promised := sprintgoal.CitedItems(issue.Description)
			pinned := sprintgoal.CitedURLs(issue.Description)
			for _, key := range m.Serves {
				j := slices.IndexFunc(groups, func(g group) bool { return g.key == key })
				if j < 0 {
					continue
				}
				g := groups[j]
				doc, _, _ := strings.Cut(g.key, "#")
				for _, item := range g.items {
					if len(promised) > 0 && !slices.Contains(promised, doc+"#"+item) {
						continue
					}
					// The goal's own pin, so the event and its goal name
					// the same text; the new plan's tag for an item the
					// goal does not cite (GH-101).
					if url, ok := pinned[doc+"#"+item]; ok {
						cites = append(cites, url)
					} else {
						cites = append(cites, goals.CiteURL(in.WebBase, in.System, r.Tag, g.docPath, item))
					}
				}
				if environment == "" {
					environment = g.environment
				}
			}
			if len(cites) == 0 {
				continue
			}
			e, text := events.Planned(r.Version, cites, environment)
			posted, err := events.Post(t, repo, issue.IID, e, events.PlannedRequired, text, in.DryRun)
			if err != nil {
				return out, err
			}
			out = append(out, PlannedOnIssue{Issue: fmt.Sprintf("%s#%d", repo, issue.IID), Posted: posted})
		}
	}
	return out, nil
}

// periodStart is the first working day of the period end closes: the
// Monday of the ISO week two weeks before end's, so a period ending on
// Friday of W45 runs from Monday of W43.
func periodStart(end time.Time) time.Time {
	weekday := (int(end.Weekday()) + 6) % 7 // Monday is 0
	return end.AddDate(0, 0, -weekday-(periodDays-7))
}

// isoWeek names the ISO week of a date, such as 2026-W43.
func isoWeek(d time.Time) string {
	y, w := d.ISOWeek()
	return fmt.Sprintf("%d-W%02d", y, w)
}
