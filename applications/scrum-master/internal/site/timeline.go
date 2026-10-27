// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package site

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/checkout"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/repofiles"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// SystemDir is the system repository's checkout beside the reports
// checkout dir: the one holding control/trackers.md, as every word finds it.
func SystemDir(dir string) (string, error) {
	// Absolute, so a reports checkout named as . still has a parent whose
	// other checkouts are its siblings.
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	trackers, err := checkout.FindTrackers(dir)
	if err != nil {
		return "", err
	}
	if trackers == "" {
		return "", fmt.Errorf("no system repository's checkout beside %s holds %s; check it out beside the reports repository", dir, checkout.TrackersFile)
	}
	return filepath.Dir(filepath.Dir(trackers)), nil
}

// TimelineSources are the inputs of a timeline besides the documents and
// road map, shared by the working tree's facts and each tag's.
type TimelineSources struct {
	Today  time.Time
	Done   map[string]string
	Landed map[string]string
	Goals  []timeline.Goal
	Issues []timeline.Issue
	// Group is the program group whose milestones date a release named by
	// title; Tracker is nil when none is reachable.
	Group   string
	Tracker tracker.Tracker
	// Notes say what the facts could not read.
	Notes []string
	// Teams are the team repositories whose documents roll up to the
	// system's: those the registry names, read through the tracker, else
	// the checkouts beside the system's (GH-2726). scratch holds the ones
	// read from a host; Close removes it.
	Teams   []backlog.TeamSource
	scratch string
	// Scrums are the program's team subsystems, from the registry the
	// tracker reads, else the system checkout's SRDs.
	Scrums program.Subsystems
}

// Close removes what LoadSources read into a scratch directory.
func (s TimelineSources) Close() {
	if s.scratch != "" {
		os.RemoveAll(s.scratch)
	}
}

// LoadSources reads the system checkout's sprint-end summaries, the day
// each merged, through local git, and, when a tracker is reachable, the
// Sprint Goals of the program's repositories (GH-82).
func LoadSources(system string, t tracker.Tracker, today time.Time) (TimelineSources, error) {
	s := TimelineSources{Today: today, Landed: map[string]string{}, Tracker: t}
	done, err := sprintgoal.StoriesDone(system)
	if err != nil {
		return s, err
	}
	s.Done = done.Release
	for _, v := range done.Release {
		if _, ok := s.Landed[v]; ok {
			continue
		}
		out, err := exec.Command("git", "-C", system, "log", "--diff-filter=A", "--format=%cs", "--", path.Join(sprintgoal.SummaryDir, v+".md")).Output()
		if lines := strings.Fields(string(out)); err == nil && len(lines) > 0 {
			s.Landed[v] = lines[len(lines)-1]
		}
	}
	projects, err := Projects(system)
	if err != nil {
		return s, err
	}
	if len(projects) > 0 && projects[0] != "" {
		s.Group = path.Dir(projects[0])
	}
	if s.scratch, err = os.MkdirTemp("", "scrum-master-teams-"); err != nil {
		return s, err
	}
	var unread []repofiles.Unread
	if s.Teams, unread, err = backlog.Sources(t, system, "main", s.scratch); err != nil {
		return s, err
	}
	for _, u := range unread {
		s.Notes = append(s.Notes, fmt.Sprintf("%s: not read, so its stories are left out: %s", u.Repository, u.Reason))
	}
	s.Scrums = scrumsOf(system, t)
	// The registry the tracker reads names the team repositories whose
	// Sprint Goals the scrums file, whether or not the system checkout's
	// own SRDs do (GH-2726).
	for _, r := range s.Scrums.Repositories() {
		if p := program.ProjectPath(r); p != "" && !contains(projects, p) {
			projects = append(projects, p)
		}
	}
	if t == nil {
		s.Notes = append(s.Notes, "no tracker reachable: Sprint Goals and milestones named by title are left out")
		return s, nil
	}
	for n, p := range projects {
		if p == "" {
			continue
		}
		// A team's issues name its own stories, which the facts name
		// <checkout>/<id> (GH-109); the system's name the system's.
		story := func(id string) string { return id }
		if n > 0 {
			story = func(id string) string { return path.Base(p) + "/" + id }
		}
		issues, err := t.ListIssues(p, tracker.IssueQuery{State: "all"})
		if err != nil {
			s.Notes = append(s.Notes, fmt.Sprintf("%s: %v", p, err))
			continue
		}
		for _, i := range issues {
			m, ok := sprintgoal.Parse(i.Description)
			if !ok {
				implements := ""
				var deps []string
				for _, l := range links.ParseLinks(i.Description) {
					switch l.Kind {
					case links.Implements:
						target, _, _ := strings.Cut(l.Target, "#")
						implements = story(strings.TrimSuffix(path.Base(target), ".md"))
					case links.DependsOn, links.BlockedBy:
						// A bare #n is in the issue's own project (GH-135).
						ref := strings.TrimSpace(l.Target)
						if strings.HasPrefix(ref, "#") {
							ref = p + ref
						}
						deps = append(deps, ref)
					}
				}
				s.Issues = append(s.Issues, timeline.Issue{Ref: fmt.Sprintf("%s#%d", p, i.IID), Title: i.Title, Milestone: i.Milestone,
					Story: implements, Created: i.CreatedAt, Closed: i.ClosedAt, URL: i.WebURL, DependsOn: deps})
				continue
			}
			s.Goals = append(s.Goals, timeline.Goal{Ref: fmt.Sprintf("%s#%d", p, i.IID), Title: i.Title, Release: m.Release,
				Created: i.CreatedAt, Closed: i.ClosedAt, URL: i.WebURL, Stories: mapped(sprintgoal.SprintBacklog(i.Description), story)})
		}
	}
	return s, nil
}

