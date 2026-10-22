// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"gopkg.in/yaml.v3"
)

// A Finding is one diagnostic: a stable kebab-case name, where it is, what
// the rule expected and what the document has, the declaration rule it
// enforces, and how to fix it. Tier 0 (canonical form) is never a finding:
// the formatter fixes it.
type Finding struct {
	Name     string
	Tier     int
	File     string
	Line     int
	Expected string
	Found    string
	Rule     string
	Hint     string
}

func (f Finding) String() string {
	return fmt.Sprintf("%s:%d: %s: expected %s, found %s [design.yaml %s]; %s",
		f.File, f.Line, f.Name, f.Expected, f.Found, f.Rule, f.Hint)
}

// findingCatalog is every finding name the diagnostics can report, by tier.
var findingCatalog = map[string]int{
	"document-type-unknown": 1, "frontmatter-missing": 1, "frontmatter-invalid": 1,
	"frontmatter-missing-key": 1, "frontmatter-unknown-key": 1, "frontmatter-id-mismatch": 1,
	"marker-malformed": 1, "marker-unknown-tag": 1, "marker-unknown-attribute": 1, "marker-orphan": 1,
	"heading-untagged": 2, "heading-misplaced": 2, "id-malformed": 2, "id-outside-parent": 2,
	"heading-id-mismatch": 2, "duplicate-id": 2, "field-disallowed": 2, "field-duplicate": 2,
	"content-untagged": 2, "untagged-structure": 2, "missing-content": 2,
	"missing-required-field": 2, "missing-required-node": 2, "field-undeclared": 2, "field-out-of-order": 2,
	"field-shape-mismatch": 3, "list-item-pattern": 3, "table-columns-mismatch": 3, "attribute-shape": 3,
	"prose-containment": 4,
}

// maxHeadingLevel bounds the node stack: Markdown headings stop at level 6.
const maxHeadingLevel = 6

// dnode is a node the diagnostics have opened from a marker and heading.
type dnode struct {
	kind       string
	tag        string
	id         string
	level      int
	line       int
	markerLine int
	fields     map[string]bool
	content    []Block
	form       string // a field's typed form: its content is data, not prose
	orderAt    int    // one past the slot index of the last field in order
	lastField  string // that field
}

// diagnosis walks one document collecting findings; it never stops early.
type diagnosis struct {
	rules    *Declaration
	file     string
	lines    []string
	offset   int
	findings []Finding
	pending  *Marker
	pendLine int
	doc      *dnode
	stack    [maxHeadingLevel + 1]*dnode
	open     *dnode
	// stray is set under a heading already reported as heading-untagged, so
	// its content is not reported a second time as untagged content.
	stray  bool
	ids    map[string]int
	counts map[string]int
	status string
}

// Diagnose reports every finding in a Markdown corpus document, against the
// declaration whose location matches its path. The caller loads the
// declarations from design.yaml; the package does not search for it.
func Diagnose(declarations Declarations, data []byte, source string) []Finding {
	r, ok := declarationFor(declarations, source)
	if !ok {
		return []Finding{{Name: "document-type-unknown", Tier: 1, File: source, Line: 1,
			Expected: "a file name matching a declared Markdown location", Found: filepath.Base(source),
			Rule: "document_types.*.location",
			Hint: "name the file after its document type, or declare a location for the type"}}
	}
	return r.Diagnose(data, source)
}

// Diagnose reports every finding in a Markdown document of this declaration's
// type.
func (r *Declaration) Diagnose(data []byte, source string) []Finding {
	d := &diagnosis{rules: r, file: source, ids: map[string]int{}, counts: map[string]int{},
		doc: &dnode{kind: "document", fields: map[string]bool{}}}
	body, offset := d.frontmatter(data)
	d.offset = offset
	blocks, lines := topLevelBlocks(body)
	d.lines = lines
	for _, b := range blocks {
		d.step(b)
	}
	d.closeOpen()
	d.finishFrom(2)
	if d.pending != nil {
		d.orphan()
	}
	d.documentLevel()
	sort.SliceStable(d.findings, func(i, j int) bool { return d.findings[i].Line < d.findings[j].Line })
	return d.findings
}

// add records a finding; rule is a path inside this type's markdown block.
func (d *diagnosis) add(name string, line int, expected, found, rule, hint string) {
	d.findings = append(d.findings, Finding{
		Name: name, Tier: findingCatalog[name], File: d.file, Line: line,
		Expected: expected, Found: found,
		Rule: "document_types." + d.rules.documentType + ".markdown." + rule, Hint: hint,
	})
}

func (d *diagnosis) fileLine(b Block) int { return b.Start + d.offset + 1 }

func (d *diagnosis) step(b Block) {
	if text, ok := commentLine(b, d.lines); ok {
		d.comment(b, text)
		return
	}
	if level, heading, ok := headingOf(b, d.lines); ok {
		d.heading(b, level, heading)
		return
	}
	if d.open == nil {
		if d.stray {
			return
		}
		d.add("content-untagged", d.fileLine(b), "content under a tagged heading",
			"content before any marker and heading", "nodes",
			"put a marker and heading above it, or move it under the section it belongs to")
		return
	}
	d.open.content = append(d.open.content, b)
}

