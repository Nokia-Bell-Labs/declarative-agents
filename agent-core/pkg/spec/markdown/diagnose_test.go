// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"strings"
	"testing"
)

// validSRD is a minimal canonical SRD; each case below mutates it to provoke
// exactly the finding it names. Line numbers in the cases refer to the
// mutated document.
const validSRD = `---
id: srd900-sample
title: Sample
---
<!-- Copyright (c) 2026 Nokia -->
<!-- SPDX-License-Identifier: BSD-3-Clause -->

<!-- field: problem -->
## Problem

Something is missing.

<!-- field: goals -->
## Goals

- G1: Fix it.

<!-- req: R1 -->
## R1 - Contract

<!-- req: R1.1 -->
### R1.1

The kit shall export one constant.

<!-- field: endpoints -->
#### Endpoints

| path | owner |
| --- | --- |
| GET /state | srd033 R3.9 |

<!-- field: non_goals -->
## Non-goals

- Rewriting the shell.

<!-- field: acceptance_criteria -->
## Acceptance criteria

<!-- ac: AC1 -->
### AC1

A kit test fails on an unknown endpoint.

<!-- field: traces -->
#### Traces

- R1.1
`

const validName = "docs/specs/software-requirements/srd900-sample.md"

func TestValidSRDHasNoFindings(t *testing.T) {
	if findings := diagnose(t, []byte(validSRD), validName); len(findings) > 0 {
		t.Fatalf("valid document reported findings:\n%v", findings)
	}
}

type diagnosticCase struct {
	name string
	doc  string
	file string
	line int
	hint string
}

func replaceOnce(old, replacement string) string {
	return strings.Replace(validSRD, old, replacement, 1)
}

// lastLine marks a finding reported at the document's last line.
const lastLine = -1

func diagnosticCases() []diagnosticCase {
	group := validSRD[strings.Index(validSRD, "<!-- req: R1 -->"):strings.Index(validSRD, "<!-- field: non_goals -->")]
	return []diagnosticCase{
		{"document-type-unknown", validSRD, "notes.md", 1, "name the file after its document type"},
		{"frontmatter-missing", strings.SplitN(validSRD, "---\n", 3)[2], "", 1, "start the file with ---"},
		{"frontmatter-invalid", replaceOnce("title: Sample", "title: [unclosed"), "", 2, "key: value lines"},
		{"frontmatter-missing-key", replaceOnce("title: Sample\n", ""), "", 2, `add "title: ..."`},
		{"frontmatter-unknown-key", replaceOnce("title: Sample\n", "title: Sample\nstatuss: draft\n"), "", 4, `did you mean "status"?`},
		{"frontmatter-id-mismatch", validSRD, "docs/specs/software-requirements/srd901-other.md", 2, "rename the file or change the id"},
		{"marker-malformed", replaceOnce("<!-- field: goals -->", "<!-- field goals -->"), "", 13, "separated by a colon"},
		{"marker-unknown-tag", replaceOnce("<!-- field: goals -->", "<!-- feild: goals -->"), "", 13, `did you mean "field"?`},
		{"marker-unknown-attribute", replaceOnce("<!-- req: R1.1 -->", "<!-- req: R1.1 | statsu: retired -->"), "", 21, `did you mean "status"?`},
		{"marker-orphan", replaceOnce("<!-- field: goals -->", "<!-- field: notes -->\n<!-- field: goals -->"), "", 13, "add the heading the marker tags"},
		{"heading-untagged", replaceOnce("<!-- field: problem -->\n", ""), "", 8, "add the marker"},
		{"heading-misplaced", replaceOnce("#### Endpoints", "##### Endpoints"), "", 27, "change the heading depth"},
		{"id-malformed", replaceOnce("<!-- req: R1.1 -->\n### R1.1", "<!-- req: R1.x -->\n### R1.x"), "", 22, "use the declared id form"},
		{"id-outside-parent", replaceOnce("<!-- req: R1.1 -->\n### R1.1", "<!-- req: R2.1 -->\n### R2.1"), "", 22, "move the node under the parent its id names"},
		{"heading-id-mismatch", replaceOnce("### R1.1", "### R1.2"), "", 22, "make the heading say exactly the id"},
		{"duplicate-id", replaceOnce("<!-- field: non_goals -->", "<!-- req: R1.1 -->\n### R1.1\n\nAgain.\n\n<!-- field: non_goals -->"), "", 34, "give the second node a new id"},
		{"field-disallowed", replaceOnce("<!-- field: endpoints -->", "<!-- field: goals -->"), "", 27, "move the section to where the field belongs"},
		{"field-duplicate", replaceOnce("<!-- req: R1 -->", "<!-- field: goals -->\n## Goals\n\n- G2: More.\n\n<!-- req: R1 -->"), "", 19, "merge the two sections"},
		{"content-untagged", replaceOnce("## R1 - Contract\n", "## R1 - Contract\n\nStray text.\n"), "", 21, "tag it with <!-- field: <key> -->"},
		{"untagged-structure", replaceOnce("The kit shall export one constant.\n", "The kit shall export one constant.\n\n- a bullet\n"), "", 26, "move the list under its own"},
		{"missing-content", replaceOnce("The kit shall export one constant.\n\n", ""), "", 22, "write the requirement_item text"},
		{"missing-required-field", replaceOnce("<!-- field: non_goals -->\n## Non-goals\n\n- Rewriting the shell.\n\n", ""), "", lastLine, "add <!-- field: non_goals -->"},
		{"missing-required-field", replaceOnce("\n<!-- field: traces -->\n#### Traces\n\n- R1.1\n", ""), "", 42, "add <!-- field: traces --> with a level-4 heading under AC1"},
		{"missing-required-node", strings.Replace(validSRD, group, "", 1), "", lastLine, "add a <!-- req: <id> --> marker above a level-2 heading"},
		{"field-shape-mismatch", replaceOnce("- G1: Fix it.", "G1: Fix it."), "", 16, "write goals as list"},
		{"list-item-pattern", replaceOnce("- G1: Fix it.", "- Fix it."), "", 16, "rewrite the item in the declared form"},
		{"list-item-pattern", replaceOnce("- R1.1\n", "- X9\n"), "", 49, "rewrite the item in the declared form"},
		{"table-columns-mismatch", replaceOnce("| path | owner |", "| path | team |"), "", 29, "use exactly the declared columns"},
		{"attribute-shape", replaceOnce("<!-- req: R1.1 -->", "<!-- req: R1.1 | weight: heavy -->"), "", 21, "write the value in its declared shape"},
		{"prose-containment", replaceOnce("Something is missing.", "The system SHALL do it."), "", 11, "move the obligation into a requirement item"},
	}
}

