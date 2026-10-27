// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package reports

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/events"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Signal kinds aggregate reports (srd018 R3.2).
const (
	SignalSharedBlocker  = "shared-blocker"
	SignalAgingBlocker   = "aging-blocker"
	SignalSlipped        = "slipped-commitment"
	SignalMissingReport  = "missing-report"
	SignalContradiction  = "tracker-contradiction"
	SignalUnreportedWork = "unreported-merged-work"
)

// Signal is one finding that no single report shows. It names components,
// blockers, Sprint Goals, and repositories, never a person (srd018 R3.4).
type Signal struct {
	Kind       string `json:"kind"`
	Component  string `json:"component"`
	Subject    string `json:"subject"`
	Week       string `json:"week"`
	Detail     string `json:"detail"`
	Actionable bool   `json:"actionable"`
	// Blocked names the components a shared blocker holds up.
	Blocked []string `json:"blocked,omitempty"`
}

// Key identifies a signal across runs, so a pushed signal is pushed once.
func (s Signal) Key() string {
	return strings.Trim(nonKey.ReplaceAllString(s.Kind+"/"+s.Component+"/"+s.Subject, "-"), "-")
}

var nonKey = regexp.MustCompile(`[^A-Za-z0-9._/#-]+`)

// Progress is one Sprint Goal's state for the range: its latest reported
// status and whether its ledger holds a met event (srd018 R3.5).
type Progress struct {
	SprintGoal string `json:"sprint_goal"`
	Component  string `json:"component"`
	Reported   string `json:"reported"`
	Complete   bool   `json:"reported_complete"`
	Met        bool   `json:"met"`
	State      string `json:"state"` // met, reported complete, or the reported status
}

// Aggregation is what scrum-master reports aggregate returns.
type Aggregation struct {
	From        string              `json:"from"`
	To          string              `json:"to"`
	Reports     int                 `json:"reports"`
	ByComponent map[string][]string `json:"by_component"` // component -> Sprint Goals and blockers
	Signals     []Signal            `json:"signals"`
	Progress    []Progress          `json:"progress"`
	Pushed      []PushResult        `json:"pushed,omitempty"`
	// Source says which reports were read, in place or central, and why
	// (ChooseSource, GH-2775).
	Source string `json:"source,omitempty"`
}

// AggregateInput is one run over a range of ISO weeks.
type AggregateInput struct {
	Dir      string // reports checkout
	Config   Config
	Policy   program.Policy
	From, To string // YYYY-Www, inclusive
	// Now is the clock a week must have started by to miss a report; the
	// zero time is the current time (GH-66).
	Now time.Time
	// Project, when set, narrows the aggregate to that repository's team:
	// only its members' reports are read and only they can miss one, so a
	// team's sprint end describes the team (GH-116).
	Project string
	// Collection, when set, is the reports to read in place of Dir's: a
	// level's reports collected from the repositories where they were
	// written. Only the roster's people who report in its repositories can
	// miss one, and merged work is read in each of them (GH-2725).
	Collection *Collection
}

// ParseWeek reads YYYY-Www as the Monday that starts it.
func ParseWeek(w string) (time.Time, error) {
	var year, week int
	if _, err := fmt.Sscanf(w, "%d-W%d", &year, &week); err != nil || week < 1 || week > 53 {
		return time.Time{}, fmt.Errorf("week %q is not YYYY-Www", w)
	}
	return WeekStart(time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)).AddDate(0, 0, 7*(week-1)), nil
}

type weekly struct {
	week string
	r    Report
}

