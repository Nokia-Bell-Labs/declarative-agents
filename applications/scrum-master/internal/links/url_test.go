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
