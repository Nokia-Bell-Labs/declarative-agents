// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
)

// gitRepo makes a repository at dir holding files, committed on the day
// given when it is not empty.
func gitRepo(t *testing.T, dir string, files map[string]string) func(day string, args ...string) {
	t.Helper()
	git := func(day string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.invalid"}, args...)...)
		if day != "" {
			cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE="+day+"T12:00:00Z", "GIT_AUTHOR_DATE="+day+"T12:00:00Z")
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	os.MkdirAll(dir, 0o755)
	git("", "init", "-q", "-b", "main")
	for p, s := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return git
}

const namedRoadMap = "---\nid: road-map-fixture\ntitle: Road map fixture\n---\n\n<!-- release: 01.0 | status: released -->\n## 01.0 - First\n\n<!-- field: release -->\n### Release\n\n<!-- field: publication | form: \"tree\" -->\n#### Publication\n\n- tag: corpus/v0.3.0\n- milestone: 2026-W43..W45\n"

// The timeline and history read the corpus tags from the oldest the road
// map names on: an earlier program's tags drop out, and a plan tag after
// the named one stays; a road map naming none scopes nothing (GH-115).
func TestProgramTagsStartAtTheRoadMapsOldest(t *testing.T) {
	system := t.TempDir()
	git := gitRepo(t, system, map[string]string{"docs/VISION.md": "# Vision\n"})
	for i, day := range []string{"2026-01-05", "2026-02-02", "2026-10-26", "2026-11-09"} {
		if i == 2 {
			os.WriteFile(filepath.Join(system, "docs", "road-map.md"), []byte(namedRoadMap), 0o644)
		}
		git(day, "add", "-A")
		git(day, "commit", "-q", "--allow-empty", "-m", "step")
		git(day, "tag", "corpus/v0."+string(rune('1'+i))+".0")
	}
	tags, err := ProgramTags(system)
	if err != nil || strings.Join(tags, " ") != "corpus/v0.3.0 corpus/v0.4.0" {
		t.Fatalf("program tags %v, %v", tags, err)
	}
	os.WriteFile(filepath.Join(system, "docs", "road-map.md"), []byte(strings.Replace(namedRoadMap, "- tag: corpus/v0.3.0\n", "", 1)), 0o644)
	if tags, err := ProgramTags(system); err != nil || len(tags) != 4 {
		t.Fatalf("a road map naming no tag: %v, %v", tags, err)
	}
	os.Remove(filepath.Join(system, "docs", "road-map.md"))
	if tags, err := ProgramTags(system); err != nil || len(tags) != 4 {
		t.Fatalf("no road map: %v, %v", tags, err)
	}
}

// With no cache named, a tag's snapshot is kept inside the system
// checkout's git directory, not the user cache (GH-115).
func TestAtTagCachesInTheGitDirectory(t *testing.T) {
	t.Setenv(CacheEnv, "")
	system := t.TempDir()
	git := gitRepo(t, system, map[string]string{"docs/VISION.md": "# Vision\n"})
	git("", "add", "-A")
	git("", "commit", "-q", "-m", "v")
	git("", "tag", "corpus/v0.1.0")
	dir, done, err := AtTag(system, "corpus/v0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	done()
	real, _ := filepath.EvalSymlinks(filepath.Join(system, ".git", "scrum-master", "site"))
	if got, _ := filepath.EvalSymlinks(filepath.Dir(dir)); got != real {
		t.Fatalf("the snapshot is at %s, want under %s", dir, real)
	}
}

// A corpus the system repository encrypts at rest is withheld from a site
// the reports repository does not encrypt, unless the reports repository
// encrypts site/ or the person opts in (GH-115).
func TestAnEncryptedCorpusIsWithheld(t *testing.T) {
	root := t.TempDir()
	system, reports := filepath.Join(root, "system"), filepath.Join(root, "reports")
	gitRepo(t, system, map[string]string{".gitattributes": "docs/** filter=git-crypt diff=git-crypt\n"})
	gitRepo(t, reports, nil)
	t.Setenv(DecryptedEnv, "")
	if why := withheld(system, reports); !strings.Contains(why, DecryptedEnv+"=1") {
		t.Fatalf("an encrypted corpus beside a plain site: %q", why)
	}
	t.Setenv(DecryptedEnv, "1")
	if why := withheld(system, reports); why != "" {
		t.Fatalf("opted in: %q", why)
	}
	t.Setenv(DecryptedEnv, "")
	os.WriteFile(filepath.Join(reports, ".gitattributes"), []byte("site/** filter=git-crypt diff=git-crypt\n"), 0o644)
	if why := withheld(system, reports); why != "" {
		t.Fatalf("an encrypted site: %q", why)
	}
	plain := filepath.Join(root, "plain")
	gitRepo(t, plain, nil)
	os.Remove(filepath.Join(reports, ".gitattributes"))
	if why := withheld(plain, reports); why != "" {
		t.Fatalf("a plain corpus: %q", why)
	}
}

// The corpus word over an encrypted corpus beside a plain site writes one
// page saying why, and prunes the pages an earlier run copied (GH-115).
func TestCorpusWordWithholdsAnEncryptedCorpus(t *testing.T) {
	root := t.TempDir()
	system, reports := filepath.Join(root, "system"), filepath.Join(root, "reports")
	gitRepo(t, system, map[string]string{".gitattributes": "docs/** filter=git-crypt diff=git-crypt\n", "control/trackers.md": "bd\n", "docs/VISION.md": "# Vision\n"})
	gitRepo(t, reports, nil)
	t.Setenv(DecryptedEnv, "")
	if _, err := PublishTree(reports, CorpusDir, map[string][]byte{Dir + "/" + CorpusDir + "/VISION.md": []byte("# Vision\n")}, "dev", false); err != nil {
		t.Fatal(err)
	}
	var r Result
	if err := corpusWord.run(&cli.Context{}, reports, &r); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(reports, Dir, CorpusDir, "VISION.md")); err == nil {
		t.Error("the copied page survived")
	}
	b, err := os.ReadFile(filepath.Join(reports, Dir, CorpusDir, "index.md"))
	if err != nil || !strings.Contains(string(b), "No document is copied here") {
		t.Fatalf("the corpus index: %s, %v", b, err)
	}
	if len(r.Notes) != 1 || !strings.HasPrefix(r.Notes[0], "corpus: ") {
		t.Fatalf("notes %v", r.Notes)
	}
}

// The embedded architecture page's docs-relative links point at the corpus
// copies; when the corpus is withheld they point at the forge, and with no
// forge they are left as written (GH-122).
func TestArchLinksFollowTheCorpus(t *testing.T) {
	page := "Each component of [ARCHITECTURE](ARCHITECTURE.md), by what it is today.\n"
	root := t.TempDir()
	system, reports := filepath.Join(root, "system"), filepath.Join(root, "reports")
	gitRepo(t, system, map[string]string{"docs/VISION.md": "# Vision\n"})
	gitRepo(t, reports, nil)
	t.Setenv(DecryptedEnv, "")
	// A document the system does not hold is named without a link.
	if got := archLinks(page, system, reports); got != "Each component of ARCHITECTURE, by what it is today.\n" {
		t.Errorf("an absent document: %q", got)
	}
	os.WriteFile(filepath.Join(system, "docs", "ARCHITECTURE.md"), []byte("# Architecture\n"), 0o644)
	if got := archLinks(page, system, reports); !strings.Contains(got, "[ARCHITECTURE](../corpus/ARCHITECTURE.md)") {
		t.Errorf("a published corpus: %q", got)
	}
	if err := os.WriteFile(filepath.Join(system, ".gitattributes"), []byte("docs/** filter=git-crypt diff=git-crypt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := archLinks(page, system, reports); got != "Each component of ARCHITECTURE, by what it is today.\n" {
		t.Errorf("withheld with no forge, the link is not plain text: %q", got)
	}
	if out, err := exec.Command("git", "-C", system, "remote", "add", "origin", "https://gitlab.example.com/acme/system.git").CombinedOutput(); err != nil {
		t.Fatalf("git remote: %v: %s", err, out)
	}
	if got := archLinks(page, system, reports); !strings.Contains(got, "[ARCHITECTURE](https://gitlab.example.com/acme/system/-/blob/HEAD/docs/ARCHITECTURE.md)") {
		t.Errorf("withheld with a forge: %q", got)
	}
	// The web root the program cites under, when set, is where the forge
	// is, whatever host the origin names (GH-141).
	t.Setenv("SCRUM_MASTER_WEB_BASE", "https://git.example.org/")
	if got := archLinks(page, system, reports); !strings.Contains(got, "[ARCHITECTURE](https://git.example.org/acme/system/-/blob/HEAD/docs/ARCHITECTURE.md)") {
		t.Errorf("withheld under a web base: %q", got)
	}
}
