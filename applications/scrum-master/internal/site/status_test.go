// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"flag"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/sprintgoal"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// statusProgram is a fixture program on 2026-11-04: the valid hierarchy
// fixture with st001 recorded done in 01.0's summary, a road map whose 01.0
// sprint holds the day, a reports repository for one person, and a tracker
// holding a Sprint Goal and an open build issue.
func statusProgram(t *testing.T) (StatusInput, string) {
	t.Helper()
	root := t.TempDir()
	system, reportsDir := filepath.Join(root, "system"), filepath.Join(root, "reports")
	src := filepath.Join("..", "..", "testdata", "hierarchy", "valid")
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		os.MkdirAll(filepath.Dir(filepath.Join(system, rel)), 0o755)
		return os.WriteFile(filepath.Join(system, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	write := func(p, s string) {
		t.Helper()
		os.MkdirAll(filepath.Dir(filepath.Join(system, p)), 0o755)
		if err := os.WriteFile(filepath.Join(system, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/road-map.md", "---\nid: road-map-fixture\ntitle: Road map fixture\n---\n\n<!-- release: 01.0 | status: specified -->\n## 01.0 - First\n\n<!-- field: deliverables -->\n### Deliverables\n\n- in001-reliable-agents#R1.1\n\n<!-- field: release -->\n### Release\n\n<!-- field: publication | form: \"tree\" -->\n#### Publication\n\n- tag: corpus/v0.1.0\n- milestone: 2026-W43..W45\n")
	write("docs/sprint-ends/01.0.md", "# Sprint end 01.0\n\n## Stories done\n\n- g/po-1#st001-score-trajectories\n")
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "the program"}} {
		if out, err := exec.Command("git", append([]string{"-C", system}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	today := time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC)
	if _, err := reports.Init(reportsDir, reports.Config{Team: []reports.Member{{Username: "alice", Project: "g/po-1"}}}, today, false); err != nil {
		t.Fatal(err)
	}
	f := fake.New()
	f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Sprint Goal 01.0", Labels: []string{"plane::management", "op::plan"},
		Description: "Score one agent.\n\n" + sprintgoal.Marker("01.0", []string{"in001-reliable-agents#R1"})})
	f.CreateIssue("g/po-1", tracker.NewIssue{Title: "Grade a trajectory", Labels: []string{"plane::data", "kind::build"}, Milestone: "01.0"})
	s, err := LoadSources(system, nil, today)
	if err != nil {
		t.Fatal(err)
	}
	s.Tracker = f
	return StatusInput{Reports: reportsDir, System: system, Sources: s, Tracker: f, Projects: []string{"g/system", "g/po-1"}, Policy: program.Defaults()}, system
}

// The status pages render the road map, the roll-up, the architecture, the
// Sprint Goals, the signals, and the open issues as plain Markdown against
// golden files, each page's facts beside it, the architecture page the
// architecture-status word's own (GH-83).
func TestStatusPagesGolden(t *testing.T) {
	in, system := statusProgram(t)
	pages, data, _, err := StatusPages(in)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index", "road-map", "roll-up", "architecture", "sprint-goals", "signals", "issues"} {
		got, ok := pages[Dir+"/status/"+name+".md"]
		if !ok {
			t.Fatalf("no %s page", name)
		}
		if name != "index" {
			if _, ok := data[Dir+"/data/status/"+name+".json"]; !ok {
				t.Errorf("no facts for %s", name)
			}
		}
		golden := filepath.Join("..", "..", "testdata", "site", "status", name+".md.golden")
		if *update {
			os.MkdirAll(filepath.Dir(golden), 0o755)
			os.WriteFile(golden, got, 0o644)
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if string(want) != string(got) {
			t.Errorf("%s differs from %s:\n%s", name, golden, got)
		}
	}
	view, err := backlog.Architecture(system)
	if err != nil {
		t.Fatal(err)
	}
	// The page is the word's own, its docs-relative links re-aimed at the
	// corpus copies, where the documents sit under the site (GH-122).
	page := string(pages[Dir+"/status/architecture.md"])
	if page != archLinks(view.Page(), system, in.Reports) {
		t.Error("the architecture page is not the architecture-status word's view with its links re-aimed")
	}
	// The fixture holds no ARCHITECTURE.md, so the page names it without
	// a link, which would be dead (GH-139).
	if !strings.Contains(page, "Each component of ARCHITECTURE,") || strings.Contains(page, "](ARCHITECTURE.md)") {
		t.Errorf("the absent ARCHITECTURE is linked:\n%s", page[:200])
	}
}

// The roll-up's numbers are hierarchy.RollUp's, and a story the summary
// records done moves them (GH-83).
func TestStatusRollUpMovesWithTheSummary(t *testing.T) {
	in, system := statusProgram(t)
	row := func() string {
		t.Helper()
		pages, _, _, err := StatusPages(in)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(pages[Dir+"/status/roll-up.md"]), "\n") {
			if strings.HasPrefix(line, "| ep001-agent-evaluation |") {
				return line
			}
		}
		t.Fatal("no epic row")
		return ""
	}
	docs, _ := hierarchy.Load(system)
	progress, _ := hierarchy.RollUp(docs, map[string]bool{"st001-score-trajectories": true})
	var epic hierarchy.Progress
	for _, p := range progress {
		if p.ID == "ep001-agent-evaluation" {
			epic = p
		}
	}
	before := row()
	if !strings.HasSuffix(before, " | "+strconv.Itoa(epic.Done)+" | "+strconv.Itoa(epic.Total)+" |") {
		t.Fatalf("roll-up row %q; RollUp gives %+v", before, epic)
	}
	os.WriteFile(filepath.Join(system, "docs", "sprint-ends", "01.0.md"), []byte("# Sprint end 01.0\n\n## Stories done\n\n- g/po-1#st001-score-trajectories\n- g/po-1#st002-store-scores\n"), 0o644)
	if after := row(); after == before {
		t.Fatalf("a story recorded done moved nothing: %s", after)
	}
}

// Every relative link in the generated status pages resolves to a page
// this run writes beside it — the scaffold's, the status word's, or the
// corpus word's — so the next dead link fails here instead of on the
// published site (GH-122).
func TestStatusPagesLinksResolve(t *testing.T) {
	in, system := statusProgram(t)
	// The fixture needs the document the architecture page links.
	arch, err := os.ReadFile(filepath.Join("..", "..", "testdata", "architecture", "ARCHITECTURE-now.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(system, "docs", "ARCHITECTURE.md"), arch, 0o644); err != nil {
		t.Fatal(err)
	}
	written := map[string]bool{}
	s, err := Scaffold(in.Reports, "v1", false)

	if err != nil {
		t.Fatal(err)
	}
	for _, p := range s.Pages {
		written[p.Path] = true
	}
	pages, data, _, err := StatusPages(in)
	if err != nil {
		t.Fatal(err)
	}
	// With the document in the corpus, the page links the corpus copy.
	if p := string(pages[Dir+"/status/architecture.md"]); !strings.Contains(p, "[ARCHITECTURE](../corpus/ARCHITECTURE.md)") {
		t.Errorf("the ARCHITECTURE link does not point at the corpus copy:\n%s", p[:200])
	}

	corpus, err := CorpusPages(system, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []map[string][]byte{pages, data, corpus} {
		for p := range m {
			written[p] = true
		}
	}
	link := regexp.MustCompile(`\]\(([^)]+)\)`)
	for p, b := range pages {
		for _, m := range link.FindAllStringSubmatch(string(b), -1) {
			target, _, _ := strings.Cut(m[1], "#")
			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			// The app resolves a folder link to the folder's index.md, and
			// a bare link to a .md page (GH-139).
			resolved := path.Join(path.Dir(p), target)
			switch {
			case strings.HasSuffix(target, "/"):
				resolved = path.Join(resolved, "index.md")
			case path.Ext(resolved) == "":
				resolved += ".md"
			}
			if !written[resolved] {
				t.Errorf("%s links %q, and %s is not written", p, m[1], resolved)
			}
		}
	}
}