// Aggregate reads every report in the range, groups their items by
// component, computes the signals, and states each Sprint Goal's progress
// (srd018 R3.1, R3.2, R3.4, R3.5).
func Aggregate(t tracker.Tracker, in AggregateInput) (Aggregation, error) {
	from, err := ParseWeek(in.From)
	if err != nil {
		return Aggregation{}, err
	}
	to, err := ParseWeek(in.To)
	if err != nil {
		return Aggregation{}, err
	}
	if to.Before(from) {
		return Aggregation{}, fmt.Errorf("the range ends at %s before it starts at %s", in.To, in.From)
	}
	var weeks []string
	for w := from; !w.After(to); w = w.AddDate(0, 0, 7) {
		weeks = append(weeks, ISOWeek(w))
	}
	a := Aggregation{From: in.From, To: in.To, ByComponent: map[string][]string{}, Signals: []Signal{}, Progress: []Progress{}}
	if in.Project != "" {
		in.Config.Team = slices.DeleteFunc(slices.Clone(in.Config.Team), func(m Member) bool { return m.Project != in.Project })
	}
	if in.Collection != nil {
		// A person in a sub-program reports to its rollup, not here.
		in.Config.Team = slices.DeleteFunc(slices.Clone(in.Config.Team), func(m Member) bool {
			p := program.ProjectPath(m.Project)
			return !slices.Contains(in.Collection.Repositories, p) || slices.Contains(in.Collection.SubPrograms, p)
		})
	}
	member := func(user string) bool {
		return in.Project == "" || slices.ContainsFunc(in.Config.Team, func(m Member) bool { return m.Username == user })
	}

	var reports []weekly
	if in.Collection != nil {
		for _, f := range in.Collection.Reports {
			r, err := ParseReport(f.Repository+":"+f.Path, []byte(f.Content))
			if err != nil || !slices.Contains(weeks, r.FrontMatter.Sprint) || !member(r.FrontMatter.Author) {
				continue
			}
			reports = append(reports, weekly{r.FrontMatter.Sprint, r})
		}
	} else {
		err = filepath.WalkDir(filepath.Join(in.Dir, "reports"), func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			r, err := ParseReport(p, b)
			if err != nil || !slices.Contains(weeks, r.FrontMatter.Sprint) || !member(r.FrontMatter.Author) {
				return nil
			}
			reports = append(reports, weekly{r.FrontMatter.Sprint, r})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return a, err
		}
	}
	sort.SliceStable(reports, func(i, j int) bool { return reports[i].week < reports[j].week })
	a.Reports = len(reports)

	add := func(component, item string) {
		if !slices.Contains(a.ByComponent[component], item) {
			a.ByComponent[component] = append(a.ByComponent[component], item)
		}
	}
	type history struct {
		component string
		statuses  []string // per week in order, "" where unreported
		complete  bool
		latest    string
	}
	goals := map[string]*history{}
	blockedBy := map[string]map[string]bool{} // week/waiting-on -> components
	for _, wr := range reports {
		for _, e := range wr.r.Work {
			add(e["component"], e["sprint_goal"])
			h := goals[e["sprint_goal"]]
			if h == nil {
				h = &history{component: e["component"], statuses: make([]string, len(weeks))}
				goals[e["sprint_goal"]] = h
			}
			h.statuses[slices.Index(weeks, wr.week)] = e["status"]
			h.latest, h.complete = e["status"], e["complete"] == "yes"
		}
		for _, e := range wr.r.Blockers {
			add(e["component"], "blocker: "+e["blocker"])
			k := wr.week + "\x00" + e["waiting-on"]
			if blockedBy[k] == nil {
				blockedBy[k] = map[string]bool{}
			}
			blockedBy[k][e["component"]] = true
			if since, err := time.Parse("2006-01-02", e["since"]); err == nil {
				end, _ := ParseWeek(wr.week)
				if age := int(end.AddDate(0, 0, 6).Sub(since).Hours() / 24); age > in.Policy.Escalation.BlockerAgeDays {
					a.Signals = append(a.Signals, Signal{SignalAgingBlocker, e["component"], e["blocker"], wr.week,
						fmt.Sprintf("%s has been blocked by %q for %d days, past the %d-day threshold", e["component"], e["blocker"], age, in.Policy.Escalation.BlockerAgeDays), true, nil})
				}
			}
		}
	}
	for k, comps := range blockedBy {
		week, waiting, _ := strings.Cut(k, "\x00")
		if countReports(reports, week, waiting) < 2 {
			continue
		}
		names := slices.DeleteFunc(sortedKeys(comps), func(c string) bool { return c == waiting })
		text := fmt.Sprintf("%s blocks %s in %d reports", waiting, strings.Join(names, ", "), countReports(reports, week, waiting))
		if len(names) == 0 {
			text = fmt.Sprintf("%s reports a blocker in its own work in %d reports", waiting, countReports(reports, week, waiting))
		}
		a.Signals = append(a.Signals, Signal{SignalSharedBlocker, waiting, waiting, week, text, true, names})
	}

	for _, ref := range sortedKeys(goals) {
		h := goals[ref]
		run := 0
		for _, s := range h.statuses {
			if s == "at-risk" || s == "blocked" {
				run++
			} else if s != "" {
				run = 0
			}
		}
		if limit := in.Policy.Escalation.SlipReleases; limit > 0 && run >= limit {
			a.Signals = append(a.Signals, Signal{SignalSlipped, h.component, ref, in.To,
				fmt.Sprintf("Sprint Goal %s (%s) has been at risk or blocked in %d consecutive reports", ref, h.component, run), true, nil})
		}
		met, err := hasEvent(t, ref, "met")
		if err != nil {
			return a, err
		}
		closedComplete, err := hasEvent(t, ref, "reported_complete")
		if err != nil {
			return a, err
		}
		p := Progress{SprintGoal: ref, Component: h.component, Reported: h.latest, Complete: h.complete || closedComplete, Met: met, State: h.latest}
		switch {
		case met:
			p.State = "met"
		case closedComplete:
			// A sprint-end merge closed it as reported complete, after the
			// last report that named its status (srd038 R2.2).
			p.State = "reported complete"
		case h.complete || h.latest == "done":
			p.State = "reported complete"
			// Closed without met, the Sprint Goal closed as reported complete
			// by its repository's sprint-end merge (srd038 R2); still open,
			// the report contradicts the tracker.
			closed, err := isClosed(t, ref)
			if err != nil {
				return a, err
			}
			if !closed {
				a.Signals = append(a.Signals, Signal{SignalContradiction, h.component, ref, in.To,
					fmt.Sprintf("Sprint Goal %s (%s) is reported complete but its ledger has no met event", ref, h.component), true, nil})
			}
		}
		a.Progress = append(a.Progress, p)
	}

	have := map[string]bool{}
	for _, wr := range reports {
		have[wr.week+"\x00"+wr.r.FrontMatter.Author] = true
	}
	// A week that has not started yet cannot miss a report (GH-66).
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	// Nor can a week that ended before the reports repository existed:
	// reporting had not begun (GH-116). A week ends as the next begins, so
	// one whose next Monday is reporting's first day is over (GH-141).
	began := reportingBegan(in.Dir)
	if in.Collection != nil {
		began = in.Collection.began()
	}
	var started []string
	for _, w := range weeks {
		if start, err := ParseWeek(w); err == nil && !start.After(now) && start.AddDate(0, 0, 7).After(began) {
			started = append(started, w)
		}
	}
	for _, m := range in.Config.Team {
		for _, w := range started {
			if !have[w+"\x00"+m.Username] {
				detail := fmt.Sprintf("no status report for %s in %s", m.Project, w)
				if open := in.Collection.openStatus(m.Project, m.Username, w); open != "" {
					detail += "; its status issue " + open + " is open"
				}
				a.Signals = append(a.Signals, Signal{SignalMissingReport, m.Project, m.Project, w, detail, false, nil})
			}
		}
	}
	// A team whose reports name no Sprint Goal of its own is as missing as
	// one that sent none: nothing of it reaches the aggregate. The system
	// repository holds no Sprint Goal (srd018 R3.2).
	var projects []string
	sent := map[string]bool{}
	for _, m := range in.Config.Team {
		if !slices.Contains(projects, m.Project) {
			projects = append(projects, m.Project)
		}
		for _, w := range weeks {
			sent[m.Project] = sent[m.Project] || have[w+"\x00"+m.Username]
		}
	}
	for _, project := range projects {
		if project == in.Config.System || !sent[project] {
			continue
		}
		named := false
		for ref := range goals {
			if strings.HasPrefix(ref, project+"#") {
				named = true
				break
			}
		}
		if !named {
			a.Signals = append(a.Signals, Signal{SignalMissingReport, project, project, weeks[len(weeks)-1],
				fmt.Sprintf("no report from %s to %s names a Sprint Goal of %s", in.From, in.To, project), false, nil})
		}
	}

	mentioned := map[string]bool{}
	for ref := range goals {
		mentioned[ref] = true
	}
	end := to.AddDate(0, 0, 6).Format("2006-01-02")
	var reported []tracker.Issue
	// backlog holds project#story for every story a reported Sprint Goal's
	// Sprint Backlog lists: a merge delivering one is the goal's work.
	backlog := map[string]bool{}
	for ref := range goals {
		if i, ok, err := issueOf(t, ref); err != nil {
			return a, err
		} else if ok {
			reported = append(reported, i)
			project, _, _ := strings.Cut(ref, "#")
			for _, s := range sprintgoal.SprintBacklog(i.Description) {
				backlog[project+"#"+s] = true
			}
		}
	}
	since := periodStart(from, reported, sprintgoal.Is).Format(time.RFC3339)
	if in.Collection != nil {
		projects = slices.Clone(in.Collection.Repositories)
	}
	for _, project := range projects {
		merged, err := t.Merges(project, "", since)
		if err != nil {
			return a, err
		}
		for _, mr := range merged {
			// A delivery closes an issue it cites; a merge citing none,
			// such as a pull request's, is not work the reports account for,
			// and neither is one closing only recurring issues, such as a
			// sprint-end or a status report.
			if mr.MergedAt > end || !strings.Contains(mr.Description, "Closes ") || closesMentioned(project, mr.Description, mentioned) {
				continue
			}
			if routine, err := closesOnlyRoutine(t, project, mr.Description); err != nil {
				return a, err
			} else if routine {
				continue
			}
			if goalWork, err := closesBacklogWork(t, project, mr.Description, backlog); err != nil {
				return a, err
			} else if goalWork {
				continue
			}
			a.Signals = append(a.Signals, Signal{SignalUnreportedWork, project, mr.Ref(project), ISOWeek(mustDate(mr.MergedAt)),
				fmt.Sprintf("%s %q merged on %s and closes no reported Sprint Goal, nor an issue that implements, or links to one that implements, a story on one's Sprint Backlog", mr.Ref(project), mr.Title, mr.MergedAt), false, nil})
		}
	}
	for c := range a.ByComponent {
		sort.Strings(a.ByComponent[c])
	}
	sort.SliceStable(a.Signals, func(i, j int) bool {
		if a.Signals[i].Kind != a.Signals[j].Kind {
			return a.Signals[i].Kind < a.Signals[j].Kind
		}
		return a.Signals[i].Key() < a.Signals[j].Key()
	})
	return a, nil
}

