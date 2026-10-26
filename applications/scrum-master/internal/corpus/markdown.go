// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package corpus reads a program repository's specification documents
// (srd039). A
// corpus is Markdown in the format the pinned declarative-agents
// release declares (its eng10): frontmatter, then headings each tagged by
// the marker comment above it (design.md D2). scrum-master's configuration
// files are config documents in the same marker form (design.md D11).
package corpus

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Document is a Markdown corpus document: its frontmatter and its tagged
// sections, nested by heading level.
type Document struct {
	Front    map[string]any
	Sections []*Section
}

// Section is one tagged heading and the text below it up to the next
// heading. Kind is "field" for a field, whose Key is the field name, or a
// record kind such as release or req, whose Key is the record id.
type Section struct {
	Level    int
	Kind     string
	Key      string
	Attrs    map[string]string
	Heading  string
	Body     string
	Children []*Section
}

// ParseMarkdown reads a Markdown corpus document. It reads structure only:
// whether the document satisfies its declaration is the audit's question.
func ParseMarkdown(data []byte) (*Document, error) {
	doc := &Document{Front: map[string]any{}}
	front, body, err := splitFrontmatter(data)
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal([]byte(front), &doc.Front); err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}
	var (
		stack   []*Section
		current *Section
		pending *marker
		text    strings.Builder
		fence   string
	)
	flush := func() {
		if current != nil {
			current.Body = strings.TrimSpace(text.String())
		}
		text.Reset()
	}
	s := bufio.NewScanner(bytes.NewReader(body))
	s.Buffer(make([]byte, 0, 64*1024), len(body)+1)
	for s.Scan() {
		line := s.Text()
		trimmed := strings.TrimSpace(line)
		if fence != "" || strings.HasPrefix(trimmed, "```") {
			fence = toggleFence(fence, trimmed)
			text.WriteString(line + "\n")
			continue
		}
		if m, ok := parseMarker(trimmed); ok {
			pending = m
			continue
		}
		level, heading, ok := parseHeading(trimmed)
		if !ok {
			text.WriteString(line + "\n")
			continue
		}
		flush()
		section := &Section{Level: level, Heading: heading, Attrs: map[string]string{}}
		if pending != nil {
			section.Kind, section.Key, section.Attrs = pending.Kind, pending.Key, pending.Attrs
			pending = nil
		}
		for len(stack) > 0 && stack[len(stack)-1].Level >= level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			doc.Sections = append(doc.Sections, section)
		} else {
			parent := stack[len(stack)-1]
			parent.Children = append(parent.Children, section)
		}
		stack = append(stack, section)
		current = section
	}
	flush()
	return doc, s.Err()
}

// ID is the document's frontmatter id.
func (d *Document) ID() string {
	id, _ := d.Front["id"].(string)
	return id
}

// Field returns the document-level field named key.
func (d *Document) Field(key string) *Section { return field(d.Sections, key) }

// Records returns every record of kind, at any depth, in document order.
func (d *Document) Records(kind string) []*Section {
	var out []*Section
	var walk func([]*Section)
	walk = func(sections []*Section) {
		for _, s := range sections {
			if s.Kind == kind {
				out = append(out, s)
			}
			walk(s.Children)
		}
	}
	walk(d.Sections)
	return out
}

// HasKey reports whether a section anywhere is the record or field named
// key: a requirement group (R2), an item (R2.1), a release, or an interface.
func (d *Document) HasKey(key string) bool {
	var walk func([]*Section) bool
	walk = func(sections []*Section) bool {
		for _, s := range sections {
			if s.Key == key || walk(s.Children) {
				return true
			}
		}
		return false
	}
	return walk(d.Sections)
}

// Field returns the field named key directly under the section; a nil
// section has none, so a chain of Field calls over a record that lacks a
// field reads as empty instead of failing.
func (s *Section) Field(key string) *Section {
	if s == nil {
		return nil
	}
	return field(s.Children, key)
}

func field(sections []*Section, key string) *Section {
	for _, s := range sections {
		if s.Kind == "field" && s.Key == key {
			return s
		}
	}
	return nil
}

// Text is the section's body with reference links read as their text; a
// nil section, or an empty field the release prints as None, has none.
func (s *Section) Text() string {
	if s == nil || s.Body == "None" {
		return ""
	}
	return unlink(s.Body)
}

// List returns the body's bullet items.
func (s *Section) List() []string {
	var out []string
	for _, line := range strings.Split(s.Text(), "\n") {
		if item, ok := strings.CutPrefix(strings.TrimSpace(line), "- "); ok {
			out = append(out, strings.TrimSpace(item))
		}
	}
	return out
}

// Tree reads a field the release prints in tree form, "- key: value" with
// a nested list under a key, as the map it holds. A field with no tree
// holds an empty map.
func (s *Section) Tree() map[string]any {
	out, _ := s.TreeErr()
	return out
}

// TreeErr is Tree with the reason a tree that does not parse holds no map,
// such as a value with a colon in it that is not quoted. A nested list
// item written "- item" rather than "1. item" still reads, but is an
// error: the parent spec tool's formatter refuses it as an entry with no
// key, so the gate refuses it too (srd045 R6.1).
func (s *Section) TreeErr() (map[string]any, error) {
	var b strings.Builder
	var keyless error
	for _, line := range strings.Split(s.Text(), "\n") {
		trimmed := strings.TrimLeft(line, " ")
		indent := len(line) - len(trimmed)
		switch {
		case indent == 0 && strings.HasPrefix(trimmed, "- "):
			b.WriteString(trimmed[2:] + "\n")
		case indent > 0 && orderedItem.MatchString(trimmed):
			b.WriteString(strings.Repeat(" ", indent) + "- " + orderedItem.ReplaceAllString(trimmed, "") + "\n")
		case indent > 0:
			if item, ok := strings.CutPrefix(trimmed, "- "); ok && keyless == nil && !strings.Contains(item, ": ") {
				keyless = fmt.Errorf("entry %q has no key; write a nested list item as \"1. %s\"", item, item)
			}
			b.WriteString(line + "\n")
		}
	}
	out := map[string]any{}
	if err := yaml.Unmarshal([]byte(b.String()), &out); err != nil {
		return map[string]any{}, fmt.Errorf("%w; quote a value that holds a colon", err)
	}
	return out, keyless
}

