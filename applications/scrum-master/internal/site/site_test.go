// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
)

func actions(pages []Page) map[string]string {
	out := map[string]string{}
	for _, p := range pages {
		out[p.Path] = p.Action
	}
	return out
}

// The scaffold writes the index and each folder's page; a second run
// changes nothing; an index the program edited and a page a person wrote
// are kept (srd048 R1.2, R2.2, R2.3).
func TestScaffoldRegenerates(t *testing.T) {
	dir := t.TempDir()
	r, err := Scaffold(dir, "v1", false)
	if err != nil {
		t.Fatal(err)
	}
	got := actions(r.Pages)
	if got[Index] != "created" || got[Shell] != "created" || got[Overview] != "created" || got["site/status/index.md"] != "created" || len(got) != 3+len(Folders) {
		t.Fatalf("first run %v", got)
	}
	page, _ := os.ReadFile(filepath.Join(dir, "site", "status", "index.md"))
	if ours, edited, _ := stamped(string(page)); !ours || edited {
		t.Fatalf("a folder page is not stamped:\n%s", page)
	}
	index, _ := os.ReadFile(filepath.Join(dir, Index))
	if strings.Contains(string(index), "scrum-master site version=") || !strings.Contains(string(index), "(status/index.md)") {
		t.Fatalf("the index:\n%s", index)
	}
	os.WriteFile(filepath.Join(dir, Index), []byte("# Our program\n"), 0o644)
	if r, err = Scaffold(dir, "v2", false); err != nil {
		t.Fatal(err)
	}
	if got := actions(r.Pages); got[Index] != "kept" || got[Shell] != "unchanged" || len(got) != 2 {
		t.Fatalf("second run %v", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, Index)); string(b) != "# Our program\n" {
		t.Fatalf("the edited index was replaced:\n%s", b)
	}
}

