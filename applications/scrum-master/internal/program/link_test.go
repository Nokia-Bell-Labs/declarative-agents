// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package program

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// issue link writes the Implements line, keeps the rest of the description,
// refuses what it cannot name, and needs --force to replace (srd007 R6.6).
func TestLinkIssue(t *testing.T) {
	f := fake.New()
	branch, _ := f.DefaultBranch("g/po-1")
	f.AddFile("g/po-1", branch, "docs/specs/epics/ep001-x.md", []byte("epic"))
	f.AddFile("g/po-1", branch, "docs/specs/epics/ep002-y.md", []byte("epic"))
	f.AddFile("g/po-1", branch, "docs/specs/stories/st001-z.md", []byte("story"))
	f.AddFile("g/system", "v1.0.0", "docs/specs/epics/ep003-z.md", []byte("epic"))
	i, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Work", Description: goodBody})
	if err != nil {
		t.Fatal(err)
	}
	body := func() string {
		t.Helper()
		got, err := f.GetIssue("g/po-1", i.IID)
		if err != nil {
			t.Fatal(err)
		}
		return got.Description
	}
	link := func(kind, target string, force, dryRun bool) (LinkResult, error) {
		return LinkIssue(f, "g/po-1", i.IID, kind, target, force, Defaults(), dryRun)
	}

	if r, err := link("epic", "ep001-x", false, true); err != nil || r.Action != "linked" || strings.Contains(body(), "Implements:") {
		t.Fatalf("dry run = %+v, %v\n%s", r, err, body())
	}
	if r, err := link("epic", "ep001-x", false, false); err != nil || r.Action != "linked" {
		t.Fatalf("link = %+v, %v", r, err)
	}
	if b := body(); !strings.HasPrefix(b, goodBody) || !strings.Contains(b, "\nImplements: ep001-x\n") {
		t.Fatalf("body:\n%s", b)
	}
	if r, err := link("epic", "ep001-x", false, false); err != nil || r.Action != "unchanged" {
		t.Fatalf("a second link = %+v, %v", r, err)
	}
	for name, try := range map[string]struct{ kind, target, want string }{
		"another without force": {"epic", "ep002-y", "already implements ep001-x"},
		"an unknown epic":       {"epic", "ep009-gone", "holds no epic"},
		"a story as an epic":    {"epic", "st001-z", "names a story, not an epic"},
		"an epic as a story":    {"story", "ep001-x", "names an epic, not a story"},
		"a feature":             {"epic", "ft001-a", "not a story or an epic"},
	} {
		if _, err := link(try.kind, try.target, false, false); err == nil || !strings.Contains(err.Error(), try.want) {
			t.Errorf("%s: err = %v, want %q", name, err, try.want)
		}
	}
	pinned := "https://gitlab.example.com/g/system/-/blob/v1.0.0/docs/specs/epics/ep003-z.md#R1"
	if r, err := link("epic", pinned, true, false); err != nil || r.Action != "linked" {
		t.Fatalf("force = %+v, %v", r, err)
	}
	if b := body(); strings.Contains(b, "ep001-x") || strings.Count(b, "Implements:") != 1 || !strings.Contains(b, "Implements: "+pinned) {
		t.Fatalf("after force:\n%s", b)
	}
	if lint := Lint(mustIssue(t, f, i.IID), Defaults()); slices.ContainsFunc(lint, func(s string) bool { return strings.Contains(s, "Implements") }) {
		t.Fatalf("lint: %v", lint)
	}
}

func mustIssue(t *testing.T, f *fake.Fake, iid int) tracker.Issue {
	t.Helper()
	i, err := f.GetIssue("g/po-1", iid)
	if err != nil {
		t.Fatal(err)
	}
	return i
}

