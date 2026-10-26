// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package links

import (
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker/fake"
)

func TestParseCitesReadsEveryLineAndAsk(t *testing.T) {
	body := "Do the thing\n\nCites: https://h/g/r/-/blob/v1.0.0/a.md#R1.1\n  Cites: https://h/g/r/-/blob/v1.0.0/b.md#R2 | review\nEnvironment: dev\n"
	c := ParseCites(body)
	if len(c) != 2 || c[0].URL != "https://h/g/r/-/blob/v1.0.0/a.md#R1.1" || c[1].Ask != "review" || c[1].String() != "Cites: https://h/g/r/-/blob/v1.0.0/b.md#R2 | review" {
		t.Fatalf("%+v", c)
	}
}

const srdMarkdown = `---
id: srd003-ingest
title: Ingest
---

<!-- req: R2 -->
## R2 - Ingest

<!-- req: R2.1 -->
### R2.1

The reader accepts records.

` + "```yaml\n<!-- req: R9.9 -->\n## R9.9\n```" + `

<!-- field: interfaces -->
## Interfaces

<!-- interface: reader-api -->
### reader-api
`

func TestResolve(t *testing.T) {
	f := fake.New()
	f.AddTag("g/system", "v1.4.0")
	f.AddFile("g/system", "v1.4.0", "docs/srd003.md", []byte(srdMarkdown))
	base := "https://gitlab.example.com/g/system/-/blob/"
	cases := []struct{ url, want string }{
		{base + "v1.4.0/docs/missing.md#R2.1", "does not exist at v1.4.0"},
		{base + "v1.4.0/docs/srd003.md#R2.1", ""},
		{base + "v1.4.0/docs/srd003.md#R2", ""},
		{base + "v1.4.0/docs/srd003.md#reader-api", ""},
		{base + "v1.4.0/docs/srd003.md#R7.1", `no entry "R7.1"`},
		{base + "v1.4.0/docs/srd003.md#R9.9", `no entry "R9.9"`},
	}
	for _, c := range cases {
		r, err := Parse(c.url)
		if err != nil {
			t.Fatal(err)
		}
		err = Resolve(f, r)
		if c.want == "" && err != nil {
			t.Errorf("%s: %v", c.url, err)
		}
		if c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)) {
			t.Errorf("%s: err = %v, want %q", c.url, err, c.want)
		}
	}
}
