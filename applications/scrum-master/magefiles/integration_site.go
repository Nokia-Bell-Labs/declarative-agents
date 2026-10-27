// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/install"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/reports"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/site"
)

func init() {
	integrationTargets = append(integrationTargets, Integration{}.SiteProfile)
}

// siteRoadMap is the fixture program's road map, its first release
// delivering R1.1 of the initiative.
const siteRoadMap = "---\nid: road-map-fixture\ntitle: Road map fixture\n---\n\n" +
	"<!-- release: 01.0 | status: released -->\n## 01.0 - First\n\n<!-- field: deliverables -->\n### Deliverables\n\n- in001-reliable-agents#R1.1\n\n" +
	"<!-- field: release -->\n### Release\n\n<!-- field: publication | form: \"tree\" -->\n#### Publication\n\n- tag: corpus/v0.1.0\n- milestone: 2026-W43..W45\n\n" +
	"<!-- release: 01.1 | status: specified -->\n## 01.1 - Second\n\n<!-- field: release -->\n### Release\n\n<!-- field: publication | form: \"tree\" -->\n#### Publication\n\n- tag: corpus/v0.2.0\n- milestone: 2026-W46..W48\n"

// SiteProfile runs the whole site end to end (GH-81 to GH-86, srd048):
// a fixture program, the system repository holding the valid hierarchy
// fixture, a road map, a sprint-end summary, and two corpus tags with a
// merge between them, and a reports repository reports init laid down,
// side by side. The agent runs the site profile, and its machine runs the
// scaffold, status, timeline, gantt, history, and corpus words to Done;
// the tree it writes matches the golden tree, the history names the merge,
// and a second run keeps the index the program edited. It needs no rig:
// both checkouts are on local disk.
func (Integration) SiteProfile() error {
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "scrum-master-site-")
	if err != nil {
		return err
	}
	// SITE_KEEP keeps the fixture program and its site to open in a
	// browser, and names where it is.
	if os.Getenv("SITE_KEEP") != "" { //nolint:forbidigo // The opt-in rig reads its tokens and endpoints from the environment (eng01).
		defer fmt.Printf("site kept in %s\n", filepath.Join(tmp, "reports", site.Dir))
	} else {
		defer os.RemoveAll(tmp)
	}
	reportsDir, system := filepath.Join(tmp, "reports"), filepath.Join(tmp, "system")
	if _, err := reports.Init(reportsDir, reports.Config{Team: []reports.Member{{Username: "alice", Project: "g/po-1"}}}, time.Now(), false, false); err != nil {
		return err
	}
	src := filepath.Join(root, "testdata", "hierarchy", "valid")
	if err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		r, _ := filepath.Rel(src, p)
		b, err := os.ReadFile(p)
		if err == nil {
			writeFileTo(filepath.Join(system, r), string(b))
		}
		return err
	}); err != nil {
		return err
	}
	writeFileTo(filepath.Join(system, "control", "trackers.md"), "---\nid: trackers\ntitle: Tracking systems\n---\n\n<!-- field: version -->\n## Version\n\n1\n\n<!-- field: default -->\n## Default\n\n<!-- field: system -->\n### System\n\nbd\n")
	writeFileTo(filepath.Join(system, "docs", "road-map.md"), siteRoadMap)
	writeFileTo(filepath.Join(system, "docs", "sprint-ends", "01.0.md"), "# Sprint end 01.0\n\n## Stories done\n\n- g/po-1#st001-score-trajectories\n")
	git := func(args ...string) error {
		return gitIn(system, append([]string{"-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"remote", "add", "origin", "https://gitlab.example.com/g/system.git"},
		{"add", "-A"}, {"commit", "-q", "-m", "the plan"}, {"tag", "corpus/v0.1.0"}, {"checkout", "-q", "-b", "gh-5-scoring"}} {
		if err := git(args...); err != nil {
			return err
		}
	}
	in001 := filepath.Join(system, "docs", "specs", "initiatives", "in001-reliable-agents.md")
	b, err := os.ReadFile(in001)
	if err != nil {
		return err
	}
	writeFileTo(in001, strings.Replace(string(b), "A grader scores one agent's trajectories.", "A grader scores one agent's trajectories against its rubric.", 1))
	for _, args := range [][]string{{"commit", "-q", "-am", "the rubric"}, {"checkout", "-q", "main"},
		{"merge", "-q", "--no-ff", "-m", "Merge GH-5: score against the rubric (g/system#5)\n\nMerged-by: alice", "gh-5-scoring"}, {"tag", "corpus/v0.2.0"}} {
		if err := git(args...); err != nil {
			return err
		}
	}
	run := func(out string) (string, error) {
		cmd := exec.Command(rel.Agent, "--profile", filepath.Join(root, "plugin", "agents", "site", "profile.yaml"),
			"--core-root", rel.CoreRoot(), "--directory", reportsDir, "--output", filepath.Join(tmp, out))
		// From the reports checkout with SCRUM_MASTER_REPORTS cleared, as the
		// README runs it: the words default to the working directory (GH-115).
		cmd.Dir = reportsDir
		cmd.Env = append(os.Environ(), "PATH="+pathWithout("tpm", "scrum-master"), install.SourceEnv+"="+rel.Src, site.ReportsEnv+"=", site.CacheEnv+"="+filepath.Join(tmp, "cache"))
		var b bytes.Buffer
		cmd.Stdout, cmd.Stderr = &b, &b
		err := cmd.Run()
		return b.String(), err
	}
	c := &check{}
	out, runErr := run("first")
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "the site run must end in Done: %v: %s", runErr, tail17(out))

	var tree []string
	filepath.WalkDir(filepath.Join(reportsDir, site.Dir), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			r, _ := filepath.Rel(reportsDir, p)
			tree = append(tree, filepath.ToSlash(r))
		}
		return nil
	})
	golden := filepath.Join(root, "testdata", "site", "tree.golden")
	if os.Getenv("SITE_UPDATE_GOLDEN") != "" { //nolint:forbidigo // The opt-in rig reads its tokens and endpoints from the environment (eng01).
		writeFileTo(golden, strings.Join(tree, "\n")+"\n")
	}
	want, err := os.ReadFile(golden)
	c.expect(err == nil && string(want) == strings.Join(tree, "\n")+"\n", "the site's tree differs from %s:\n%s", golden, strings.Join(tree, "\n"))

	read := func(p string) string {
		b, _ := os.ReadFile(filepath.Join(reportsDir, filepath.FromSlash(p)))
		return string(b)
	}
	c.expect(strings.Contains(read("site/history/corpus-v0.1.0..corpus-v0.2.0.md"), "R1.1 reworded: merge"), "the history lacks the merged rewording:\n%s", read("site/history/corpus-v0.1.0..corpus-v0.2.0.md"))
	c.expect(strings.Contains(read("site/corpus/index.md"), "(specs/epics/ep001-agent-evaluation.md)"), "the corpus index does not link the epic:\n%s", read("site/corpus/index.md"))
	// Every internal link on every generated page lands on a file, as the
	// app resolves it: relative to the page, a folder to its index.md, a
	// bare name to its .md (GH-139).
	for _, dead := range deadLinks(filepath.Join(reportsDir, site.Dir)) {
		c.expect(false, "a dead link: %s", dead)
	}

	// The page draws the plan itself from the facts it inlines (GH-136).
	timelinePage := read("site/timeline/gantt/program.html")
	c.expect(strings.Contains(timelinePage, `id="chart-data"`) && strings.Contains(timelinePage, "function drawTree(list)") && !strings.Contains(timelinePage, "frappe"),
		"the HTML timeline does not carry its renderer and data")
	c.expect(strings.Contains(read("site/status/roll-up.md"), "| ep001-agent-evaluation | epic |"), "the roll-up lacks the epic:\n%s", read("site/status/roll-up.md"))
	c.expect(strings.Contains(read(site.Index), "(status/index.md)"), "the first run kept reports init's placeholder as the index:\n%s", read(site.Index))

	if err := os.WriteFile(filepath.Join(reportsDir, site.Index), []byte("# Our program\n"), 0o644); err != nil {
		return err
	}
	out, runErr = run("second")
	c.expect(runErr == nil && strings.Contains(out, "final machine state: Done"), "a second site run must end in Done: %v: %s", runErr, tail17(out))
	c.expect(read(site.Index) == "# Our program\n", "the second run replaced the edited index: %q", read(site.Index))
	if len(c.failures) > 0 {
		fmt.Fprintln(os.Stderr, out)
	}
	return c.err("siteProfile")
}

