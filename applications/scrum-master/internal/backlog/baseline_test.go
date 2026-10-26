// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package backlog_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func states(t *testing.T, dir string) map[string]backlog.ComponentStatus {
	t.Helper()
	v, err := backlog.Architecture(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]backlog.ComponentStatus{}
	for _, c := range v.Components {
		out[c.ID] = c
	}
	return out
}

func warnings(t *testing.T, dir string) map[string]string {
	t.Helper()
	r, err := backlog.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	return architectureFindings(r.Warnings)
}

// A plan's publication tag is no release: after a later corpus tag the
// baseline is still the released release's, so what the plan added stays
// added, and nothing the plan declared reads as existing (GH-99, finding 1).
func TestAPlanTagIsNoRelease(t *testing.T) {
	dir := architectureRepo(t)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "the plan")
	gitIn(t, dir, "tag", "corpus/v1.1.0")
	s := states(t, dir)
	if s["store"].State != backlog.Actual || s["store"].Tag != "corpus/v1.0.0" || s["publisher"].State != backlog.InTransition || !s["publisher"].Added {
		t.Fatalf("after a plan tag: store %+v, publisher %+v", s["store"], s["publisher"])
	}
	if w := warnings(t, dir); !strings.Contains(w["scorer"], "added since corpus/v1.0.0") {
		t.Fatalf("warnings %v", w)
	}
}

// A component the release declared but whose citing agreed epic still has
// work open, its horizon past the release's due date, is being built, not
// existing (GH-99, finding 1).
func TestADeclaredComponentWithOpenWorkIsNotActual(t *testing.T) {
	dir := architectureRepo(t)
	b, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "hierarchy", "valid", "docs", "specs", "initiatives", "in001-reliable-agents.md"))
	os.MkdirAll(filepath.Join(dir, "docs", "specs", "initiatives"), 0o755)
	os.WriteFile(filepath.Join(dir, "docs", "specs", "initiatives", "in001-reliable-agents.md"), b, 0o644)
	roadMap, _ := os.ReadFile(filepath.Join(dir, "docs", "road-map.md"))
	os.WriteFile(filepath.Join(dir, "docs", "road-map.md"), []byte(strings.Replace(string(roadMap), "- tag: corpus/v1.0.0\n", "- tag: corpus/v1.0.0\n\n<!-- field: deliverables -->\n### Deliverables\n\n- in001-reliable-agents#R1.1\n", 1)), 0o644)
	epic := filepath.Join(dir, "docs", "specs", "epics", "ep001-publish-scorecards.md")
	e, _ := os.ReadFile(epic)
	os.WriteFile(epic, []byte(strings.NewReplacer("1. publisher", "1. store", "2026-W43..2026-W51", "2026-W47..2026-W51").Replace(string(e))), 0o644)
	if s := states(t, dir)["store"]; s.State != backlog.InTransition || len(s.Documents) != 1 {
		t.Fatalf("store, cited by an epic still open past the release: %+v", s)
	}
	os.WriteFile(epic, []byte(strings.NewReplacer("1. publisher", "1. store", "2026-W43..2026-W51", "2026-W43..2026-W44").Replace(string(e))), 0o644)
	if s := states(t, dir)["store"]; s.State != backlog.Actual {
		t.Fatalf("store, its epic's work delivered by the release: %+v", s)
	}
}

// A change is covered only by an agreed document added or changed since
// the release: an epic agreed before it planned an earlier move (GH-99,
// finding 2).
func TestAnOlderAgreementDoesNotCoverANewChange(t *testing.T) {
	dir := architectureRepo(t)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "the epics")
	roadMap, _ := os.ReadFile(filepath.Join(dir, "docs", "road-map.md"))
	os.WriteFile(filepath.Join(dir, "docs", "road-map.md"), []byte(strings.Replace(string(roadMap), "- tag: corpus/v1.0.0", "- tag: corpus/v1.1.0", 1)), 0o644)
	gitIn(t, dir, "commit", "-q", "-am", "release 01.0 at v1.1.0")
	gitIn(t, dir, "tag", "corpus/v1.1.0")
	arch := filepath.Join(dir, hierarchy.ArchitectureFile)
	b, _ := os.ReadFile(arch)
	os.WriteFile(arch, []byte(strings.Replace(string(b), "### publisher\n", "### publisher\n\nPublishes scorecards, and now digests.\n", 1)), 0o644)
	if w := warnings(t, dir); !strings.Contains(w["publisher"], "changed since corpus/v1.1.0") {
		t.Fatalf("a change cited only by an epic agreed before the release: %v", w)
	}
	epic := filepath.Join(dir, "docs", "specs", "epics", "ep001-publish-scorecards.md")
	e, _ := os.ReadFile(epic)
	os.WriteFile(epic, append(e, []byte("\n")...), 0o644)
	os.WriteFile(epic, []byte(strings.Replace(string(e), "Teams cannot show", "Teams still cannot show", 1)), 0o644)
	if w := warnings(t, dir); w["publisher"] != "" {
		t.Fatalf("a change its epic, edited since, plans: %v", w)
	}
}