// issue list --epic lists the epic's stories' issues in rank order, then the
// issues naming the epic directly; another epic's stay out (srd036 R5.1).
func TestListEpicIncludesItsDirectIssues(t *testing.T) {
	f := fake.New()
	const valid = "../../testdata/hierarchy/valid"
	file := func(implements string) int {
		t.Helper()
		i, err := f.CreateIssue("g/po-1", tracker.NewIssue{Title: implements, Description: "Implements: " + implements})
		if err != nil {
			t.Fatal(err)
		}
		return i.IID
	}
	direct := file("ep001-agent-evaluation")
	third := file("st003-scorecard-page")
	first := file("st001-score-trajectories")
	pinned := file("https://gitlab.example.com/g/system/-/blob/v1.0.0/docs/specs/epics/ep001-agent-evaluation.md")
	file("ep999-another")

	l, err := ListImplementing(f, "g/po-1", valid, "", "ep001-agent-evaluation")
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for _, i := range l.Issues {
		got = append(got, i.IID)
	}
	if want := []int{first, third, direct, pinned}; !slices.Equal(got, want) {
		t.Fatalf("issues = %v, want %v", got, want)
	}
	if _, err := ListImplementing(f, "g/po-1", valid, "", "ep999-another"); err == nil || !strings.Contains(err.Error(), "holds no epic") {
		t.Fatalf("an epic the hierarchy does not hold: %v", err)
	}
}

// A story only the team's scrum repository holds is pinned to the newest tag
// holding it, in the form the host serves, or to the tag named; no tag is
// refused, and a repository that is its own scrum repository looks nowhere else
// (srd007 R6.6, GH-2907).
func TestTeamTargetPinsInTheScrumRepository(t *testing.T) {
	subs := Subsystems{{Owner: "Product Owner, team", Scrum: "https://github.com/acme/plan.git",
		Components: []Component{{Name: "code", Repository: "https://github.com/acme/code"}}}}
	tagged := func(tag string) func(string, string) (string, error) {
		return func(holder, file string) (string, error) {
			if holder != "acme/plan" || file != "docs/specs/stories/st001-x.md" {
				t.Errorf("looked for %s in %s", file, holder)
			}
			return tag, nil
		}
	}
	got, err := teamTarget(subs, "acme/code", "story", "st001-x", "", "https://github.com", tagged("corpus/v0.2.0"))
	want := "https://github.com/acme/plan/blob/corpus/v0.2.0/docs/specs/stories/st001-x.md"
	if err != nil || got != want {
		t.Fatalf("newest tag = %q, %v; want %q", got, err, want)
	}
	if err := links.CheckTarget(links.Link{Kind: links.Implements, Target: got}); err != nil {
		t.Errorf("the line it writes is refused: %v", err)
	}
	if got, err := teamTarget(subs, "acme/code", "story", "st001-x", "v1", "https://gitlab.example", tagged("unused")); err != nil || got != "https://gitlab.example/acme/plan/-/blob/v1/docs/specs/stories/st001-x.md" {
		t.Errorf("a named tag = %q, %v", got, err)
	}
	if _, err := teamTarget(subs, "acme/code", "story", "st001-x", "", "https://github.com", tagged("")); err == nil || !strings.Contains(err.Error(), "no tag of acme/plan holds") {
		t.Errorf("no tag = %v, want a refusal", err)
	}
	for _, project := range []string{"acme/plan", "acme/unregistered"} {
		if got, err := teamTarget(subs, project, "story", "st001-x", "", "https://github.com", tagged("unused")); err != nil || got != "" {
			t.Errorf("%s = %q, %v; want nothing to look in", project, got, err)
		}
	}
}

// The newest tag holding the file answers, older or newer ones without it passed
// over (GH-2907).
func TestNewestTagHolding(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.invalid", "GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	file := "docs/specs/stories/st001-x.md"
	git("init", "-q")
	git("commit", "-q", "--allow-empty", "-m", "start")
	git("tag", "corpus/v0.1.0")
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(file)), 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := newestTagHolding(ExecGit{}, dir, file); err != nil || got != "" {
		t.Fatalf("no tag holds it = %q, %v", got, err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(file)), []byte("story"), 0o644); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "story")
	git("tag", "-a", "corpus/v0.2.0", "-m", "holds it")
	if got, err := newestTagHolding(ExecGit{}, dir, file); err != nil || got != "corpus/v0.2.0" {
		t.Fatalf("newest = %q, %v", got, err)
	}
}