// A word's run replaces the pages it generated, removes those it no
// longer generates, and keeps a person's page and a generated page a
// person edited (srd048 R2.1, R2.2).
func TestPublishOwnsOnlyItsPages(t *testing.T) {
	dir := t.TempDir()
	pages := map[string][]byte{"site/status/index.md": []byte("# Status\n"), "site/status/old.md": []byte("# Old\n")}
	if _, err := Publish(dir, "status", pages, "v1", false); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "site", "status", "notes.md"), []byte("# Our notes\n"), 0o644)
	edited := filepath.Join(dir, "site", "status", "index.md")
	b, _ := os.ReadFile(edited)
	os.WriteFile(edited, []byte(strings.Replace(string(b), "# Status", "# Status, annotated", 1)), 0o644)

	out, err := Publish(dir, "status", map[string][]byte{"site/status/index.md": []byte("# Status v2\n"), "site/status/new.md": []byte("# New\n")}, "v2", false)
	if err != nil {
		t.Fatal(err)
	}
	got := actions(out)
	want := map[string]string{"site/status/index.md": "kept", "site/status/new.md": "created", "site/status/old.md": "removed", "site/status/notes.md": "kept"}
	for p, a := range want {
		if got[p] != a {
			t.Errorf("%s: %s, want %s (%v)", p, got[p], a, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "site", "status", "old.md")); !os.IsNotExist(err) {
		t.Fatalf("a page no longer generated was left: %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "site", "status", "notes.md")); string(b) != "# Our notes\n" {
		t.Fatal("a person's page was changed")
	}
	again, _ := Publish(dir, "status", map[string][]byte{"site/status/new.md": []byte("# New\n")}, "v3", true)
	if a := actions(again)["site/status/new.md"]; a != "unchanged" {
		t.Fatalf("an unchanged page on a later version: %s", a)
	}
}

// The stamp closes the page, so an HTML page still opens with its doctype,
// and a checksum mismatch reads as edited.
func TestStampClosesThePage(t *testing.T) {
	s := Stamp("<!doctype html>\n<p>x</p>\n", "v1")
	if !strings.HasPrefix(s, "<!doctype html>") {
		t.Fatalf("stamped page:\n%s", s)
	}
	if ours, edited, body := stamped(s); !ours || edited || body != "<!doctype html>\n<p>x</p>\n" {
		t.Fatalf("stamped: %v %v %q", ours, edited, body)
	}
	if _, edited, _ := stamped(strings.Replace(s, "<p>x</p>", "<p>y</p>", 1)); !edited {
		t.Fatal("an edited page reads as unedited")
	}
}

// --dir wins, then SCRUM_MASTER_REPORTS, then the checkout beside this one
// named for the project.
func TestReportsDir(t *testing.T) {
	t.Setenv(ReportsEnv, "/env/reports")
	if d, _ := reportsDir("/flag/reports", ""); d != "/flag/reports" {
		t.Fatalf("--dir: %s", d)
	}
	if d, _ := reportsDir("", "g/reports"); d != "/env/reports" {
		t.Fatalf("env: %s", d)
	}
	parent := t.TempDir()
	here, reports := filepath.Join(parent, "system"), filepath.Join(parent, "reports")
	os.MkdirAll(filepath.Join(reports, ".git"), 0o755)
	os.MkdirAll(here, 0o755)
	t.Setenv(ReportsEnv, "")
	t.Chdir(here)
	if d, _ := reportsDir("", "g/reports"); filepath.Base(d) != "reports" {
		t.Fatalf("sibling: %s", d)
	}
}

// A JSON file has no comment to carry the stamp, so it is written as it is,
// still valid JSON, and its folder's stamps file records it: refreshed while
// unedited, kept once edited, removed with its entry when no longer
// generated (srd048 R2.2).
func TestJSONIsStampedBesideIt(t *testing.T) {
	dir := t.TempDir()
	facts := filepath.Join(dir, "site", "data", "x", "a.json")
	if _, err := Publish(dir, "data/x", map[string][]byte{"site/data/x/a.json": []byte(`{"a":1}`), "site/data/x/b.json": []byte(`[1]`)}, "v1", false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(facts); string(b) != "{\"a\":1}\n" {
		t.Fatalf("a.json = %q", b)
	}
	stamps, _ := os.ReadFile(filepath.Join(dir, "site", "data", "x", StampsFile))
	if !strings.Contains(string(stamps), " v1 a.json\n") || !strings.Contains(string(stamps), " v1 b.json\n") {
		t.Fatalf("stamps:\n%s", stamps)
	}
	out, err := Publish(dir, "data/x", map[string][]byte{"site/data/x/a.json": []byte(`{"a":2}`)}, "v2", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(out); got["site/data/x/a.json"] != "refreshed" || got["site/data/x/b.json"] != "removed" {
		t.Fatalf("second run %v", got)
	}
	if stamps, _ := os.ReadFile(filepath.Join(dir, "site", "data", "x", StampsFile)); strings.Contains(string(stamps), "b.json") {
		t.Fatalf("a removed file keeps its stamp:\n%s", stamps)
	}
	os.WriteFile(facts, []byte(`{"a":"mine"}`), 0o644)
	if out, _ := Publish(dir, "data/x", map[string][]byte{"site/data/x/a.json": []byte(`{"a":3}`)}, "v3", false); actions(out)["site/data/x/a.json"] != "kept" {
		t.Fatalf("an edited JSON file: %v", actions(out))
	}
}

// The placeholder reports init lays down is replaced by the site's index on
// the first run, which then links each generated folder; an index the
// program edited is still kept (GH-107).
func TestScaffoldReplacesReportsInitsPlaceholder(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "site"), 0o755)
	os.WriteFile(filepath.Join(dir, Index), []byte(reports.SiteIndexStub), 0o644)
	r, err := Scaffold(dir, "v1", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(r.Pages)[Index]; got != "refreshed" {
		t.Fatalf("the placeholder: %s", got)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, Index)); !strings.Contains(string(b), "(status/index.md)") {
		t.Fatalf("the index after the first run:\n%s", b)
	}
	os.WriteFile(filepath.Join(dir, Index), []byte(reports.SiteIndexStub+"\nOur notes.\n"), 0o644)
	if r, _ := Scaffold(dir, "v1", false); actions(r.Pages)[Index] != "kept" {
		t.Fatalf("an edited placeholder: %v", actions(r.Pages))
	}
}

// The app shell is written once, pinned, and the program's after: an
// edited shell is kept, and the shell maps folder links to index pages and
// leaves .html links plain (srd048 R7.2, GH-119).
func TestScaffoldWritesTheShell(t *testing.T) {
	dir := t.TempDir()
	if _, err := Scaffold(dir, "v1", false); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, Shell))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"npm/docsify@4.13.1/", "npm/mermaid@11.4.1/", `"/(.*)/$": "/$1/index"`, `"/.*/_sidebar.md": "/_sidebar.md"`, "loadSidebar: true", `/\.(html|json|css|js|txt)$/`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the shell lacks %q", want)
		}
	}
	// Every CDN URL is a well-formed pinned npm specifier: the authoring
	// tool once redacted them as email addresses, and the test then
	// asserted the mangled literal, so it passed (GH-121).
	spec := regexp.MustCompile(`cdn\.jsdelivr\.net/npm/[a-z-]+@\d+\.\d+\.\d+/`)
	for _, line := range strings.Split(string(b), "\n") {
		if strings.Contains(line, "jsdelivr") && !spec.MatchString(line) {
			t.Errorf("a CDN URL is not a pinned specifier: %s", strings.TrimSpace(line))
		}
	}
	if strings.Contains(string(b), "email") {
		t.Error("the shell carries a redacted specifier")
	}
	// The shell is a generated page: stamped, so the next release
	// refreshes it, and kept once a person edits it (GH-127).
	if ours, edited, _ := stamped(string(b)); !ours || edited {
		t.Fatal("the shell is not stamped as the site's own")
	}
	os.WriteFile(filepath.Join(dir, Shell), []byte("<!doctype html><title>Ours</title>\n"), 0o644)
	r, err := Scaffold(dir, "v2", false)
	if err != nil {
		t.Fatal(err)
	}
	if actions(r.Pages)[Shell] != "kept" {
		t.Fatalf("second run %v", actions(r.Pages))
	}
	if b, _ := os.ReadFile(filepath.Join(dir, Shell)); string(b) != "<!doctype html><title>Ours</title>\n" {
		t.Fatalf("the edited shell was replaced:\n%s", b)
	}
}

