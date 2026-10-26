// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/timeline"
)

// The as-of-tag facts are read from the corpus each corpus tag holds, from
// local git: two tags whose plans differ in one epic's horizon give facts
// that differ in that epic's planned days and nothing else, and a tag of
// another name is no corpus release (GH-82).
func TestTimelineAsOfEachCorpusTag(t *testing.T) {
	t.Setenv(CacheEnv, t.TempDir())
	system := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", system, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	epic, err := os.ReadFile(filepath.Join("..", "..", "testdata", "architecture", "docs", "specs", "epics", "ep001-publish-scorecards.md"))
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
	git("init", "-q", "-b", "main")
	write("docs/road-map.md", "---\nid: road-map-fixture\ntitle: Road map fixture\n---\n\n<!-- release: 01.0 | status: specified -->\n## 01.0 - First\n\n<!-- field: release -->\n### Release\n\n<!-- field: publication | form: \"tree\" -->\n#### Publication\n\n- tag: corpus/v0.1.0\n- milestone: 2026-W43..W45\n")
	write("docs/specs/epics/ep001-publish-scorecards.md", strings.Replace(string(epic), "2026-W43..2026-W51", "2026-W43..2026-W45", 1))
	git("add", "-A")
	git("commit", "-q", "-m", "plan 01.0")
	git("tag", "corpus/v0.1.0")
	write("docs/specs/epics/ep001-publish-scorecards.md", strings.Replace(string(epic), "2026-W43..2026-W51", "2026-W43..2026-W47", 1))
	git("commit", "-q", "-am", "the epic slips two weeks")
	git("tag", "corpus/v0.2.0")
	git("tag", "step/03-plan/20261024T000000Z")

	tags, err := CorpusTags(system)
	if err != nil || strings.Join(tags, " ") != "corpus/v0.1.0 corpus/v0.2.0" {
		t.Fatalf("corpus tags %v, %v", tags, err)
	}
	pages, now, err := TimelinePages(system, TimelineSources{Today: time.Date(2026, 11, 18, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	read := func(name string) timeline.Facts {
		t.Helper()
		var f timeline.Facts
		if err := json.Unmarshal(pages["site/timeline/"+name], &f); err != nil {
			t.Fatalf("%s: %v (%v)", name, err, keys(pages))
		}
		return f
	}
	before, after := read("facts@corpus-v0.1.0.json"), read("facts@corpus-v0.2.0.json")
	if before.AsOf != "corpus/v0.1.0" || after.AsOf != "corpus/v0.2.0" || now.AsOf != "" {
		t.Fatalf("as of %q, %q, %q", before.AsOf, after.AsOf, now.AsOf)
	}
	if len(before.Items) != 1 || len(after.Items) != 1 || before.Items[0].Planned.End != "2026-11-06" || after.Items[0].Planned.End != "2026-11-20" {
		t.Fatalf("planned at the tags: %+v, %+v", before.Items, after.Items)
	}
	b, a := before.Items[0], after.Items[0]
	b.Planned, a.Planned = nil, nil
	b.Release, a.Release, b.Tag, a.Tag = "", "", "", ""
	b.Horizon, a.Horizon = "", ""
	bj, _ := json.Marshal(b)
	aj, _ := json.Marshal(a)
	if string(bj) != string(aj) {
		t.Fatalf("the tags' facts differ beyond the slipped epic's days:\n%s\n%s", bj, aj)
	}
	if !strings.Contains(string(pages["site/timeline/index.md"]), `src="timeline/gantt/program.html?embed=1&view=timeline"`) {
		t.Errorf("the timeline page does not embed the interactive timeline:\n%s", pages["site/timeline/index.md"])
	}
	if !strings.Contains(string(pages["site/timeline/index.md"]), "| ep001-publish-scorecards | epic |") {
		t.Fatalf("the timeline page:\n%s", pages["site/timeline/index.md"])
	}
}

func keys(m map[string][]byte) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The system checkout is found beside a reports checkout named by a
// relative path, . included.
func TestSystemDirFromARelativeReportsDir(t *testing.T) {
	parent := t.TempDir()
	os.MkdirAll(filepath.Join(parent, "reports"), 0o755)
	os.MkdirAll(filepath.Join(parent, "system", "control"), 0o755)
	os.WriteFile(filepath.Join(parent, "system", "control", "trackers.md"), []byte("bd\n"), 0o644)
	t.Chdir(filepath.Join(parent, "reports"))
	if got, err := SystemDir("."); err != nil || filepath.Base(got) != "system" {
		t.Fatalf("SystemDir(.) = %s, %v", got, err)
	}
}

// A tag's corpus is read once and kept by commit, so a second reader, as
// the next word's process would be, gets the same copy (GH-96).
func TestAtTagKeepsEachTagOnce(t *testing.T) {
	cache := t.TempDir()
	t.Setenv(CacheEnv, cache)
	system := t.TempDir()
	os.MkdirAll(filepath.Join(system, "docs"), 0o755)
	os.WriteFile(filepath.Join(system, "docs", "VISION.md"), []byte("# Vision\n"), 0o644)
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@example.invalid", "commit", "-q", "-m", "v"}, {"tag", "corpus/v0.1.0"}} {
		if out, err := exec.Command("git", append([]string{"-C", system}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	first, done, err := AtTag(system, "corpus/v0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	done()
	second, done, err := AtTag(system, "corpus/v0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	done()
	if first != second || !strings.HasPrefix(first, cache) {
		t.Fatalf("the tag was read twice: %s, %s", first, second)
	}
	if b, err := os.ReadFile(filepath.Join(second, "docs", "VISION.md")); err != nil || string(b) != "# Vision\n" {
		t.Fatalf("the kept corpus: %q, %v", b, err)
	}
}