func mapped(ids []string, f func(string) string) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = f(id)
	}
	return out
}

// Projects are the program's repositories as project paths: the system
// repository's own first, from its origin, empty when it has none, then each
// component repository its subsystem SRDs declare.
func Projects(system string) ([]string, error) {
	origin, _ := exec.Command("git", "-C", system, "config", "--get", "remote.origin.url").Output()
	projects := []string{program.ProjectPath(strings.TrimSpace(string(origin)))}
	subs, err := program.SubsystemsIn(system)
	if err != nil {
		return nil, err
	}
	for _, r := range subs.Repositories() {
		if p := program.ProjectPath(r); p != "" && !contains(projects, p) {
			projects = append(projects, p)
		}
	}
	return projects, nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// Releases reads a road map's releases with their sprints: the ISO weeks
// its milestone names, else the dates of the program's milestone of that
// title, when a tracker is reachable.
func Releases(roadMap []byte, s TimelineSources) []timeline.Release {
	d, err := corpus.ParseMarkdown(roadMap)
	if err != nil {
		return nil
	}
	var out []timeline.Release
	for _, r := range d.Records("release") {
		publication := r.Field("release").Field("publication").Tree()
		rel := timeline.Release{Version: r.Key, Status: r.Attrs["status"], Tag: corpus.TreeString(publication, "tag"),
			Milestone: corpus.TreeString(publication, "milestone"), Items: r.Field("deliverables").List()}
		if start, end, err := timeline.ParseWeekRange(rel.Milestone); err == nil {
			rel.Sprint = &timeline.Span{Start: start.Format(time.DateOnly), End: end.Format(time.DateOnly)}
		} else if rel.Milestone != "" && s.Tracker != nil && s.Group != "" {
			if m, err := s.Tracker.FindMilestone(s.Group, rel.Milestone); err == nil && m.StartDate != "" && m.DueDate != "" {
				rel.Sprint = &timeline.Span{Start: m.StartDate, End: m.DueDate}
			}
		}
		out = append(out, rel)
	}
	return out
}

// Facts is the timeline of the system checkout's corpus in dir, the working
// tree's or a tag's materialized there. The working tree's adds the team
// checkouts' documents beside it under the documents they serve, named
// <checkout>/<id> (GH-109); a tag's is the system's plan as it stood.
func Facts(dir, asOf string, s TimelineSources) (timeline.Facts, error) {
	docs, err := hierarchy.Load(dir)
	if err != nil {
		return timeline.Facts{}, err
	}
	if asOf == "" {
		team, done := backlog.TeamDocs(s.Teams, docs)
		docs = append(docs, team...)
		if len(done) > 0 {
			s.Done = maps.Clone(s.Done)
			if s.Done == nil {
				s.Done = map[string]string{}
			}
			maps.Copy(s.Done, done)
		}
	}
	roadMap, err := os.ReadFile(filepath.Join(dir, "docs", "road-map.md"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return timeline.Facts{}, err
	}
	sources, paths := docSources(docs)
	f := timeline.Make(timeline.Input{AsOf: asOf, Today: s.Today, Docs: docs, Releases: Releases(roadMap, s),
		Done: s.Done, Landed: s.Landed, Goals: s.Goals, Issues: s.Issues, Sources: sources, Paths: paths})
	if asOf == "" {
		f.Teams = teamFacts(s.Scrums)
	}
	return f, nil
}

// scrumsOf is the program's team subsystems: the registry the tracker
// reads, else the system checkout's own SRDs.
func scrumsOf(system string, t tracker.Tracker) program.Subsystems {
	control := program.SystemControl(system)
	if t != nil && control != "" {
		if teams, err := program.Teams(t, control, "main"); err == nil && len(teams) > 0 {
			return teams
		}
	}
	subs, err := program.SubsystemsIn(system)
	if err != nil {
		return nil
	}
	if control == "" {
		control = filepath.Base(system)
	}
	return subs.Teams(control)
}

// teamFacts are the scrums as the facts carry them, each named by its
// first repository, as its documents are.
func teamFacts(subs program.Subsystems) []timeline.Team {
	var out []timeline.Team
	for _, sub := range subs {
		tm := timeline.Team{Subsystem: sub.ID, Owner: sub.Owner}
		for _, c := range sub.Components {
			tm.Components = append(tm.Components, c.Name)
			if r := program.ProjectPath(c.Repository); r != "" && !slices.Contains(tm.Repositories, r) {
				tm.Repositories = append(tm.Repositories, r)
				if tm.URL == "" && (strings.HasPrefix(c.Repository, "https://") || strings.HasPrefix(c.Repository, "http://")) {
					tm.URL = strings.TrimSuffix(c.Repository, ".git")
				}
			}
		}
		if len(tm.Repositories) == 0 {
			continue
		}
		tm.Name = path.Base(tm.Repositories[0])
		out = append(out, tm)
	}
	return out
}

// docSources maps each document to its page on the forge of the repository
// holding it, the system's or a team's, from that checkout's configured
// origin, and to its path within that repository (GH-126). A document
// outside a checkout, as a tag's snapshot is, has neither; one in a
// checkout whose origin no browser opens has a path alone.
func docSources(docs []*hierarchy.Doc) (sources, paths map[string]string) {
	sources, paths = map[string]string{}, map[string]string{}
	roots := map[string]string{}
	for _, d := range docs {
		dir, err := filepath.Abs(filepath.Dir(d.Path))
		if err != nil {
			continue
		}
		root, ok := roots[dir]
		if !ok {
			if b, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output(); err == nil {
				root, _ = filepath.EvalSymlinks(strings.TrimSpace(string(b)))
			}
			roots[dir] = root
		}
		if root == "" {
			continue
		}
		abs, err := filepath.EvalSymlinks(d.Path)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			continue
		}
		paths[d.ID] = filepath.ToSlash(rel)
		if base := blobBase(root); base != "" {
			sources[d.ID] = base + filepath.ToSlash(rel)
		}
	}
	return sources, paths
}

// CorpusTags lists the system checkout's corpus release tags, oldest first,
// from local git alone: the tags the policy's tag format makes for the
// corpus artifact (GH-82).
func CorpusTags(system string) ([]string, error) {
	format := program.Defaults().Releases.TagFormat
	if data, err := os.ReadFile(filepath.Join(system, program.PolicyFile)); err == nil {
		if f, err := program.TagFormatIn(data); err == nil {
			format = f
		}
	}
	match := strings.NewReplacer("{artifact}", "corpus", "{version}", "[0-9]*").Replace(format)
	out, err := exec.Command("git", "-C", system, "tag", "--list", "--sort=creatordate", match).Output()
	if err != nil {
		return nil, fmt.Errorf("list the corpus tags of %s: %w", system, err)
	}
	return strings.Fields(string(out)), nil
}

// ProgramTags are the corpus tags of this program, oldest first: every
// corpus tag from the oldest its road map names on, so the tags of an
// earlier program the repository still holds drop out while the plan tags
// between this program's releases stay (GH-115). With no road map, or one
// naming no tag, there is nothing to scope by, and every corpus tag is the
// program's.
func ProgramTags(system string) ([]string, error) {
	tags, err := CorpusTags(system)
	if err != nil {
		return nil, err
	}
	roadMap, err := os.ReadFile(filepath.Join(system, "docs", "road-map.md"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	named := map[string]bool{}
	for _, r := range Releases(roadMap, TimelineSources{}) {
		if r.Tag != "" {
			named[r.Tag] = true
		}
	}
	for i, t := range tags {
		if named[t] {
			return tags[i:], nil
		}
	}
	return tags, nil
}

// CacheEnv names where the site keeps each tag's corpus between its words,
// in place of the system checkout's git directory.
const CacheEnv = "SCRUM_MASTER_SITE_CACHE"

// AtTag is the system checkout's docs/ as tag held it, read through
// textconv so an encrypted corpus reads as text. A tag's corpus never
// changes, so it is kept by commit, and by the textconv filters that read
// it, in the user cache directory: the timeline and history words, which a
// run starts as separate processes, read each tag once (GH-96). The
// function returned releases it.
func AtTag(system, tag string) (string, func(), error) {
	commit, err := exec.Command("git", "-C", system, "rev-parse", tag+"^{commit}").Output()
	if err != nil {
		return "", nil, fmt.Errorf("read %s at %s: %w", system, tag, err)
	}
	filters, _ := exec.Command("git", "-C", system, "config", "--get-regexp", `^diff\..*\.textconv$`).Output()
	// By default the snapshots, decrypted where the repository encrypts
	// its documents, stay inside the system checkout's own git directory
	// rather than outliving it in the user cache (GH-115).
	root := os.Getenv(CacheEnv) //nolint:forbidigo // Reads the documented SCRUM_MASTER_* environment interface.
	if root == "" {
		common, err := exec.Command("git", "-C", system, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
		if err != nil {
			return atTag(system, tag)
		}
		root = filepath.Join(strings.TrimSpace(string(common)), "scrum-master", "site")
	}
	key := strings.TrimSpace(string(commit)) + "-" + digest(string(filters))[:12]
	cached := filepath.Join(root, key)
	if _, err := os.Stat(filepath.Join(cached, ".complete")); err == nil {
		return cached, func() {}, nil
	}
	dir, cleanup, err := atTag(system, tag)
	if err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, ".complete"), nil, 0o644); err == nil && os.MkdirAll(root, 0o755) == nil && os.Rename(dir, cached) == nil {
		return cached, func() {}, nil
	}
	// Another run cached it first, or the cache cannot be written: the
	// temporary copy serves this run.
	return dir, cleanup, nil
}

// atTag writes the corpus at tag into a temporary directory.
func atTag(system, tag string) (string, func(), error) {
	tmp, err := os.MkdirTemp("", "scrum-master-site-tag-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { os.RemoveAll(tmp) }
	out, err := exec.Command("git", "-C", system, "ls-tree", "-r", "--name-only", tag, "--", "docs").Output()
	if err != nil {
		cleanup()
		return "", nil, fmt.Errorf("read %s at %s: %w", system, tag, err)
	}
	for _, p := range strings.Fields(string(out)) {
		b, err := exec.Command("git", "-C", system, "cat-file", "--textconv", tag+":"+p).Output()
		if err != nil {
			cleanup()
			return "", nil, fmt.Errorf("read %s at %s: %w", p, tag, err)
		}
		dst := filepath.Join(tmp, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			cleanup()
			return "", nil, err
		}
		if err := os.WriteFile(dst, b, 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
	}
	return tmp, cleanup, nil
}

// TimelinePages are the timeline word's pages: the working tree's facts,
// each corpus tag's as planned then, and an index page listing the items.
func TimelinePages(system string, s TimelineSources) (map[string][]byte, timeline.Facts, error) {
	now, err := Facts(system, "", s)
	if err != nil {
		return nil, now, err
	}
	pages := map[string][]byte{}
	b, _ := json.MarshalIndent(now, "", "  ")
	pages[Dir+"/timeline/facts.json"] = append(b, '\n')
	tags, err := ProgramTags(system)
	if err != nil {
		return nil, now, err
	}
	for _, tag := range tags {
		dir, cleanup, err := AtTag(system, tag)
		if err != nil {
			return nil, now, err
		}
		f, err := Facts(dir, tag, s)
		cleanup()
		if err != nil {
			return nil, now, err
		}
		b, _ := json.MarshalIndent(f, "", "  ")
		pages[Dir+"/timeline/facts@"+strings.ReplaceAll(tag, "/", "-")+".json"] = append(b, '\n')
	}
	pages[Dir+"/timeline/index.md"] = []byte(timelineIndex(now, tags))
	return pages, now, nil
}

// timelineIndex is the timeline folder's page: each item, its state, and its
// planned and actual days, until the gantt views draw them.
func timelineIndex(f timeline.Facts, tags []string) string {
	var b strings.Builder
	b.WriteString("# Timeline\n\nEach initiative, epic, feature, story, and Sprint Goal on the sprint axis: planned days, actual days, and state, as of " + f.Today + ". The facts are in [facts.json](facts.json)")
	if len(tags) > 0 {
		b.WriteString(", and as planned at each corpus tag in facts@<tag>.json")
	}
	b.WriteString(". The [gantt views](gantt/program.md) draw them.\n\n")
	b.WriteString(embed("timeline", "The program's interactive timeline") + "\n")
	b.WriteString("| item | kind | state | release | planned | actual | points |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	span := func(s *timeline.Span) string {
		if s == nil {
			return ""
		}
		if s.End == "" {
			return s.Start + " .."
		}
		return s.Start + " .. " + s.End
	}
	for _, it := range f.Items {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %d of %d |\n", it.ID, it.Kind, it.Status, it.Release, span(it.Planned), span(it.Actual), it.PointsDone, it.Points)
	}
	return b.String()
}