// A shell exactly as GH-119 or GH-121 wrote it, once and unstamped, is
// taken back and refreshed, stamped, so it refreshes from then on; a dry
// run reports the same and writes nothing (GH-127).
func TestScaffoldTakesBackAnEarlierShell(t *testing.T) {
	for _, rev := range []string{"0e3bbc3d", "a9d991e4"} {
		old, err := exec.Command("git", "show", rev+":./templates/shell.html").Output()
		if err != nil {
			t.Fatalf("the %s shell: %v", rev, err)
		}
		if !priorShells[digest(string(old))] {
			t.Fatalf("the %s shell's digest is not recognised", rev)
		}
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, Dir), 0o755)
		os.WriteFile(filepath.Join(dir, Shell), old, 0o644)
		r, err := Scaffold(dir, "v3", true)
		if err != nil {
			t.Fatal(err)
		}
		if b, _ := os.ReadFile(filepath.Join(dir, Shell)); string(b) != string(old) || actions(r.Pages)[Shell] != "refreshed" {
			t.Fatalf("%s, dry run: %v", rev, actions(r.Pages))
		}
		if r, err = Scaffold(dir, "v3", false); err != nil {
			t.Fatal(err)
		}
		b, _ := os.ReadFile(filepath.Join(dir, Shell))
		if ours, edited, _ := stamped(string(b)); !ours || edited || actions(r.Pages)[Shell] != "refreshed" {
			t.Fatalf("%s: %v, stamped %v", rev, actions(r.Pages), ours)
		}
	}
}

// The divider between the navigator and the page drags and keys from 769
// pixels, kept in the browser; an embedded timeline that lands on anything
// but the timeline renews the session; the plan's cards route from the
// page's folder (GH-2723).
func TestShellResizesTheNavigatorAndWatchesItsFrames(t *testing.T) {
	for _, want := range []string{
		"--nav-width: 300px", ".content { left: var(--nav-width); }", `handle.className = "nav-resizer"`, `setAttribute("role", "separator")`,
		`localStorage.setItem(KEY`, "handle.setPointerCapture(e.pointerId)", `e.key === "ArrowLeft"`, `addEventListener("dblclick"`,
		"function watchFrames()", `f.contentDocument.getElementById("plot")`, "if (!ok) renewSession();", "watchFrames();",
		`new URL(href, "http://site" + folder)`, "plainLinks(vm.route && vm.route.path)",
		".kind-theme, .kb-theme", ".kind-story, .kb-story",
		"function buildTabs()", "buildTabs();", `strip.setAttribute("role", "tablist")`, `p.setAttribute("role", "tabpanel")`, `localStorage.setItem(key, String(i))`, `e.key === "ArrowRight"`,
		".board { display: grid; grid-auto-flow: column; grid-auto-columns: minmax(0, 1fr);", "@container (max-width: 520px) { .board { grid-auto-flow: row; } }",
	} {
		if !strings.Contains(shellHTML, want) {
			t.Errorf("the shell lacks %q", want)
		}
	}
	// The timeline the frame checks for still carries the element.
	if !strings.Contains(timelineHTML, `id="plot"`) {
		t.Error("the timeline has no element with id plot for the shell to find")
	}
}

// The navigator is an expandable tree, and stays visible in narrow
// windows: docked from 520 pixels, a labelled Menu button below (GH-131).
func TestShellFoldsTheNavigatorAndDocksIt(t *testing.T) {
	for _, want := range []string{
		"function foldTree()", "foldTree();", "subMaxLevel: 0,", `r.type === "opaqueredirect"`, `Date.now() - last < 60000`, "location.reload();", `typeof e.data.timelineHeight !== "number"`, "f.contentWindow === e.source", `li.classList.toggle("folded", !open)`, "depth <= 1", `nav.querySelector("li.active")`,
		"@media screen and (min-width: 520px) and (max-width: 768px)", ".sidebar { left: 0; width: 210px; transform: none; }",
		"@media screen and (max-width: 519px)", `content: "Menu"`,
	} {
		if !strings.Contains(shellHTML, want) {
			t.Errorf("the shell lacks %q", want)
		}
	}
}