// TreeString returns a tree item's scalar value as text.
func TreeString(tree map[string]any, key string) string {
	if v, ok := tree[key]; ok && v != nil {
		if _, list := v.([]any); !list {
			return fmt.Sprint(v)
		}
	}
	return ""
}

// TreeList returns a tree item's list value as text.
func TreeList(tree map[string]any, key string) []string {
	items, _ := tree[key].([]any)
	if items == nil {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, v := range items {
		out = append(out, fmt.Sprint(v))
	}
	return out
}

var (
	// referenceLink is a reference the release renders as a Markdown link;
	// the corpus reads it back as its text.
	referenceLink = regexp.MustCompile(`\[([^\[\]]+)\]\(([^()\s]*)\)`)
	orderedItem   = regexp.MustCompile(`^[0-9]+\. `)
)

func unlink(text string) string { return referenceLink.ReplaceAllString(text, "$1") }

// Table returns the body's pipe table as one map per row, keyed by the
// header; an escaped pipe in a cell is a literal pipe.
func (s *Section) Table() []map[string]string {
	var rows [][]string
	for _, line := range strings.Split(s.Text(), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "|") {
			rows = append(rows, splitRow(line))
		}
	}
	if len(rows) < 2 {
		return nil
	}
	var out []map[string]string
	for _, cells := range rows[2:] {
		row := map[string]string{}
		for i, key := range rows[0] {
			if i < len(cells) {
				row[key] = cells[i]
			}
		}
		out = append(out, row)
	}
	return out
}

// File is one corpus document as fetched and parsed.
type File struct {
	Path     string
	Markdown *Document
}

// Get reads and parses the Markdown document at path from project at ref.
// A missing document's error wraps tracker.ErrNotFound.
func Get(t tracker.Forge, project, path, ref string) (File, error) {
	b, err := t.GetFile(project, path, ref)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	return Decode(path, b)
}

// Decode parses a Markdown corpus document read from path.
func Decode(path string, content []byte) (File, error) {
	if !strings.HasSuffix(path, ".md") {
		return File{}, fmt.Errorf("%s is not a Markdown document", path)
	}
	doc, err := ParseMarkdown(content)
	if err != nil {
		return File{}, fmt.Errorf("%s: %w", path, err)
	}
	return File{Path: path, Markdown: doc}, nil
}

// Documents keeps the Markdown documents of a directory listing.
func Documents(paths []string) []string {
	var out []string
	for _, p := range paths {
		if strings.HasSuffix(p, ".md") {
			out = append(out, p)
		}
	}
	return out
}

type marker struct {
	Kind, Key string
	Attrs     map[string]string
}

// parseMarker reads a marker comment, <!-- tag: value | key: value -->; a
// license comment is not one.
func parseMarker(line string) (*marker, bool) {
	body, ok := strings.CutPrefix(line, "<!--")
	if !ok {
		return nil, false
	}
	if body, ok = strings.CutSuffix(body, "-->"); !ok {
		return nil, false
	}
	segments := strings.Split(strings.TrimSpace(body), " | ")
	kind, key, ok := strings.Cut(segments[0], ":")
	if !ok || kind != strings.ToLower(kind) || strings.ContainsAny(kind, " -") {
		return nil, false
	}
	m := &marker{Kind: strings.TrimSpace(kind), Key: strings.TrimSpace(key), Attrs: map[string]string{}}
	for _, segment := range segments[1:] {
		if k, v, ok := strings.Cut(segment, ":"); ok {
			m.Attrs[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m, true
}

func parseHeading(line string) (int, string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || level >= len(line) || line[level] != ' ' {
		return 0, "", false
	}
	return level, strings.TrimSpace(line[level:]), true
}

// toggleFence opens a fence on a backtick run, or closes the open one on a
// run at least as long.
func toggleFence(open, line string) string {
	run := line[:len(line)-len(strings.TrimLeft(line, "`"))]
	switch {
	case open == "" && len(run) >= 3:
		return run
	case open != "" && len(run) >= len(open) && strings.TrimSpace(line[len(run):]) == "":
		return ""
	}
	return open
}

func splitFrontmatter(data []byte) (string, []byte, error) {
	text := string(data)
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return "", nil, errors.New("no frontmatter: a corpus document opens with ---")
	}
	front, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return "", nil, errors.New("frontmatter is not closed by ---")
	}
	return front, []byte(body), nil
}

// splitRow splits a pipe-table row on unescaped pipes.
func splitRow(line string) []string {
	inner := strings.TrimSuffix(strings.TrimPrefix(line, "|"), "|")
	var cells []string
	var cell strings.Builder
	for i := 0; i < len(inner); i++ {
		switch {
		case inner[i] == '\\' && i+1 < len(inner) && inner[i+1] == '|':
			cell.WriteByte('|')
			i++
		case inner[i] == '|':
			cells = append(cells, strings.TrimSpace(cell.String()))
			cell.Reset()
		default:
			cell.WriteByte(inner[i])
		}
	}
	return append(cells, strings.TrimSpace(cell.String()))
}
