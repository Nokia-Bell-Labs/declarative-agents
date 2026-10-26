// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitManaged(t *testing.T) {
	doc := "mine above\n" + BlockBegin + "\ntpm text\n" + BlockEnd + "\nmine below\n"
	before, block, after, ok := splitManaged([]byte(doc))
	if !ok || string(before) != "mine above\n" || string(after) != "mine below\n" ||
		string(block) != BlockBegin+"\ntpm text\n"+BlockEnd+"\n" {
		t.Fatalf("split: %q %q %q %v", before, block, after, ok)
	}
	if _, _, _, ok := splitManaged([]byte(BlockBegin + "\nno end\n")); ok {
		t.Fatal("a block without its end line split")
	}
	reworded := "<!-- tpm:begin: an older note -->\nold\n" + BlockEnd + "\n"
	if _, _, _, ok := splitManaged([]byte(reworded)); !ok {
		t.Fatal("a begin line with another note did not open the block")
	}
}

func TestRefreshManaged(t *testing.T) {
	cur := "notes\n" + BlockBegin + "\nold\n" + BlockEnd + "\nmore notes\n"
	fresh := BlockBegin + "\nnew\n" + BlockEnd + "\n"
	got, how, ok := refreshManaged([]byte(cur), []byte(fresh))
	if !ok || how != "refreshed" || string(got) != "notes\n"+BlockBegin+"\nnew\n"+BlockEnd+"\nmore notes\n" {
		t.Fatalf("refresh: %q %s %v", got, how, ok)
	}
	if _, _, ok := refreshManaged([]byte("anything\n"), []byte("no block in scrum-master's text\n")); ok {
		t.Fatal("a file scrum-master writes without a block was treated as an orientation file")
	}
}

func TestRefreshManagedAppends(t *testing.T) {
	fresh := BlockBegin + "\nnew\n" + BlockEnd + "\n"
	for cur, want := range map[string]string{
		"mine\n":     "mine\n\n" + fresh,
		"mine":       "mine\n\n" + fresh,
		"mine\n\n":   "mine\n\n" + fresh,
		"":           fresh,
		"a\n\nb\n\n": "a\n\nb\n\n" + fresh,
	} {
		got, how, ok := refreshManaged([]byte(cur), []byte(fresh))
		if !ok || how != "appended" || string(got) != want {
			t.Errorf("append to %q: %q %s %v, want %q", cur, got, how, ok, want)
		}
	}
}

// olderBlock rewrites an orientation file's scrum-master text as an older release would
// have written it, and adds the owner's notes around the block.
func olderBlock(t *testing.T, path string) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, ok := splitManaged(b)
	if !ok {
		t.Fatalf("%s has no block", path)
	}
	old := "owner's note above\n" + BlockBegin + "\nan older tpm's text\n" + BlockEnd + "\nowner's note below\n"
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestInitRefreshesBlockKeepsNotes(t *testing.T) {
	dir := repo(t)
	if _, err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(dir, "README.md")
	olderBlock(t, readme)
	r, err := Init(InitOptions{Dir: dir})
	if err != nil || actions(r.Files)["README.md"] != "refreshed" {
		t.Fatalf("second init: %v, %v", actions(r.Files)["README.md"], err)
	}
	b, _ := os.ReadFile(readme)
	for _, want := range []string{"owner's note above\n" + BlockBegin, BlockEnd + "\nowner's note below\n", "# Program system repository"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("refreshed README lacks %q:\n%s", want, b)
		}
	}
	if strings.Contains(string(b), "an older tpm's text") {
		t.Errorf("the older block survived:\n%s", b)
	}
	again, err := Init(InitOptions{Dir: dir})
	if err != nil || actions(again.Files)["README.md"] != "kept" {
		t.Fatalf("third init: %v, %v", actions(again.Files)["README.md"], err)
	}
}

func TestInitAppendsToOrientationWithoutBlock(t *testing.T) {
	dir := repo(t)
	if _, err := Init(InitOptions{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(dir, "README.md")
	os.WriteFile(readme, []byte("the owner's own README\n"), 0o644)
	r, err := Init(InitOptions{Dir: dir})
	if err != nil || actions(r.Files)["README.md"] != "appended" {
		t.Fatalf("init over a README without a block: %v, %v", actions(r.Files)["README.md"], err)
	}
	b, _ := os.ReadFile(readme)
	if !strings.HasPrefix(string(b), "the owner's own README\n\n"+BlockBegin+"\n") || !strings.Contains(string(b), "# Program system repository") {
		t.Fatalf("appended README:\n%s", b)
	}
	again, err := Init(InitOptions{Dir: dir})
	if err != nil || actions(again.Files)["README.md"] != "kept" {
		t.Fatalf("second init: %v, %v", actions(again.Files)["README.md"], err)
	}
	vision := filepath.Join(dir, "docs", "VISION.md")
	os.WriteFile(vision, []byte("the owner's vision\n"), 0o644)
	os.WriteFile(readme, []byte("the owner's own README\n"), 0o644)
	forced, err := Init(InitOptions{Dir: dir, Force: true})
	if err != nil || actions(forced.Files)["README.md"] != "overwritten" {
		t.Fatalf("--force: %v, %v", actions(forced.Files)["README.md"], err)
	}
	if b, _ := os.ReadFile(readme); strings.Contains(string(b), "the owner's own README") {
		t.Fatalf("--force kept the owner's README:\n%s", b)
	}
	os.WriteFile(vision, []byte("the owner's vision\n"), 0o644)
	if r, _ := Init(InitOptions{Dir: dir}); actions(r.Files)["docs/VISION.md"] != "kept" {
		t.Fatalf("an edited VISION was %s", actions(r.Files)["docs/VISION.md"])
	}
}

// A block an earlier release wrote between scrum-master markers is found and
// replaced with the current markers, keeping the owner's text (GH-36).
func TestRefreshReplacesALegacyBlock(t *testing.T) {
	cur := []byte("owner above\n<!-- tpm:begin: scrum-master writes the text -->\nold\n<!-- tpm:end -->\nowner below\n")
	fresh := []byte(BlockBegin + "\nnew\n" + BlockEnd + "\n")
	merged, how, ok := refreshManaged(cur, fresh)
	if !ok || how != "refreshed" || string(merged) != "owner above\n"+BlockBegin+"\nnew\n"+BlockEnd+"\nowner below\n" {
		t.Fatalf("%v %q\n%s", ok, how, merged)
	}
}
