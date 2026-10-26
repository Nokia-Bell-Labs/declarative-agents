// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog_test

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
)

// architectureRepo is a git repository holding testdata/architecture's
// shipped ARCHITECTURE at the corpus release tag corpus/v1.0.0, a later
// commit carrying a run's step tag, which is no release (GH-88), then its
// current ARCHITECTURE and the epics in the working tree (GH-56).
func architectureRepo(t *testing.T) string {
	t.Helper()
	dir, src := t.TempDir(), filepath.Join("..", "..", "testdata", "architecture")
	copyFile := func(from, to string) {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(src, from))
		if err != nil {
			t.Fatal(err)
		}
		os.MkdirAll(filepath.Dir(filepath.Join(dir, to)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, to), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("init", "-q", "-b", "main")
	copyFile("ARCHITECTURE-shipped.md", hierarchy.ArchitectureFile)
	// The road map records 01.0 released at corpus/v1.0.0, the baseline
	// the plan is compared with (GH-99).
	copyFile("road-map.md", "docs/road-map.md")
	git("add", "-A")
	git("commit", "-q", "-m", "ship")
	git("tag", "corpus/v1.0.0")
	git("commit", "-q", "--allow-empty", "-m", "a step of a run")
	git("tag", "step/01-start/20261024T000000Z")
	copyFile("ARCHITECTURE-now.md", hierarchy.ArchitectureFile)
	for _, e := range []string{"ep001-publish-scorecards", "ep002-score-runs"} {
		copyFile("docs/specs/epics/"+e+".md", "docs/specs/epics/"+e+".md")
	}
	return dir
}

func architectureFindings(fs []hierarchy.Finding) map[string]string {
	out := map[string]string{}
	for _, f := range fs {
		if f.File == hierarchy.ArchitectureFile && strings.HasPrefix(f.Field, "components.") {
			out[strings.TrimPrefix(f.Field, "components.")] = f.Message
		}
	}
	return out
}

// Under the default policy, a component changed or added since the tag
// and cited by no agreed document is a warning: drift when nothing cites
// it, uncovered when only documents not agreed do; a component cited by an
// agreed document, or unchanged since the tag, is none. Under a policy
// that refuses, the same are findings that fail the gate (GH-56).
func TestArchitectureCoverageAndDrift(t *testing.T) {
	dir := architectureRepo(t)
	r, err := backlog.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	warned := architectureFindings(r.Warnings)
	if keys := slices.Sorted(maps.Keys(warned)); strings.Join(keys, " ") != "grader ranker scorer" {
		t.Fatalf("warned about %v, want grader ranker scorer: %v", keys, r.Warnings)
	}
	if !strings.HasPrefix(warned["ranker"], "drift:") || !strings.Contains(warned["scorer"], "added since corpus/v1.0.0") || !strings.Contains(warned["grader"], "changed since corpus/v1.0.0") {
		t.Fatalf("warnings %v", warned)
	}
	if len(architectureFindings(r.Findings)) != 0 {
		t.Fatalf("a warning failed the gate: %v", r.Findings)
	}

	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "architecture", "control", "policy-refuse.md"))
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(filepath.Join(dir, "control"), 0o755)
	if err := os.WriteFile(filepath.Join(dir, "control", "policy.md"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	if r, err = backlog.Check(dir); err != nil {
		t.Fatal(err)
	}
	if got := architectureFindings(r.Findings); len(got) != 3 || len(r.Warnings) != 0 {
		t.Fatalf("under refuse: findings %v, warnings %v", got, r.Warnings)
	}
}

// A release marked released whose tag only published its plan is named,
// and its plan tag is no baseline; once the record names a tag where the
// release was released, nothing is named and that tag is the baseline. A
// release tag cut before the road map named it is no plan tag (GH-117).
func TestArchitectureReleasedPlanTagIsNoBaseline(t *testing.T) {
	dir := architectureRepo(t)
	b, err := os.ReadFile(filepath.Join(dir, "docs", "road-map.md"))
	if err != nil {
		t.Fatal(err)
	}
	roadMap := func(status, tag string) {
		t.Helper()
		s := strings.Replace(strings.Replace(string(b), "status: released", "status: "+status, 1), "- tag: corpus/v1.0.0", "- tag: "+tag, 1)
		if err := os.WriteFile(filepath.Join(dir, "docs", "road-map.md"), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	named := func() (string, string) {
		t.Helper()
		r, err := backlog.Check(dir)
		if err != nil {
			t.Fatal(err)
		}
		v, err := backlog.Architecture(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range append(r.Warnings, r.Findings...) {
			if f.Field == "releases.01.0.publication" {
				return f.Message, v.Tag
			}
		}
		return "", v.Tag
	}
	// The plan for 01.0 is tagged corpus/v1.1.0, then 01.0 is marked
	// released with the plan's tag left in its publication.
	roadMap("specified", "corpus/v1.1.0")
	git("commit", "-q", "-m", "plan 01.0", "--", "docs/road-map.md")
	git("tag", "corpus/v1.1.0")
	roadMap("released", "corpus/v1.1.0")
	if msg, tag := named(); !strings.Contains(msg, "its tag corpus/v1.1.0 only published its plan") || tag != "" {
		t.Fatalf("a released plan tag: %q, baseline %q", msg, tag)
	}
	// The release tag is cut before the road map names it, then named.
	git("tag", "corpus/v1.2.0")
	roadMap("released", "corpus/v1.2.0")
	if msg, tag := named(); msg != "" || tag != "corpus/v1.2.0" {
		t.Fatalf("a release tag cut first: %q, baseline %q", msg, tag)
	}
	// Or cut after the road map marks the release released.
	git("commit", "-q", "-m", "release 01.0", "--", "docs/road-map.md")
	git("tag", "corpus/v1.3.0")
	roadMap("released", "corpus/v1.3.0")
	if msg, tag := named(); msg != "" || tag != "corpus/v1.3.0" {
		t.Fatalf("a release tag cut after: %q, baseline %q", msg, tag)
	}
}

// The coverage verdict is the same whatever form dir takes: a document
// unchanged since the release is no plan for a component changed since,
// read from an absolute, a relative, or the default directory (GH-114).
func TestArchitectureCoverageWhateverTheDir(t *testing.T) {
	dir := architectureRepo(t)
	// ep001 ships with the release, so the agreement citing publisher is
	// older than the release and plans none of its move.
	for _, args := range [][]string{{"add", "docs/specs/epics"}, {"-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "epics"}, {"tag", "-f", "corpus/v1.0.0"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	warned := func(d string) map[string]string {
		t.Helper()
		r, err := backlog.Check(d)
		if err != nil {
			t.Fatal(err)
		}
		return architectureFindings(r.Warnings)
	}
	want := warned(dir)
	if !strings.HasPrefix(want["publisher"], "uncovered:") {
		t.Fatalf("from the absolute dir, publisher: %q", want["publisher"])
	}
	t.Chdir(filepath.Dir(dir))
	if got := warned(filepath.Base(dir)); !maps.Equal(got, want) {
		t.Errorf("from a relative dir %v, want %v", got, want)
	}
	t.Chdir(dir)
	if got := warned("."); !maps.Equal(got, want) {
		t.Errorf("from . %v, want %v", got, want)
	}
}

// An untracked document draws the untracked finding and leaves a page that
// was current before it was written current (GH-114).
func TestArchitectureUntrackedDocumentLeavesThePageCurrent(t *testing.T) {
	dir := architectureRepo(t)
	git := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	git("add", "-A")
	git("commit", "-q", "-m", "the plan")
	v, err := backlog.Architecture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, backlog.StatusPage), []byte(v.Page()), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "the page")
	b, err := os.ReadFile(filepath.Join(dir, "docs", "specs", "epics", "ep001-publish-scorecards.md"))
	if err != nil {
		t.Fatal(err)
	}
	ranker := strings.NewReplacer("ep001-publish-scorecards", "ep011-rank-agents", "1. publisher", "1. ranker").Replace(string(b))
	if err := os.WriteFile(filepath.Join(dir, "docs", "specs", "epics", "ep011-rank-agents.md"), []byte(ranker), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := backlog.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	untracked := false
	for _, f := range append(r.Warnings, r.Findings...) {
		if strings.HasSuffix(f.File, backlog.StatusPage) {
			t.Errorf("the page drew %s: %s", f.Field, f.Message)
		}
		untracked = untracked || f.Field == "untracked"
	}
	if !untracked {
		t.Errorf("no untracked finding: %v %v", r.Findings, r.Warnings)
	}
}

// Before any release there is no tag to measure from, so the findings say
// no release has shipped the component yet rather than naming a release
// that does not exist (GH-109).
func TestArchitectureDriftBeforeAnyRelease(t *testing.T) {
	dir := architectureRepo(t)
	b, err := os.ReadFile(filepath.Join(dir, "docs", "road-map.md"))
	if err != nil {
		t.Fatal(err)
	}
	unreleased := strings.Replace(string(b), "status: released", "status: agreed", 1)
	if err := os.WriteFile(filepath.Join(dir, "docs", "road-map.md"), []byte(unreleased), 0o644); err != nil {
		t.Fatal(err)
	}
	r, err := backlog.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	warned := architectureFindings(r.Warnings)
	if !strings.HasPrefix(warned["ranker"], "drift: no release has shipped it yet and") {
		t.Fatalf("ranker: %q", warned["ranker"])
	}
	for id, m := range warned {
		if strings.Contains(m, "first release") || strings.Contains(m, "since then") {
			t.Errorf("%s names a release that does not exist: %q", id, m)
		}
	}
}

// The view names each component's state: actual as of the tag when it is
// unchanged since; in transition through the agreed documents citing it,
// with the release the road map plans them for; intended otherwise. The
// page renders the same view under a generated-file header (GH-57).
func TestArchitectureViewNamesEachComponentsState(t *testing.T) {
	dir := architectureRepo(t)
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "architecture", "road-map.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "road-map.md"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := backlog.Architecture(dir)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]backlog.ComponentStatus{}
	var order []string
	for _, c := range v.Components {
		state[c.ID] = c
		order = append(order, c.ID+"="+c.State)
	}
	if strings.Join(order, " ") != "grader=intended store=actual scorer=intended ranker=intended publisher=in_transition" {
		t.Fatalf("states %v", order)
	}
	if state["store"].Tag != "corpus/v1.0.0" || !state["publisher"].Added || state["grader"].Added {
		t.Fatalf("store %+v, publisher %+v, grader %+v", state["store"], state["publisher"], state["grader"])
	}
	docs := state["publisher"].Documents
	if len(docs) != 1 || docs[0].ID != "ep001-publish-scorecards" || docs[0].Release != "02.0" || docs[0].ReleaseStatus != "specified" {
		t.Fatalf("publisher's documents %+v", docs)
	}
	page := v.Page()
	for _, want := range []string{"<!-- Generated by scrum-master backlog architecture", "## What exists", "- **store**, as of corpus/v1.0.0",
		"## What is being built", "**publisher** (new); epic ep001-publish-scorecards, release 02.0 (specified)", "## What is intended", "- **ranker** (new)"} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %q:\n%s", want, page)
		}
	}
}

// writeArchitecture replaces the working tree's ARCHITECTURE with the
// fixture's current one transformed by edit.
func writeArchitecture(t *testing.T, dir string, edit func(string) string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "architecture", "ARCHITECTURE-now.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, hierarchy.ArchitectureFile), []byte(edit(string(b))), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A component whose status ARCHITECTURE writes as deferred is intended and
// planned by nobody on purpose: no drift, and the view says deferred (GH-88).
func TestArchitectureDeferredComponentIsNoDrift(t *testing.T) {
	dir := architectureRepo(t)
	writeArchitecture(t, dir, func(s string) string {
		return strings.Replace(s, "Ranks agents.\n", "Ranks agents.\n\n<!-- field: status -->\n#### Status\n\nDeferred: until the scale work.\n", 1)
	})
	r, err := backlog.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := architectureFindings(r.Warnings)["ranker"]; ok {
		t.Fatalf("a deferred component drew a warning: %v", r.Warnings)
	}
	v, err := backlog.Architecture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if page := v.Page(); !strings.Contains(page, "- **ranker** (new), deferred") {
		t.Fatalf("the page does not say ranker is deferred:\n%s", page)
	}
}

// A citation of a component ARCHITECTURE does not declare is named on the
// view, never dropped (GH-88).
func TestArchitectureViewNamesUndeclaredCitations(t *testing.T) {
	dir := architectureRepo(t)
	writeArchitecture(t, dir, func(s string) string {
		i := strings.Index(s, "<!-- component: publisher -->")
		j := strings.Index(s[i+1:], "<!-- ")
		if j < 0 {
			return s[:i]
		}
		return s[:i] + s[i+1+j:]
	})
	v, err := backlog.Architecture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Undeclared) != 1 || v.Undeclared[0].Component != "publisher" || v.Undeclared[0].Document != "ep001-publish-scorecards" {
		t.Fatalf("undeclared %+v", v.Undeclared)
	}
	if page := v.Page(); !strings.Contains(page, "## Cited but not declared") || !strings.Contains(page, "- **publisher**, cited by ep001-publish-scorecards") {
		t.Fatalf("the page does not name the undeclared citation:\n%s", page)
	}
}

// A committed status page that differs from what the view renders now is
// stale, and the gate says so; the page the view writes is current (GH-88).
func TestArchitectureStatusPageStaleness(t *testing.T) {
	dir := architectureRepo(t)
	page := filepath.Join(dir, backlog.StatusPage)
	if err := os.WriteFile(page, []byte("# Architecture status\n\nOld.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stale := func() bool {
		t.Helper()
		r, err := backlog.Check(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range append(r.Warnings, r.Findings...) {
			if strings.HasSuffix(f.File, backlog.StatusPage) && strings.HasPrefix(f.Message, "stale:") {
				return true
			}
		}
		return false
	}
	if !stale() {
		t.Fatal("a stale status page drew no finding")
	}
	v, err := backlog.Architecture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(page, []byte(v.Page()), 0o644); err != nil {
		t.Fatal(err)
	}
	if stale() {
		t.Fatal("a current status page drew a stale finding")
	}
}

// A repository that encrypts its documents at rest reads the tagged
// ARCHITECTURE through its textconv filter, as git-crypt sets one, so a
// component unchanged since the release is actual (GH-88). The filter here
// is rot13.
func TestArchitectureReadsTheTagThroughTextconv(t *testing.T) {
	dir, src := t.TempDir(), filepath.Join("..", "..", "testdata", "architecture")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	rot := func(s string) string {
		return strings.Map(func(r rune) rune {
			switch {
			case r >= 'a' && r <= 'z':
				return 'a' + (r-'a'+13)%26
			case r >= 'A' && r <= 'Z':
				return 'A' + (r-'A'+13)%26
			}
			return r
		}, s)
	}
	shipped, err := os.ReadFile(filepath.Join(src, "ARCHITECTURE-shipped.md"))
	if err != nil {
		t.Fatal(err)
	}
	git("init", "-q", "-b", "main")
	git("config", "diff.rot13.textconv", "sh -c 'tr A-Za-z N-ZA-Mn-za-m < \"$0\"'")
	os.MkdirAll(filepath.Join(dir, "docs"), 0o755)
	roadMap, _ := os.ReadFile(filepath.Join(src, "road-map.md"))
	os.WriteFile(filepath.Join(dir, "docs", "road-map.md"), roadMap, 0o644)
	os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte(hierarchy.ArchitectureFile+" diff=rot13\n"), 0o644)
	os.WriteFile(filepath.Join(dir, hierarchy.ArchitectureFile), []byte(rot(string(shipped))), 0o644)
	git("add", "-A")
	git("commit", "-q", "-m", "ship, encrypted at rest")
	git("tag", "corpus/v1.0.0")
	os.WriteFile(filepath.Join(dir, hierarchy.ArchitectureFile), shipped, 0o644)
	v, err := backlog.Architecture(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range v.Components {
		if c.State != backlog.Actual {
			t.Fatalf("%s is %s through the textconv filter; want actual: %+v", c.ID, c.State, v.Components)
		}
	}
	if len(v.Components) == 0 {
		t.Fatal("no components")
	}
}

// releaseRepo lays down a system repository under root whose ARCHITECTURE
// adds publisher and scorer, moved by an epic landing in the first sprint
// and one in the second, under an initiative whose R1 falls due on
// 2026-11-06 and R2 on 2026-11-20; release 01.0 delivers R1.1 and 01.1
// R2.1, 01.0 with status first.
func releaseRepo(t *testing.T, root, first string) string {
	t.Helper()
	dir := filepath.Join(root, "system")
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("..", "..", "testdata", p))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	write := func(p, s string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "init", "-q", "-b", "main", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	write(hierarchy.ArchitectureFile, read("architecture/ARCHITECTURE-now.md"))
	in := read("hierarchy/valid/docs/specs/initiatives/in001-reliable-agents.md")
	in = strings.Replace(in, "- parent: th001-dependable-agents\n", "", 1)
	in = strings.Replace(in, "<!-- field: non_goals -->", "<!-- req: R2 -->\n## R2 - Evaluation of every agent\n\n<!-- field: applicability | form: \"tree\" -->\n### Applicability\n\n- due: \"2026-11-20\"\n\n<!-- req: R2.1 -->\n### R2.1\n\nA grader scores every agent's trajectories.\n\n<!-- field: non_goals -->", 1)
	write("docs/specs/initiatives/in001-reliable-agents.md", in)
	epic := read("architecture/docs/specs/epics/ep001-publish-scorecards.md")
	write("docs/specs/epics/ep001-publish-scorecards.md", strings.Replace(epic, "2026-W43..2026-W51", "2026-W43..2026-W45", 1))
	second := strings.NewReplacer("ep001-publish-scorecards", "ep003-score-every-agent", "2026-W43..2026-W51", "2026-W46..2026-W47", "1. publisher", "1. scorer").Replace(epic)
	write("docs/specs/epics/ep003-score-every-agent.md", second)
	write("docs/road-map.md", "---\nid: road-map-fixture\ntitle: Road map fixture\n---\n\n<!-- release: 01.0 | status: "+first+" -->\n## 01.0 - First sprint\n\n<!-- field: deliverables -->\n### Deliverables\n\n- in001-reliable-agents#R1.1\n\n<!-- release: 01.1 | status: specified -->\n## 01.1 - Second sprint\n\n<!-- field: deliverables -->\n### Deliverables\n\n- in001-reliable-agents#R2.1\n")
	return dir
}

func documentFor(t *testing.T, v backlog.ArchitectureView, component string) backlog.StatusDocument {
	t.Helper()
	for _, c := range v.Components {
		if c.ID == component && len(c.Documents) == 1 {
			return c.Documents[0]
		}
	}
	t.Fatalf("%s has not one document: %+v", component, v.Components)
	return backlog.StatusDocument{}
}

// A component lands in the release whose items fall due by the end of the
// horizon of the documents moving it, not the first release naming their
// initiative; a released release is never named for work still being built
// (GH-89).
func TestArchitectureViewNamesTheReleaseWorkLandsIn(t *testing.T) {
	v, err := backlog.Architecture(releaseRepo(t, t.TempDir(), "specified"))
	if err != nil {
		t.Fatal(err)
	}
	if d := documentFor(t, v, "publisher"); d.Release != "01.0" {
		t.Fatalf("publisher, moved in the first sprint, lands in %q; want 01.0", d.Release)
	}
	if d := documentFor(t, v, "scorer"); d.Release != "01.1" {
		t.Fatalf("scorer, moved in the second sprint, lands in %q; want 01.1", d.Release)
	}
	v, err = backlog.Architecture(releaseRepo(t, t.TempDir(), "released"))
	if err != nil {
		t.Fatal(err)
	}
	if d := documentFor(t, v, "publisher"); d.Release != "01.1" || d.ReleaseStatus != "specified" {
		t.Fatalf("publisher, still changing after 01.0 released, lands in %q (%s); want 01.1", d.Release, d.ReleaseStatus)
	}
}

// A system epic's points come from the team repositories' stories beside
// it, through their pinned parent; with no team checkout beside it, the
// page says the points are not rolled up rather than 0 of 0 (GH-89).
func TestArchitectureViewRollsUpTeamStories(t *testing.T) {
	root := t.TempDir()
	dir := releaseRepo(t, root, "specified")
	v, err := backlog.Architecture(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d := documentFor(t, v, "scorer"); !d.NotRolledUp || !strings.Contains(v.Page(), "points not rolled up") {
		t.Fatalf("with no team checkout, scorer's epic %+v", d)
	}
	team := filepath.Join(root, "team")
	if out, err := exec.Command("git", "init", "-q", "-b", "main", team).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "hierarchy", "valid", "docs", "specs", "stories", "st001-score-trajectories.md"))
	if err != nil {
		t.Fatal(err)
	}
	story := strings.Replace(string(b), "- parent: ft001-grader", "- parent: https://gitlab.example.com/program/system/-/blob/corpus/v1.0.0/docs/specs/epics/ep003-score-every-agent.md", 1)
	os.MkdirAll(filepath.Join(team, "docs", "specs", "stories"), 0o755)
	if err := os.WriteFile(filepath.Join(team, "docs", "specs", "stories", "st001-score-trajectories.md"), []byte(story), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, err = backlog.Architecture(dir); err != nil {
		t.Fatal(err)
	}
	if d := documentFor(t, v, "scorer"); d.NotRolledUp || d.Total != 5 || d.Done != 0 {
		t.Fatalf("scorer's epic with the team's 5-point story beside it: %+v", d)
	}
}