// siteLink is a Markdown link's target or an HTML anchor's href.
var siteLink = regexp.MustCompile(`\]\(([^)\s]+)(?:\s+"(?:[^"\\]|\\.)*")?\)|href="([^"]+)"`)

// deadLinks is each internal link of the site's Markdown pages under root
// that resolves to no file, by the app's rules, as "page -> target".
func deadLinks(root string) []string {
	var dead []string
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".md") {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		for _, m := range siteLink.FindAllStringSubmatch(string(b), -1) {
			t := m[1] + m[2]
			if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "mailto:") || regexp.MustCompile(`^[a-z][a-z0-9+.-]*:`).MatchString(t) {
				continue
			}
			target, _, _ := strings.Cut(t, "#")
			target, _, _ = strings.Cut(target, "?")
			resolved := path.Join(path.Dir(rel), target)
			if strings.HasPrefix(target, "/") {
				resolved = strings.TrimPrefix(target, "/")
			}
			switch {
			case strings.HasSuffix(target, "/"):
				resolved = path.Join(resolved, "index.md")
			case path.Ext(resolved) == "":
				resolved += ".md"
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(resolved))); err != nil || strings.HasPrefix(resolved, "..") {
				dead = append(dead, rel+" -> "+t)
			}
		}
		return nil
	})
	return dead
}
