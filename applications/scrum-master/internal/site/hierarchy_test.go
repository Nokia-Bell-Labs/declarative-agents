// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// hierarchyProgram is the status program with po-1's checkout beside it, a
// Sprint Goal of po-1 selecting its st001, and an issue implementing it, its
// timeline facts written into the reports checkout as the timeline word
// writes them.
func hierarchyProgram(t *testing.T) (reports, system string) {
	t.Helper()
	in, system := statusProgram(t)
	teamCheckout(t, system)
	team := filepath.Join(filepath.Dir(system), "po-1")
	if out, err := exec.Command("git", "-C", team, "remote", "add", "origin", "https://gitlab.example.com/g/po-1.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, out)
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "system", "srd010-delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(system, "docs", "specs", "software-requirements"), 0o755)
	if err := os.WriteFile(filepath.Join(system, "docs", "specs", "software-requirements", "srd010-delivery.md"), []byte(strings.ReplaceAll(string(b), "program/delivery/fleet", "g/po-1")), 0o644); err != nil {
		t.Fatal(err)
	}
	in.Tracker.CreateIssue("g/po-1", tracker.NewIssue{Title: "Sprint Goal 01.0, selected", Labels: []string{"plane::management", "op::plan"},
		Description: "Score one agent.\n\n" + sprintgoal.Marker("01.0", []string{"in001-reliable-agents#R1"}) + "\n\n## Sprint Backlog\n\n- st001-score-trajectories\n"})
	in.Tracker.CreateIssue("g/po-1", tracker.NewIssue{Title: "Grade one trajectory", Labels: []string{"plane::data", "kind::build"},
		Description: "Grade it.\n\nImplements: st001-score-trajectories\n"})
	s, err := LoadSources(system, in.Tracker, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	f, err := Facts(system, "", s)
	if err != nil {
		t.Fatal(err)
	}
	j, _ := json.Marshal(f)
	os.MkdirAll(filepath.Join(in.Reports, Dir, "timeline"), 0o755)
	if err := os.WriteFile(filepath.Join(in.Reports, Dir, "timeline", "facts.json"), j, 0o644); err != nil {
		t.Fatal(err)
	}
	return in.Reports, system
}

// Every planning document has a page, and every link between two pages is
// reciprocated: a parent's page links each child, and each child's page
// links its parent (GH-126).
func TestHierarchyPagesLinkBothWays(t *testing.T) {
	reports, _ := hierarchyProgram(t)
	pages, data, _, _, err := HierarchyPages(reports, nil)
	if err != nil {
		t.Fatal(err)
	}
	var nodes []Node
	if err := json.Unmarshal(data[Dir+"/data/hierarchy/hierarchy.json"], &nodes); err != nil {
		t.Fatal(err)
	}
	byID := map[string]Node{}
	for _, n := range nodes {
		byID[n.ID] = n
	}
	for _, id := range []string{"th001-dependable-agents", "in001-reliable-agents", "ep001-agent-evaluation", "ft001-grader", "st001-score-trajectories", "po-1/st001-score-trajectories"} {
		if _, ok := pages[Dir+"/"+HierarchyDir+"/"+id+".md"]; !ok {
			t.Errorf("no page for %s", id)
		}
	}
	checked := 0
	for _, n := range nodes {
		p, ok := byID[n.Parent]
		if !ok {
			continue
		}
		child, parent := string(pages[Dir+"/"+n.Page]), string(pages[Dir+"/"+p.Page])
		if !strings.Contains(child, "]("+relLink(n.Page, p.Page)+")") {
			t.Errorf("%s does not link its parent %s", n.ID, p.ID)
		}
		if !strings.Contains(parent, "]("+relLink(p.Page, n.Page)+")") {
			t.Errorf("%s does not link its child %s", p.ID, n.ID)
		}
		checked++
	}
	if checked < 5 {
		t.Fatalf("only %d parent links checked", checked)
	}
	// The team's story sits under the system epic it serves.
	if byID["po-1/st001-score-trajectories"].Parent != "ep001-agent-evaluation" {
		t.Errorf("the team story's parent: %q", byID["po-1/st001-score-trajectories"].Parent)
	}
	plan := string(pages[Dir+"/"+HierarchyDir+"/index.md"])
	theme, initiative, epic := strings.Index(plan, `href="th001-dependable-agents.md"`), strings.Index(plan, `class="card-title" href="in001-reliable-agents.md"`), strings.Index(plan, `class="subcard subcard-epic`)
	if theme < 0 || initiative < theme || epic < initiative {
		t.Errorf("the plan does not nest its epic in its initiative's card in its theme's band:\n%s", plan)
	}
}

// The plan's index is boards: the legend names every level, a theme is a
// band with a tab panel per initiative, each panel a kanban board of its
// epics in their state's column with what lies under each, and a document
// no theme holds sits on a board of its own; every card links its page
// beside the index (GH-2723).
func TestPlanIsBoards(t *testing.T) {
	reports, _ := hierarchyProgram(t)
	pages, _, _, _, err := HierarchyPages(reports, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := string(pages[Dir+"/"+HierarchyDir+"/index.md"])
	for _, want := range []string{
		`<div class="legend">`, `<span class="kind kind-theme">Theme</span> <span class="legend-what">A strategic area: the why · months to years</span>`,
		`<section class="band band-theme`, `<a class="band-title" href="th001-dependable-agents.md">Dependable agents</a>`,
		`<div class="tabbed" data-tabs="th001-dependable-agents">`, `<section class="tab-panel state-active" data-tab="Reliable agents the product teams can deploy" data-state="active"`,
		`<a class="card-title" href="in001-reliable-agents.md">Reliable agents the product teams can deploy</a>`,
		`<div class="board">`, `<div class="column column-future"><div class="column-head">Future <span class="column-count">`,
		`<div class="column column-done"><div class="column-head">Done <span class="column-count">`,
		`<a class="subcard subcard-epic`, `href="ep001-agent-evaluation.md"`, `<span class="subcard-title">Agent evaluation</span>`, "1 feature · 4 stories · 10 of 21 points",
		`<section class="band band-none">`, "Not under a theme",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("the plan lacks %q:\n%s", want, plan)
		}
	}
	// An HTML block ends at a blank line, and a line indented four spaces
	// after one is code: the cards hold neither.
	for _, block := range strings.Split(plan, "\n\n") {
		if strings.HasPrefix(block, "<section") && !strings.HasSuffix(strings.TrimSpace(block), "</section>") {
			t.Errorf("a band breaks across a blank line:\n%s", block)
		}
	}
	// The epic sits in its state's column, after the column's head.
	active := strings.Index(plan, `<div class="column column-active">`)
	epic := strings.Index(plan, `href="ep001-agent-evaluation.md"`)
	if active < 0 || epic < active || strings.Contains(plan[active+1:epic], `<div class="column `) {
		t.Errorf("the active epic is not in the Active column:\n%s", plan)
	}
}

// Each document's page names its level with its chip, and its breadcrumb
// links the plan and each document above it; its Down list names each
// child's level (GH-2723).
func TestPagesNameTheirLevel(t *testing.T) {
	reports, _ := hierarchyProgram(t)
	pages, _, _, _, err := HierarchyPages(reports, nil)
	if err != nil {
		t.Fatal(err)
	}
	story := string(pages[Dir+"/"+HierarchyDir+"/st001-score-trajectories.md"])
	for _, want := range []string{
		"<div class=\"crumbs\">\n\n[Plan](../hierarchy/index.md) › <span class=\"kind kind-theme\">Theme</span> [Dependable agents](../hierarchy/th001-dependable-agents.md) › <span class=\"kind kind-initiative\">Initiative</span>",
		"<span class=\"kind kind-feature\">Feature</span> [The grader](../hierarchy/ft001-grader.md)\n\n</div>",
		"<span class=\"kind kind-story\">Story</span> `st001-score-trajectories`",
	} {
		if !strings.Contains(story, want) {
			t.Errorf("the story's page lacks %q:\n%s", want, story)
		}
	}
	team := string(pages[Dir+"/"+HierarchyDir+"/po-1/st001-score-trajectories.md"])
	if !strings.Contains(team, "[Plan](../../hierarchy/index.md)") {
		t.Errorf("the team story's breadcrumb does not reach the plan:\n%s", team)
	}
	if epic := string(pages[Dir+"/"+HierarchyDir+"/ep001-agent-evaluation.md"]); !strings.Contains(epic, "- <span class=\"kind kind-feature\">Feature</span> [The grader](../hierarchy/ft001-grader.md): ") {
		t.Errorf("the epic's Down list does not name its child's level:\n%s", epic)
	}
}

// A story's page lists the Sprint Goal that selected it and the issue that
// implements it; the epic above it lists the goal too; and a team story's
// source is its own repository on the forge (GH-126).
func TestHierarchyStoryPageNamesItsGoalAndIssue(t *testing.T) {
	reports, _ := hierarchyProgram(t)
	pages, _, _, _, err := HierarchyPages(reports, nil)
	if err != nil {
		t.Fatal(err)
	}
	story := string(pages[Dir+"/"+HierarchyDir+"/po-1/st001-score-trajectories.md"])
	for _, want := range []string{"Sprint Goal 01.0, selected", "Grade one trajectory", "https://gitlab.example.com/g/po-1/-/blob/HEAD/docs/specs/stories/st001-score-trajectories.md", "> As a product team lead, I want score trajectories"} {
		if !strings.Contains(story, want) {
			t.Errorf("the team story's page lacks %q:\n%s", want, story)
		}
	}
	if epic := string(pages[Dir+"/"+HierarchyDir+"/ep001-agent-evaluation.md"]); !strings.Contains(epic, "Sprint Goal 01.0, selected") {
		t.Errorf("the epic does not name the goal working under it:\n%s", epic)
	}
}

// The navigator lists every planning document under the document it
// serves, a team's story under the system epic, and each repository with a
// checkout beside the reports checkout, linked on its forge when its
// origin opens in a browser (GH-127).
func TestSidebarListsThePlanAndTheRepositories(t *testing.T) {
	reports, system := hierarchyProgram(t)
	_, _, _, sidebar, err := HierarchyPages(reports, Repos(system, reports))
	if err != nil {
		t.Fatal(err)
	}
	nav := string(sidebar)
	for _, want := range []string{
		"- [Overview](/overview.md)",
		`  - [<span class="kb kb-theme" title="Theme">T</span> Dependable agents](/hierarchy/th001-dependable-agents.md "Dependable agents")`,
		`    - [<span class="kb kb-initiative" title="Initiative">I</span> Reliable agents the product teams can deploy](/hierarchy/in001-reliable-agents.md "Reliable agents the product teams can deploy")`,
		"(/hierarchy/po-1/st001-score-trajectories.md ",
		"  - [g/po-1](https://gitlab.example.com/g/po-1)",
		"  - system\n",
	} {
		if !strings.Contains(nav, want) {
			t.Errorf("the navigator lacks %q:\n%s", want, nav)
		}
	}
	epic := strings.Index(nav, "(/hierarchy/ep001-agent-evaluation.md ")
	team := strings.Index(nav, "(/hierarchy/po-1/st001-score-trajectories.md ")
	if epic < 0 || team < epic {
		t.Errorf("the team story is not under the epic:\n%s", nav)
	}
}

// The overview embeds the interactive timeline and has one tile per
// initiative, with its points and a link to its page; the releases follow
// (GH-128).
func TestOverviewHasTheTimelineAndATilePerInitiative(t *testing.T) {
	reports, _ := hierarchyProgram(t)
	b, err := OverviewPage(reports, "g/system")
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	for _, want := range []string{
		"# g/system\n",
		`<iframe class="timeline" src="timeline/gantt/program.html?embed=1&view=timeline"`,
		`<a class="tile state-`, `href="hierarchy/in001-reliable-agents.md"`,
		`<span class="tile-title">Reliable agents the product teams can deploy</span>`,
		"| release | status | sprint | tag |",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the overview lacks %q:\n%s", want, page)
		}
	}
	if n := strings.Count(page, `<a class="tile `); n != 2 {
		t.Errorf("%d tiles, want one per initiative (in001 and obj001)", n)
	}
	if !regexp.MustCompile(`<span class="tile-points">\d+ of \d+ points</span>`).MatchString(page) {
		t.Errorf("the tiles carry no points:\n%s", page)
	}
}

// The scaffold lays down a placeholder overview, so the shell's home opens
// before the overview word has run, and the word's page replaces it.
func TestScaffoldLaysDownTheOverview(t *testing.T) {
	dir := t.TempDir()
	r, err := Scaffold(dir, "v1", false)
	if err != nil {
		t.Fatal(err)
	}
	if actions(r.Pages)[Overview] != "created" {
		t.Fatalf("pages %v", actions(r.Pages))
	}
	if p, err := Write(dir, Overview, []byte("# g/system\n"), "v1", false); err != nil || p.Action != "refreshed" {
		t.Fatalf("the word's overview: %+v, %v", p, err)
	}
}

// The reports checkout given as ".", as the site's words take it from the
// working directory, is listed once, under its own name (GH-127).
func TestReposListsTheReportsCheckoutOnce(t *testing.T) {
	_, system := hierarchyProgram(t)
	reports := filepath.Join(filepath.Dir(system), "reports")
	if out, err := exec.Command("git", "init", "-q", reports).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	t.Chdir(reports)
	var names []string
	for _, r := range Repos(system, ".") {
		names = append(names, r.Name)
	}
	if strings.Count(strings.Join(names, " "), "reports") != 1 || slices.Contains(names, ".") {
		t.Fatalf("repositories %v", names)
	}
}

// A checkout beside the system repository that the program does not declare
// is someone else's and stays off the navigator, and a reports worktree is
// named by its repository, not its folder (GH-141).
func TestReposListsOnlyTheProgramsRepositories(t *testing.T) {
	_, system := hierarchyProgram(t)
	parent := filepath.Dir(system)
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	elsewhere := filepath.Join(parent, "elsewhere")
	git("init", "-q", elsewhere)
	git("-C", elsewhere, "remote", "add", "origin", "https://github.com/other/elsewhere.git")
	reports := filepath.Join(parent, "reports")
	git("init", "-q", "-b", "main", reports)
	git("-C", reports, "commit", "-q", "--allow-empty", "-m", "start")
	worktree := filepath.Join(parent, "reports-1-generate-the-site")
	git("-C", reports, "worktree", "add", "-q", worktree, "-b", "site")
	var names []string
	for _, r := range Repos(system, worktree) {
		names = append(names, r.Name)
	}
	if slices.Contains(names, "other/elsewhere") || slices.Contains(names, "elsewhere") ||
		slices.Contains(names, "reports-1-generate-the-site") || !slices.Contains(names, "reports") || !slices.Contains(names, "g/po-1") {
		t.Fatalf("repositories %v", names)
	}
}

// An issue's Depends-on and Blocked-by lines are its dependencies, a bare
// #n in its own project (GH-135).
func TestIssueDependencies(t *testing.T) {
	in, system := statusProgram(t)
	teamCheckout(t, system)
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "system", "srd010-delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(system, "docs", "specs", "software-requirements"), 0o755)
	os.WriteFile(filepath.Join(system, "docs", "specs", "software-requirements", "srd010-delivery.md"), []byte(strings.ReplaceAll(string(b), "program/delivery/fleet", "g/po-1")), 0o644)
	first, _ := in.Tracker.CreateIssue("g/po-1", tracker.NewIssue{Title: "Store scores", Labels: []string{"plane::data", "kind::build"}, Description: "Store them.\n"})
	in.Tracker.CreateIssue("g/po-1", tracker.NewIssue{Title: "Publish scores", Labels: []string{"plane::data", "kind::build"},
		Description: fmt.Sprintf("Publish them.\n\nDepends-on: #%d\nBlocked-by: g/other#4\n", first.IID)})
	s, err := LoadSources(system, in.Tracker, in.Sources.Today)
	if err != nil {
		t.Fatal(err)
	}
	for _, i := range s.Issues {
		if i.Title == "Publish scores" {
			if strings.Join(i.DependsOn, " ") != fmt.Sprintf("g/po-1#%d g/other#4", first.IID) {
				t.Fatalf("dependencies %v", i.DependsOn)
			}
			return
		}
	}
	t.Fatal("no Publish scores issue")
}

// Each release sprint has a page with its epics, its stories, and its
// release's Sprint Goals, every link to a page the run writes; the
// navigator nests the sprints under Timeline (GH-139).
func TestSprintPages(t *testing.T) {
	reports, system := hierarchyProgram(t)
	pages, _, sprints, sidebar, err := HierarchyPages(reports, Repos(system, reports))
	if err != nil {
		t.Fatal(err)
	}
	page := string(sprints[Dir+"/sprints/01.0.md"])
	for _, want := range []string{
		"# Sprint W43–45, release 01.0", "2026-10-19 to 2026-11-06",
		"[Agent evaluation](../hierarchy/ep001-agent-evaluation.md)",
		"| [Score trajectories](../hierarchy/po-1/st001-score-trajectories.md) | [Agent evaluation](../hierarchy/ep001-agent-evaluation.md) | done |",
		"| [g/po-1#3 Sprint Goal 01.0, selected](https://gitlab.example.com/g/po-1/-/issues/3) | active | [Score trajectories](../hierarchy/po-1/st001-score-trajectories.md) |",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the sprint page lacks %q:\n%s", want, page)
		}
	}
	if !strings.Contains(string(sprints[Dir+"/sprints/index.md"]), "[Sprint W43–45, release 01.0](01.0.md)") {
		t.Errorf("the sprints index:\n%s", sprints[Dir+"/sprints/index.md"])
	}
	for _, m := range regexp.MustCompile(`\]\(\.\./([^)]+)\)`).FindAllStringSubmatch(page, -1) {
		if _, ok := pages[Dir+"/"+m[1]]; !ok && m[1] != "timeline/index.md" {
			t.Errorf("the sprint page links %s, which the run does not write", m[1])
		}
	}
	nav := string(sidebar)
	if !strings.Contains(nav, "- [Timeline](/timeline/index.md)\n  - [Sprints](/sprints/index.md)\n    - [Sprint W43–45, release 01.0](/sprints/01.0.md)\n") {
		t.Errorf("the navigator does not nest the sprints under Timeline:\n%s", nav)
	}
}