func countReports(reports []weekly, week, waiting string) int {
	n := 0
	for _, wr := range reports {
		if wr.week != week {
			continue
		}
		for _, e := range wr.r.Blockers {
			if e["waiting-on"] == waiting {
				n++
				break
			}
		}
	}
	return n
}

var closesPattern = regexp.MustCompile(`(?i)closes\s+(\S*#\d+)`)

// closesMentioned reports whether the merge closes a Sprint Goal a
// report names.
func closesMentioned(project, description string, mentioned map[string]bool) bool {
	for _, m := range closesPattern.FindAllStringSubmatch(description, -1) {
		ref := m[1]
		if strings.HasPrefix(ref, "#") {
			ref = project + ref
		}
		if mentioned[ref] {
			return true
		}
	}
	return false
}

// closesOnlyRoutine reports whether every issue the merge closes is
// routine or planning, not delivery the reports account for: a recurring
// one (type::recurring), such as a sprint-end or a status report, one
// on the management plane, such as writing a goal's stories (GH-24), or one
// that cuts a release (kind::release), which publishes delivered work and
// is none itself (GH-141).
func closesOnlyRoutine(t tracker.Tracker, project, description string) (bool, error) {
	refs := closesPattern.FindAllStringSubmatch(description, -1)
	for _, m := range refs {
		ref := m[1]
		if strings.HasPrefix(ref, "#") {
			ref = project + ref
		}
		i, ok, err := issueOf(t, ref)
		if err != nil || !ok || !(i.HasLabel("type::recurring") || i.HasLabel("plane::management") || i.HasLabel("kind::release")) {
			return false, err
		}
	}
	return len(refs) > 0, nil
}