func (d *diagnosis) comment(b Block, text string) {
	m, isMarker, err := parseMarker(text)
	switch {
	case err != nil:
		d.add("marker-malformed", d.fileLine(b), "<!-- <tag>: <value> | <key>: <value> -->",
			text, "nodes", "write the tag and its value separated by a colon, attributes after \" | \"")
		return
	case !isMarker:
		return
	}
	if tags := d.rules.tags(); !contains(tags, m.Tag) {
		d.add("marker-unknown-tag", d.fileLine(b), "a declared tag ("+strings.Join(tags, ", ")+")",
			m.Tag, "nodes", suggest(m.Tag, tags))
		return
	}
	if d.pending != nil {
		d.orphan()
	}
	d.pending, d.pendLine = &m, d.fileLine(b)
}

func (d *diagnosis) orphan() {
	d.add("marker-orphan", d.pendLine, "a heading directly below the marker",
		"no heading before the next marker or the end of the document", "nodes",
		"add the heading the marker tags, or delete the marker")
	d.pending = nil
}

// suggest names the nearest declared value, when one is close.
func suggest(found string, known []string) string {
	best, bestDistance := "", len(found)/2+2
	for _, candidate := range known {
		if distance := editDistance(found, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	if best == "" {
		return "use one of " + strings.Join(known, ", ")
	}
	return fmt.Sprintf("did you mean %q?", best)
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

// frontmatter checks the frontmatter and returns the body and its line
// offset in the file.
func (d *diagnosis) frontmatter(data []byte) ([]byte, int) {
	text := string(data)
	if !strings.HasPrefix(text, "---\n") {
		d.add("frontmatter-missing", 1, "a \"---\" frontmatter block first", "no frontmatter",
			"frontmatter", "start the file with ---, id and title, then ---")
		return data, 0
	}
	closing := strings.Index(text[4:], "\n---\n")
	if closing < 0 {
		d.add("frontmatter-missing", 1, "a closing \"---\"", "an unterminated frontmatter block",
			"frontmatter", "close the frontmatter with a line holding only ---")
		return data, 0
	}
	head := text[4 : 4+closing]
	d.frontmatterKeys(head)
	return []byte(text[4+closing+5:]), strings.Count(head, "\n") + 3
}

func (d *diagnosis) frontmatterKeys(head string) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(head), &doc); err != nil || len(doc.Content) == 0 ||
		doc.Content[0].Kind != yaml.MappingNode {
		d.add("frontmatter-invalid", 2, "a YAML mapping", "text that is not a YAML mapping",
			"frontmatter", "write the frontmatter as key: value lines")
		return
	}
	root := doc.Content[0]
	seen := map[string]string{}
	allowed := d.rules.Frontmatter.Allowed
	for i := 0; i+1 < len(root.Content); i += 2 {
		key := root.Content[i]
		seen[key.Value] = root.Content[i+1].Value
		if !contains(allowed, key.Value) {
			d.add("frontmatter-unknown-key", key.Line+1, "one of "+strings.Join(allowed, ", "),
				key.Value, "frontmatter.allowed", suggest(key.Value, allowed))
		}
	}
	d.status = seen["status"]
	for _, key := range d.rules.Frontmatter.Required {
		if _, ok := seen[key]; !ok {
			d.add("frontmatter-missing-key", 2, "frontmatter key "+key, "none", "frontmatter.required",
				"add \""+key+": ...\" to the frontmatter")
		}
	}
	want := strings.TrimSuffix(filepath.Base(d.file), filepath.Ext(d.file))
	if id, ok := seen["id"]; ok && d.rules.Frontmatter.IDMatchesFilename && id != want {
		d.add("frontmatter-id-mismatch", frontmatterLine(root, "id"), "id "+want+" (the file name)", id,
			"frontmatter.id_matches_filename", "rename the file or change the id so they agree")
	}
}

func frontmatterLine(root *yaml.Node, key string) int {
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == key {
			return root.Content[i].Line + 1
		}
	}
	return 2
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

// exempt reports a stub whose status lifts the required-content checks.
func (d *diagnosis) exempt() bool {
	return d.status != "" && contains(d.rules.ExemptStatus, d.status)
}

// documentLevel checks what the whole document must carry.
func (d *diagnosis) documentLevel() {
	if d.exempt() {
		return
	}
	end := len(d.lines) + d.offset
	if len(d.lines) > 0 && d.lines[len(d.lines)-1] == "" {
		end--
	}
	for _, key := range d.rules.RequiredFields {
		if !d.doc.fields[key] {
			d.add("missing-required-field", end, "a "+key+" field", "none", "required_fields",
				"add <!-- field: "+key+" --> with a level-2 heading and its content")
		}
	}
	kinds := make([]string, 0, len(d.rules.RequiredNodes))
	for kind := range d.rules.RequiredNodes {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		if want := d.rules.RequiredNodes[kind]; d.counts[kind] < want {
			node := d.rules.Nodes[kind]
			d.add("missing-required-node", end, fmt.Sprintf("at least %d %s", want, kind),
				fmt.Sprintf("%d", d.counts[kind]), "required_nodes."+kind,
				fmt.Sprintf("add a <!-- %s: <id> --> marker above a level-%d heading", node.Tag, node.Level))
		}
	}
}

// containsStructure reports whether any block is a table or a list:
// structure a prose node must carry under its own tagged field.
func containsStructure(blocks []Block) (Block, string, bool) {
	for _, b := range blocks {
		switch b.Node.(type) {
		case *east.Table:
			return b, "table", true
		case *ast.List:
			return b, "list", true
		}
	}
	return Block{}, "", false
}
