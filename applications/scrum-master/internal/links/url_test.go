// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package links

import (
	"strings"
	"testing"
)

func TestCheckPinned(t *testing.T) {
	base := "https://gitlab.example.com/group/system/-/blob/"
	cases := []struct {
		url, want string
	}{
		{base + "v1.4.0/docs/specs/software-requirements/srd003-ingest.md#R2.1", ""},
		{base + "v1.4.0/docs/specs/objectives/obj001-a.md#R1", ""},
		{base + "main/docs/x.md#R1.1", `branch "main"`},
		{base + "3f2a9c1/docs/x.md#R1.1", `commit "3f2a9c1"`},
		{base + "v1.4.0/docs/x.md", "no requirement or interface anchor"},
		{"https://gitlab.example.com/group/system/docs/x.md#R1", "not a /-/blob/ file URL"},
		{"docs/x.md#R1", "not an absolute URL"},
	}
	for _, c := range cases {
		err := CheckPinned(c.url)
		if c.want == "" && err != nil {
			t.Errorf("%s: unexpected %v", c.url, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: err = %v, want %q", c.url, err, c.want)
		}
	}
}

func TestParseSplitsNestedProject(t *testing.T) {
	r, err := Parse("https://gitlab.example.com/a/b/c/-/blob/v2.0.0/docs/s.md#R3.2")
	if err != nil {
		t.Fatal(err)
	}
	if r.Project != "a/b/c" || r.Tag != "v2.0.0" || r.Path != "docs/s.md" || r.Anchor != "R3.2" || r.Host != "gitlab.example.com" {
		t.Fatalf("%+v", r)
	}
}

// GitHub's file URLs read as GitLab's do, and a GitLab group that merely holds
// a segment named blob is not read as GitHub's form (srd007 R5.1, GH-2901).
func TestParseReadsBothForges(t *testing.T) {
	for raw, want := range map[string]Ref{
		"https://github.com/acme/plan/blob/v1.2.3/docs/specs/stories/st001-x.md#R1": {Host: "github.com", Project: "acme/plan", Tag: "v1.2.3", Path: "docs/specs/stories/st001-x.md", Anchor: "R1"},
		"https://github.com/acme/plan/blob/main/docs/a.md":                          {Host: "github.com", Project: "acme/plan", Tag: "main", Path: "docs/a.md"},
		"https://gitlab.com/g/blob/sub/repo/-/blob/v1/docs/a.md#R2":                 {Host: "gitlab.com", Project: "g/blob/sub/repo", Tag: "v1", Path: "docs/a.md", Anchor: "R2"},
	} {
		got, err := Parse(raw)
		if err != nil {
			t.Errorf("Parse(%q): %v", raw, err)
			continue
		}
		want.Raw = raw
		if got != want {
			t.Errorf("Parse(%q) = %+v, want %+v", raw, got, want)
		}
	}
	for _, raw := range []string{
		"https://gitlab.com/g/blob/sub/repo/docs/a.md",
		"https://github.com/acme/plan/tree/main/docs",
		"https://github.com/acme/plan/blob/main",
	} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("Parse(%q) read a page that is no file", raw)
		}
	}
	if err := CheckPinned("https://github.com/acme/plan/blob/main/docs/a.md#R1"); err == nil {
		t.Error("a GitHub URL on a branch reads as pinned")
	}
	if err := CheckPinned("https://github.com/acme/plan/blob/v1.2.3/docs/a.md#R1"); err != nil {
		t.Errorf("a GitHub URL on a tag: %v", err)
	}
}

// A citation is written in the form its host serves, and reads back (GH-2901).
func TestBlobURLFollowsTheHost(t *testing.T) {
	for base, want := range map[string]string{
		"https://github.com/":    "https://github.com/acme/plan/blob/v1/docs/a.md",
		"https://gitlab.example": "https://gitlab.example/acme/plan/-/blob/v1/docs/a.md",
	} {
		got := BlobURL(base, "acme/plan", "v1", "docs/a.md")
		if got != want {
			t.Errorf("BlobURL(%q) = %q, want %q", base, got, want)
		}
		if r, err := Parse(got); err != nil || r.Project != "acme/plan" || r.Tag != "v1" || r.Path != "docs/a.md" {
			t.Errorf("%q reads back as %+v, %v", got, r, err)
		}
	}
}

// The corpus's own tags hold a slash, and a story pinned at one still reads as
// that story, on either forge (srd007 R5.1, R6.5, GH-2901).
func TestARefMayHoldASlash(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/acme/plan/blob/corpus/v0.1.0/docs/specs/stories/st001-x.md#R1",
		"https://gitlab.com/g/plan/-/blob/corpus/v0.1.0/docs/specs/stories/st001-x.md#R1",
	} {
		r, err := Parse(raw)
		if err != nil || r.Tag != "corpus/v0.1.0" || r.Path != "docs/specs/stories/st001-x.md" {
			t.Errorf("Parse(%q) = %+v, %v", raw, r, err)
		}
		if err := CheckTarget(Link{Kind: Implements, Target: raw}); err != nil {
			t.Errorf("CheckTarget(%q): %v", raw, err)
		}
		if k := ImplementsKind(raw); k != "story" {
			t.Errorf("ImplementsKind(%q) = %q, want story", raw, k)
		}
	}
	if r, err := Parse("https://gitlab.com/g/plan/-/blob/v1/README.md"); err != nil || r.Tag != "v1" || r.Path != "README.md" {
		t.Errorf("a file outside docs/ = %+v, %v", r, err)
	}
}