// closesBacklogWork reports whether the merge closes an issue that
// implements a story on a reported Sprint Goal's Sprint Backlog.
func closesBacklogWork(t tracker.Tracker, project, description string, backlog map[string]bool) (bool, error) {
	for _, m := range closesPattern.FindAllStringSubmatch(description, -1) {
		ref := m[1]
		if strings.HasPrefix(ref, "#") {
			ref = project + ref
		}
		i, ok, err := issueOf(t, ref)
		if err != nil {
			return false, err
		}
		if !ok {
			continue
		}
		owner, _, _ := strings.Cut(ref, "#")
		if implementsBacklog(owner, i.Description, backlog) {
			return true, nil
		}
		// Work that links to an issue implementing a backlog story, such as
		// a defect found in it, serves the same goal (GH-24).
		for _, l := range links.ParseLinks(i.Description) {
			if !slices.Contains(servingLinks, l.Kind) {
				continue
			}
			target := strings.TrimSpace(l.Target)
			if strings.HasPrefix(target, "#") {
				target = owner + target
			}
			linked, ok, err := issueOf(t, target)
			if err != nil {
				return false, err
			}
			if ok && implementsBacklog(linked.Project, linked.Description, backlog) {
				return true, nil
			}
		}
	}
	return false, nil
}

