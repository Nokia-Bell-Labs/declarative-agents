// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

// siteCheckout lays a site repository down at g/reports beside the system
// checkout rollProgram made, as the reports-init profile does: the reports init
// files, and the release it vendors when one is named.
func siteCheckout(t *testing.T, root, vendored string) string {
	t.Helper()
	bare := filepath.Join(filepath.Dir(root), "origins", "g", "reports.git")
	checkoutsGit(t, root, "init", "-q", "--bare", "-b", "main", bare)
	dir := filepath.Join(root, "g", "reports")
	checkoutsGit(t, root, "clone", "-q", "git@example.com:g/reports.git", dir)
	if _, err := reports.Init(dir, reports.Config{}, time.Now(), true, false); err != nil {
		t.Fatal(err)
	}
	if vendored != "" {
		os.MkdirAll(filepath.Join(dir, install.VendorDir), 0o755)
		b, _ := json.Marshal(install.Manifest{Release: vendored, Files: map[string]string{}})
		if err := os.WriteFile(filepath.Join(dir, install.ManifestFile), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	checkoutsGit(t, dir, "add", "-A")
	checkoutsGit(t, dir, "commit", "-q", "-m", "Lay down the site")
	checkoutsGit(t, dir, "push", "-q", "origin", "main")
	return dir
}

// declareSite names the repository in the system checkout's control/program.md.
func declareSite(t *testing.T, system, project string) {
	t.Helper()
	p := filepath.Join(system, filepath.FromSlash(program.LineageFile))
	os.MkdirAll(filepath.Dir(p), 0o755)
	doc := "---\nid: program\ntitle: Program\n---\n\n<!-- field: site_repository -->\n## Site repository\n\n" + project + "\n"
	if err := os.WriteFile(p, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The site repository the program names takes the release as any other does:
// an admitted issue, and a branch doctor did not refuse (GH-2915).
func TestRollTakesTheDeclaredSiteRepository(t *testing.T) {
	root, system := rollProgram(t, "g/pods/p1/tpm")
	atRelease(t, filepath.Join(root, "g", "pods", "p1", "tpm"))
	dir := siteCheckout(t, root, "v1.0.0")
	declareSite(t, system, "git@example.com:g/reports.git")
	f := fake.New()
	r, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	e := byRollProject(t, r, "g/reports")
	if e.Action != "opened" || e.Issue == "" {
		t.Fatalf("the site repository = %+v, want opened with its issue", e)
	}
	checkoutsGit(t, dir, "fetch", "-q", "origin", e.Branch)
	if m := checkoutsGit(t, dir, "show", "origin/"+e.Branch+":"+install.ManifestFile); !strings.Contains(m, "v9.9.9") {
		t.Errorf("the branch's manifest does not take v9.9.9:\n%s", m)
	}
	issues, _ := f.ListIssues("g/reports", tracker.IssueQuery{State: "opened"})
	if len(issues) != 1 {
		t.Fatalf("issues in the site repository: %+v", issues)
	}
	if reasons := program.Lint(issues[0], program.Defaults()); len(reasons) > 0 {
		for _, reason := range reasons {
			if strings.Contains(reason, "component") {
				t.Errorf("the upgrade issue is not admitted: %v", reasons)
			}
		}
	}
	if len(r.Notes) != 0 {
		t.Errorf("a declared site repository was noted: %v", r.Notes)
	}
}

// A site written from the system checkout needs no release, so a roll gives it
// none (GH-2915).
func TestRollSkipsASiteRepositoryVendoringNothing(t *testing.T) {
	root, system := rollProgram(t, "g/pods/p1/tpm")
	atRelease(t, filepath.Join(root, "g", "pods", "p1", "tpm"))
	dir := siteCheckout(t, root, "")
	declareSite(t, system, "g/reports")
	f := fake.New()
	r, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	if e := byRollProject(t, r, "g/reports"); e.Action != "skipped" || !strings.Contains(e.Detail, "vendors no") {
		t.Errorf("the site repository = %+v, want skipped", e)
	}
	if issues, _ := f.ListIssues("g/reports", tracker.IssueQuery{State: "opened"}); len(issues) != 0 {
		t.Errorf("issues filed: %+v", issues)
	}
	if _, err := os.Stat(filepath.Join(dir, install.ManifestFile)); err == nil {
		t.Error("the roll vendored into it")
	}
}

// Beside the system checkout may stand another program's site, so a roll takes
// nothing it was not told about, and says what it left (GH-2915).
func TestRollNotesAnUndeclaredSiteRepositoryAndLeavesIt(t *testing.T) {
	root, system := rollProgram(t, "g/pods/p1/tpm")
	atRelease(t, filepath.Join(root, "g", "pods", "p1", "tpm"))
	siteCheckout(t, root, "v1.0.0")
	f := fake.New()
	r, err := Roll(f, system, false)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	for _, e := range r.Repositories {
		if e.Project == "g/reports" {
			t.Fatalf("an undeclared site repository was rolled: %+v", e)
		}
	}
	if issues, _ := f.ListIssues("g/reports", tracker.IssueQuery{State: "opened"}); len(issues) != 0 {
		t.Errorf("issues filed: %+v", issues)
	}
	if len(r.Notes) != 1 || !strings.Contains(r.Notes[0], "reports beside") || !strings.Contains(r.Notes[0], "site_repository") {
		t.Fatalf("notes = %v", r.Notes)
	}
	if h := r.Human(); !strings.Contains(h, "note ") {
		t.Errorf("Human does not carry the note: %q", h)
	}
}

// One vendoring nothing is not worth a note: the roll would not take it anyway.
func TestRollNotesNoSiteRepositoryVendoringNothing(t *testing.T) {
	root, system := rollProgram(t, "g/pods/p1/tpm")
	atRelease(t, filepath.Join(root, "g", "pods", "p1", "tpm"))
	siteCheckout(t, root, "")
	r, err := Roll(fake.New(), system, false)
	if err != nil {
		t.Fatalf("Roll: %v", err)
	}
	if len(r.Notes) != 0 {
		t.Errorf("notes = %v", r.Notes)
	}
}

// A site repository has no corpus or control files, so doctor has nothing to do
// there and an upgrade can finish; anything else still refuses (GH-2915).
func TestDoctorHasNothingToRepairInASiteRepository(t *testing.T) {
	dir := repo(t)
	if _, err := reports.Init(dir, reports.Config{}, time.Now(), true, false); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "add", "-A")
	run(t, dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "-m", "laid down")
	r, err := Doctor(DoctorOptions{Dir: dir})
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if r.Level != "site" || len(r.Files) != 0 {
		t.Errorf("level = %q, files = %v", r.Level, r.Files)
	}
	out, err := exec.Command("git", "-C", dir, "status", "--porcelain").CombinedOutput()
	if err != nil || len(out) != 0 {
		t.Errorf("doctor wrote: %s (%v)", out, err)
	}
	other := repo(t)
	if err := os.MkdirAll(filepath.Join(other, "site"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "site", "index.md"), []byte("# A site of its own\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Doctor(DoctorOptions{Dir: other}); err == nil || !strings.Contains(err.Error(), "no program repository") {
		t.Errorf("a repository with a site of its own: %v", err)
	}
}

// A site repository is declared by its program's lineage, or by the system
// standing beside it as its README says to run it; a site found only through the
// account is told how to declare itself, and anything else is refused as before
// (GH-2915).
func TestDeclaredBySiteRepository(t *testing.T) {
	gitRepo := func(origin string, site bool) string {
		dir := t.TempDir()
		for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", origin}} {
			if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
				t.Fatalf("git %v: %s", args, out)
			}
		}
		if site {
			if _, err := reports.Init(dir, reports.Config{}, time.Now(), true, false); err != nil {
				t.Fatal(err)
			}
		}
		return dir
	}
	system := gitRepo("https://github.com/acme/system.git", false)
	const own = "https://github.com/acme/site.git"
	if err := declaredBy(system, gitRepo(own, true), true); err != nil {
		t.Errorf("a site repository with the system beside it: %v", err)
	}
	err := declaredBy(system, gitRepo(own, true), false)
	if err == nil || !strings.Contains(err.Error(), "does not declare acme/site") || !strings.Contains(err.Error(), "site_repository") {
		t.Errorf("a site repository found through the account: %v", err)
	}
	if err := declaredBy(system, gitRepo("https://github.com/acme/notes", false), true); err == nil || strings.Contains(err.Error(), "site_repository") {
		t.Errorf("an unrelated clone with the system beside it: %v", err)
	}
	declareSite(t, system, "acme/site")
	if err := declaredBy(system, gitRepo(own, true), false); err != nil {
		t.Errorf("the declared site repository: %v", err)
	}
	if err := declaredBy(system, gitRepo(own, false), false); err != nil {
		t.Errorf("a declared repository is declared, however it is laid out: %v", err)
	}
}
