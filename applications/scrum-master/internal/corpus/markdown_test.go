// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package corpus

import (
	"errors"
	"slices"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

const doc = `---
id: program-roadmap
title: Program road map
---
<!-- Copyright (c) 2026 Example -->

<!-- field: overview -->
## Overview

Two releases.

<!-- release: 01.0 | status: implemented -->
## 01.0 - Objectives

<!-- field: tag -->
### Tag

v0.1.0

<!-- field: objectives -->
### Objectives

- obj001-a#R1.1
- obj001-a#R1.2

<!-- field: rows -->
### Rows

| name | note |
| --- | --- |
| a | one \| two |

<!-- release: 01.1 -->
## 01.1

` + "````md\n<!-- release: 09.9 -->\n## 09.9\n````" + `
`

func TestParseReadsFrontmatterRecordsAndFields(t *testing.T) {
	d, err := ParseMarkdown([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if d.ID() != "program-roadmap" || d.Field("overview").Text() != "Two releases." {
		t.Fatalf("front %v, overview %q", d.Front, d.Field("overview").Text())
	}
	releases := d.Records("release")
	if len(releases) != 2 || releases[0].Key != "01.0" || releases[0].Attrs["status"] != "implemented" || releases[1].Key != "01.1" {
		t.Fatalf("releases = %+v", releases)
	}
	r := releases[0]
	if r.Field("tag").Text() != "v0.1.0" || !slices.Equal(r.Field("objectives").List(), []string{"obj001-a#R1.1", "obj001-a#R1.2"}) {
		t.Fatalf("tag %q objectives %v", r.Field("tag").Text(), r.Field("objectives").List())
	}
	if rows := r.Field("rows").Table(); len(rows) != 1 || rows[0]["note"] != "one | two" {
		t.Fatalf("rows = %v", rows)
	}
	if r.Field("milestone").Text() != "" || r.Field("milestone").List() != nil {
		t.Fatal("a missing field has a value")
	}
	if !d.HasKey("01.1") || !d.HasKey("objectives") || d.HasKey("09.9") {
		t.Fatal("HasKey reads a key inside a fenced block, or misses one outside")
	}
}

func TestParseRefusesADocumentWithoutFrontmatter(t *testing.T) {
	if _, err := ParseMarkdown([]byte("## Overview\n")); err == nil {
		t.Fatal("no error")
	}
}

func TestGetReadsMarkdown(t *testing.T) {
	f := fake.New()
	f.AddFile("g/s", "main", "docs/a.md", []byte("---\nid: a\n---\n"))
	f.AddFile("g/s", "main", "docs/b.yaml", []byte("id: b\n"))
	got, err := Get(f, "g/s", "docs/a.md", "main")
	if err != nil || got.Path != "docs/a.md" || got.Markdown.ID() != "a" {
		t.Fatalf("a: %+v %v", got, err)
	}
	if _, err := Get(f, "g/s", "docs/c.md", "main"); !errors.Is(err, tracker.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if _, err := Get(f, "g/s", "docs/b.yaml", "main"); err == nil {
		t.Fatal("a YAML document read")
	}
}

func TestDocumentsKeepsMarkdown(t *testing.T) {
	got := Documents([]string{"d/a.md", "d/a.yaml", "d/README", "d/c.md"})
	if !slices.Equal(got, []string{"d/a.md", "d/c.md"}) {
		t.Fatalf("documents = %v", got)
	}
}

const declared = `---
id: obj001-a
title: A
---

<!-- field: context | form: "tree" -->
## Context

- owner: technical program manager
- horizon: 2026-W43..2027-W03

<!-- req: R1 -->
## R1 - First

<!-- field: applicability | form: "tree" -->
### Applicability

- due: "2026-11-06"
- delivered_by:
  1. https://h/g/r/-/blob/v1/docs/srd001.md#R1.1
  2. https://h/g/r/-/blob/v1/docs/srd001.md#R1.2

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

Scored.

<!-- field: traces -->
#### Traces

- [R1.1](#r11)
- [srd001-x](../srd001-x.md) [R2.1](../srd001-x.md#r21)

<!-- field: tests -->
#### Tests

None
`

func TestReadsTheDeclaredLayout(t *testing.T) {
	d, err := ParseMarkdown([]byte(declared))
	if err != nil {
		t.Fatal(err)
	}
	ctx := d.Field("context").Tree()
	if TreeString(ctx, "owner") != "technical program manager" || TreeString(ctx, "horizon") != "2026-W43..2027-W03" {
		t.Fatalf("context = %v", ctx)
	}
	app := d.Records("req")[0].Field("applicability").Tree()
	if TreeString(app, "due") != "2026-11-06" || !slices.Equal(TreeList(app, "delivered_by"), []string{"https://h/g/r/-/blob/v1/docs/srd001.md#R1.1", "https://h/g/r/-/blob/v1/docs/srd001.md#R1.2"}) {
		t.Fatalf("applicability = %v", app)
	}
	if TreeString(app, "delivered_by") != "" || TreeList(app, "due") != nil {
		t.Fatal("a scalar read as a list or a list as a scalar")
	}
	ac := d.Records("ac")[0]
	if got := ac.Field("traces").List(); !slices.Equal(got, []string{"R1.1", "srd001-x R2.1"}) {
		t.Fatalf("traces = %v", got)
	}
	if ac.Field("tests").Text() != "" || ac.Field("tests").List() != nil {
		t.Fatal("None reads as a value")
	}
	if len(d.Field("missing").Tree()) != 0 {
		t.Fatal("a missing field has a tree")
	}
}