// designCase is a fixture diagnosed against its own declarations: the SRD
// slots list no fields yet, so the order checks run on a type that does
// (GH-2654).
type designCase struct {
	diagnosticCase
	design string
}

func designDiagnosticCases() []designCase {
	return []designCase{
		{diagnosticCase{"field-undeclared", string(planDoc(overviewOpen, summaryField, strangerField, notesField)),
			"docs/plans/plan.md", 15, "declare stranger in nodes.document_field.fields"}, orderedDesign},
		{diagnosticCase{"field-out-of-order", string(planDoc(notesField, overviewOpen, summaryField)),
			"docs/plans/plan.md", 12, "move overview ahead of notes"}, orderedDesign},
	}
}

// TestEveryFindingIsDiagnosedAtItsLineWithAHint provokes each finding name
// from one fixture and checks its name, file line, and fix hint (GH-2589 AC2).
func TestEveryFindingIsDiagnosedAtItsLineWithAHint(t *testing.T) {
	cases := []designCase{}
	for _, tc := range diagnosticCases() {
		cases = append(cases, designCase{diagnosticCase: tc})
	}
	for _, dc := range append(cases, designDiagnosticCases()...) {
		tc := dc.diagnosticCase
		t.Run(tc.name, func(t *testing.T) {
			file := tc.file
			if file == "" {
				file = validName
			}
			findings := diagnose(t, []byte(tc.doc), file)
			if dc.design != "" {
				declarations, err := LoadDeclarations([]byte(dc.design))
				if err != nil {
					t.Fatal(err)
				}
				findings = Diagnose(declarations, []byte(tc.doc), file)
			}
			line := tc.line
			if line == lastLine {
				line = strings.Count(tc.doc, "\n")
			}
			for _, f := range findings {
				if f.Name != tc.name {
					continue
				}
				if f.Line != line || !strings.Contains(f.Hint, tc.hint) || f.Tier != findingCatalog[tc.name] {
					t.Fatalf("got %s (tier %d), want line %d, tier %d, hint containing %q", f, f.Tier, line, findingCatalog[tc.name], tc.hint)
				}
				return
			}
			t.Fatalf("no %s finding; got:\n%v", tc.name, findings)
		})
	}
}

// TestStrayHeadingIsOneFinding checks that the content under an untagged
// heading is not reported again as untagged content (GH-2699).
func TestStrayHeadingIsOneFinding(t *testing.T) {
	doc := replaceOnce("Something is missing.\n",
		"Something is missing.\n\n### A free subheading\n\nMore prose.\n\nAnd more.\n")
	findings := diagnose(t, []byte(doc), validName)
	if len(findings) != 1 || findings[0].Name != "heading-untagged" {
		t.Fatalf("want one heading-untagged finding, got:\n%v", findings)
	}
}

func TestEveryCatalogFindingHasAFixture(t *testing.T) {
	covered := map[string]bool{}
	for _, tc := range diagnosticCases() {
		covered[tc.name] = true
	}
	for _, dc := range designDiagnosticCases() {
		covered[dc.name] = true
	}
	for name := range findingCatalog {
		if !covered[name] {
			t.Errorf("finding %s has no diagnostic fixture", name)
		}
	}
	if len(covered) != len(findingCatalog) {
		t.Errorf("%d fixtures for %d catalog names", len(covered), len(findingCatalog))
	}
}
