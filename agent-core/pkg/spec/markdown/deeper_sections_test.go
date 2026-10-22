// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// deeperSectionsDesign is eng10's example of a corpus that breaks a free-text
// section into subsections by declaring one more node kind with the same tag
// one level down, rather than writing untagged headings (GH-2699).
const deeperSectionsDesign = `
document_types:
  guide:
    location: docs/guides/name.md
    markdown:
      frontmatter: {required: [id, title], allowed: [id, title]}
      nodes:
        section:
          tag: section
          level: 2
          parent: document
          id_pattern: '^[a-z0-9]+(-[a-z0-9]+)*$'
          heading: free
          content: any
          yaml: {from: sections, records: list, id: id, title: title, content: content}
        subsection:
          tag: section
          level: 3
          parent: section
          id_pattern: '^[a-z0-9]+(-[a-z0-9]+)*$'
          heading: free
          content: any
          yaml: {from: sections, records: list, id: id, title: title, content: content}
`

// deeperSectionsDoc is the eng10 example document.
const deeperSectionsDoc = `---
id: scorer
title: Scorer
---

<!-- section: processes -->
## Processes

The scorer runs as two processes.

<!-- section: ingest-process -->
### The ingest process

Reads trajectories from the queue.
`

func TestDeeperSectionsPatternIsCanonical(t *testing.T) {
	declarations, err := LoadDeclarations([]byte(deeperSectionsDesign))
	if err != nil {
		t.Fatal(err)
	}
	guide := declarations["guide"]
	if findings := guide.Diagnose([]byte(deeperSectionsDoc), "docs/guides/scorer.md"); len(findings) > 0 {
		t.Fatalf("the pattern's example reported findings:\n%v", findings)
	}
	tree, findings, err := guide.Parse([]byte(deeperSectionsDoc), "docs/guides/scorer.md")
	if err != nil || len(findings) > 0 {
		t.Fatalf("parse: %v %v", err, findings)
	}
	out, err := yaml.Marshal(tree)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "id: ingest-process") || !strings.Contains(string(out), "Reads trajectories from the queue.") {
		t.Fatalf("the subsection is missing from the tree:\n%s", out)
	}
	formatted, findings, err := guide.Format([]byte(deeperSectionsDoc), "docs/guides/scorer.md")
	if err != nil || len(findings) > 0 {
		t.Fatalf("format: %v %v", err, findings)
	}
	if string(formatted) != deeperSectionsDoc {
		line, found, expected := FirstDifference([]byte(deeperSectionsDoc), formatted)
		t.Fatalf("not canonical at line %d: found %q, expected %q", line, found, expected)
	}
}