// A deferred component draws no warning, though its text changes (GH-99,
// finding 3).
func TestADeferredComponentNeverWarns(t *testing.T) {
	dir := architectureRepo(t)
	writeArchitecture(t, dir, func(s string) string {
		s = strings.Replace(s, "Ranks agents.\n", "Ranks agents.\n\n<!-- field: status -->\n#### Status\n\nDeferred: until the scale work.\n", 1)
		return strings.Replace(s, "<!-- component: grader -->", "<!-- component: grader -->", 1)
	})
	b, _ := os.ReadFile(filepath.Join(dir, hierarchy.ArchitectureFile))
	// grader changed since the release; mark it deferred too.
	s := strings.Replace(string(b), "<!-- component: store -->", "<!-- field: status -->\n#### Status\n\nDeferred: later.\n\n<!-- component: store -->", 1)
	os.WriteFile(filepath.Join(dir, hierarchy.ArchitectureFile), []byte(s), 0o644)
	w := warnings(t, dir)
	if _, ok := w["ranker"]; ok {
		t.Errorf("a deferred, added component warned: %v", w)
	}
	if _, ok := w["grader"]; ok {
		t.Errorf("a deferred, changed component warned: %v", w)
	}
	if st := states(t, dir)["grader"]; st.State != backlog.Intended || !st.Deferred {
		t.Errorf("grader %+v", st)
	}
}

// The baseline is read by name from the road map, so a branch cut before
// the release's tag compares with that release, as main does, not with an
// older tag its history holds (GH-99, finding 4).
func TestABranchCutBeforeTheReleaseSeesIt(t *testing.T) {
	dir := architectureRepo(t)
	gitIn(t, dir, "tag", "corpus/v0.9.0", "HEAD~1")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "the release")
	gitIn(t, dir, "tag", "corpus/v1.2.0")
	gitIn(t, dir, "checkout", "-q", "-b", "older", "HEAD~1")
	os.WriteFile(filepath.Join(dir, hierarchy.ArchitectureFile), mustRead(t, filepath.Join("..", "..", "testdata", "architecture", "ARCHITECTURE-now.md")), 0o644)
	roadMap := strings.Replace(string(mustRead(t, filepath.Join("..", "..", "testdata", "architecture", "road-map.md"))), "corpus/v1.0.0", "corpus/v1.2.0", 1)
	os.WriteFile(filepath.Join(dir, "docs", "road-map.md"), []byte(roadMap), 0o644)
	if out, _ := exec.Command("git", "-C", dir, "merge-base", "--is-ancestor", "corpus/v1.2.0", "HEAD").CombinedOutput(); len(out) != 0 {
		t.Fatalf("merge-base: %s", out)
	}
	s := states(t, dir)
	if s["store"].Tag != "corpus/v1.2.0" && s["store"].State == backlog.Actual {
		t.Fatalf("store on a branch cut before the release: %+v", s["store"])
	}
	if v, _ := backlog.Architecture(dir); v.Tag != "corpus/v1.2.0" {
		t.Fatalf("the baseline on the branch is %q, want the road map's released release corpus/v1.2.0", v.Tag)
	}
}

func mustRead(t *testing.T, p string) []byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// An untracked document is reported as untracked and nothing else, though
// it cites a component ARCHITECTURE does not declare; warnings are a list,
// empty or not (GH-100).
func TestUntrackedIsUntrackedOnly(t *testing.T) {
	dir := architectureRepo(t)
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-q", "-m", "the epics")
	epic := mustRead(t, filepath.Join("..", "..", "testdata", "architecture", "docs", "specs", "epics", "ep001-publish-scorecards.md"))
	draft := strings.NewReplacer("ep001-publish-scorecards", "ep003-spend", "1. publisher", "1. spend_control").Replace(string(epic))
	os.WriteFile(filepath.Join(dir, "docs", "specs", "epics", "ep003-spend.md"), []byte(draft), 0o644)
	r, err := backlog.Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	var about []string
	for _, f := range r.Findings {
		if strings.Contains(f.File, "ep003-spend") {
			about = append(about, f.Field)
		}
	}
	if strings.Join(about, ",") != "untracked" {
		t.Fatalf("findings about the untracked epic: %v (%v)", about, r.Findings)
	}
	if r.Warnings == nil {
		t.Fatal("warnings is nil")
	}
}
