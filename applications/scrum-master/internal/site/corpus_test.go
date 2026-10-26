// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package site

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The corpus pages keep a link that lands inside docs/, which the site holds
// at the same place, point one that leaves it at the forge, and leave
// absolute links and anchors alone (GH-86).
func TestCorpusLinks(t *testing.T) {
	page := []byte("[the architecture](../../ARCHITECTURE.md#components) [the policy](../../../control/policy.md) [here](#r11) [web](https://example.com/x) ![fig](figure.png)\n")
	got := string(fixLinks(page, "docs/specs/epics/ep001-x.md", "https://gitlab.example.com/g/system/-/blob/HEAD/"))
	for _, want := range []string{"](../../ARCHITECTURE.md#components)", "](https://gitlab.example.com/g/system/-/blob/HEAD/control/policy.md)", "](#r11)", "](https://example.com/x)", "](figure.png)"} {
		if !strings.Contains(got, want) {
			t.Errorf("links lack %q:\n%s", want, got)
		}
	}
	if got := string(fixLinks(page, "docs/specs/epics/ep001-x.md", "")); got != string(page) {
		t.Errorf("with no forge, a link was changed:\n%s", got)
	}
}

// The corpus word copies every document under docs/ to the same place
// under site/corpus/ with an index naming each by its title, and a forge
// origin as git@host:path reads as its web address (GH-86).
func TestCorpusPages(t *testing.T) {
	system := t.TempDir()
	os.MkdirAll(filepath.Join(system, "docs", "specs", "epics"), 0o755)
	os.WriteFile(filepath.Join(system, "docs", "VISION.md"), []byte("---\nid: vision\ntitle: The vision\n---\n\nSee [the policy](../control/policy.md).\n"), 0o644)
	os.WriteFile(filepath.Join(system, "docs", "specs", "epics", "ep001-x.md"), []byte("---\nid: ep001-x\ntitle: X\n---\n\n[vision](../../VISION.md)\n"), 0o644)
	exec.Command("git", "init", "-q", system).Run()
	exec.Command("git", "-C", system, "remote", "add", "origin", "git@github.com:acme/system.git").Run()
	pages, err := CorpusPages(system, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(pages["site/corpus/VISION.md"]); !strings.Contains(got, "(https://github.com/acme/system/blob/HEAD/control/policy.md)") {
		t.Errorf("VISION.md:\n%s", got)
	}
	if got := string(pages["site/corpus/specs/epics/ep001-x.md"]); !strings.Contains(got, "[vision](../../VISION.md)") {
		t.Errorf("the epic:\n%s", got)
	}
	if index := string(pages["site/corpus/index.md"]); !strings.Contains(index, "- [The vision](VISION.md)") || !strings.Contains(index, "- [X](specs/epics/ep001-x.md)") {
		t.Errorf("index:\n%s", index)
	}
	dir := t.TempDir()
	if _, err := PublishTree(dir, CorpusDir, pages, "v1", false); err != nil {
		t.Fatal(err)
	}
	delete(pages, "site/corpus/specs/epics/ep001-x.md")
	out, err := PublishTree(dir, CorpusDir, pages, "v2", false)
	if err != nil || actions(out)["site/corpus/specs/epics/ep001-x.md"] != "removed" {
		t.Fatalf("a document gone from the corpus: %v, %v", actions(out), err)
	}
}