// servingLinks are the link kinds by which an issue serves the issue it
// names.
var servingLinks = []string{links.RelatesTo, links.BlockedBy, links.Blocks, links.DependsOn}

// implementsBacklog reports whether an issue of project, by its
// description, implements a story on a reported Sprint Goal's Sprint
// Backlog.
func implementsBacklog(project, description string, backlog map[string]bool) bool {
	for _, l := range links.ParseLinks(description) {
		if l.Kind != links.Implements {
			continue
		}
		story, _, _ := strings.Cut(strings.TrimSpace(l.Target), "#")
		if backlog[project+"#"+strings.TrimSuffix(path.Base(story), ".md")] {
			return true
		}
	}
	return false
}

// hasMet reports whether the Sprint Goal's ledger holds a met event
// (srd006 R10).
// isClosed reports whether the Sprint Goal ref is closed; an issue the
// tracker cannot find is not.
// issueOf reads the issue a project#iid reference names; ok is false when
// the reference is malformed or names no issue.
func issueOf(t tracker.Tracker, ref string) (tracker.Issue, bool, error) {
	project, n, ok := strings.Cut(ref, "#")
	iid, err := strconv.Atoi(n)
	if !ok || err != nil {
		return tracker.Issue{}, false, nil
	}
	i, err := t.GetIssue(project, iid)
	if errors.Is(err, tracker.ErrNotFound) {
		return tracker.Issue{}, false, nil
	}
	return i, err == nil, err
}

func isClosed(t tracker.Tracker, ref string) (bool, error) {
	i, ok, err := issueOf(t, ref)
	return ok && i.State == "closed", err
}

// hasEvent reports whether the issue ref's ledger holds an event named name.
func hasEvent(t tracker.Tracker, ref, name string) (bool, error) {
	project, n, ok := strings.Cut(ref, "#")
	iid, err := strconv.Atoi(n)
	if !ok || err != nil {
		return false, nil
	}
	notes, err := t.ListNotes(project, iid)
	if errors.Is(err, tracker.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, note := range notes {
		if e, ok := events.Parse(note.Body); ok && e.Name == name {
			return true, nil
		}
	}
	return false, nil
}

func mustDate(s string) time.Time {
	d, _ := time.Parse("2006-01-02", s)
	return d
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// reportingBegan is the day, in UTC as the weeks are, of the reports
// checkout's first commit, when reporting began; zero when dir is no
// repository or has no commit.
func reportingBegan(dir string) time.Time {
	out, err := exec.Command("git", "-C", dir, "log", "--reverse", "--format=%ct").Output()
	if err != nil {
		return time.Time{}
	}
	first, _, _ := strings.Cut(string(out), "\n")
	secs, err := strconv.ParseInt(strings.TrimSpace(first), 10, 64)
	if err != nil {
		return time.Time{}
	}
	y, m, d := time.Unix(secs, 0).UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// began is the day reporting began in the collection: the Monday of the
// earliest week a report was merged or a status issue opened for, so a
// week before it misses no report; zero when it holds neither.
func (c *Collection) began() time.Time {
	first := ""
	for _, f := range c.Reports {
		if first == "" || f.Week < first {
			first = f.Week
		}
	}
	for _, o := range c.Open {
		if o.Kind == "status" && (first == "" || o.Week < first) {
			first = o.Week
		}
	}
	start, err := ParseWeek(first)
	if err != nil {
		return time.Time{}
	}
	return start
}

// openStatus names the open status issue for user's week in repo, as
// repo#iid; empty when there is none or no collection.
func (c *Collection) openStatus(repo, user, week string) string {
	if c == nil {
		return ""
	}
	for _, o := range c.Open {
		if o.Kind == "status" && o.Name == user && o.Week == week && o.Repository == program.ProjectPath(repo) {
			return fmt.Sprintf("%s#%d", o.Repository, o.IID)
		}
	}
	return ""
}
