// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package corpus

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// A config document is a configuration file scrum-master reads, in the form
// design D11 states (srd039 R3): each setting a field, a field of fields a mapping, a
// field of records a list of mappings named by their ids, and a leaf a
// table, a bullet list, or a value.

// DecodeConfig reads a config document into v, as yaml.v3 decodes a value:
// the struct tags of v name the settings.
func DecodeConfig(data []byte, v any) error {
	d, err := ParseMarkdown(data)
	if err != nil {
		return err
	}
	return d.Config().Decode(v)
}

// Config is the value the document holds.
func (d *Document) Config() *yaml.Node { return fields(d.Sections) }

func fields(sections []*Section) *yaml.Node {
	m := &yaml.Node{Kind: yaml.MappingNode}
	for _, s := range sections {
		if s.Kind == "field" {
			m.Content = append(m.Content, plain(s.Key), s.value())
		}
	}
	return m
}

func (s *Section) value() *yaml.Node {
	if len(s.Children) > 0 {
		if s.Children[0].Kind == "field" {
			return fields(s.Children)
		}
		list := &yaml.Node{Kind: yaml.SequenceNode}
		for _, r := range s.Children {
			if r.Kind == "field" || r.Kind == "" {
				continue
			}
			entry := fields(r.Children)
			entry.Content = append([]*yaml.Node{plain("name"), plain(r.Key)}, entry.Content...)
			list.Content = append(list.Content, entry)
		}
		return list
	}
	if rows := s.Table(); rows != nil {
		list := &yaml.Node{Kind: yaml.SequenceNode}
		for _, cells := range s.tableCells() {
			row := &yaml.Node{Kind: yaml.MappingNode}
			for _, c := range cells {
				row.Content = append(row.Content, plain(c[0]), plain(c[1]))
			}
			list.Content = append(list.Content, row)
		}
		return list
	}
	if items := s.List(); items != nil {
		list := &yaml.Node{Kind: yaml.SequenceNode}
		for _, item := range items {
			list.Content = append(list.Content, plain(item))
		}
		return list
	}
	return plain(strings.Join(strings.Fields(s.Text()), " "))
}

// tableCells is the table as rows of header and cell pairs, in header
// order, leaving out empty cells.
func (s *Section) tableCells() [][][2]string {
	var rows [][]string
	for _, line := range strings.Split(s.Text(), "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, "|") {
			rows = append(rows, splitRow(line))
		}
	}
	var out [][][2]string
	for _, cells := range rows[2:] {
		var row [][2]string
		for i, key := range rows[0] {
			if i < len(cells) && cells[i] != "" {
				row = append(row, [2]string{key, cells[i]})
			}
		}
		out = append(out, row)
	}
	return out
}

// plain is an untagged scalar, so decoding resolves it as YAML would a
// plain value: an integer, true or false, or text. None is empty.
func plain(v string) *yaml.Node {
	if v == "" || v == "None" {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null"}
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Value: v}
}

// FormatConfig writes v as a config document with the given id and title.
func FormatConfig(id, title string, v any) ([]byte, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("a config document holds a mapping, not %v", n.Kind)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\nid: %s\ntitle: %s\n---\n", id, title)
	writeFields(&b, &n, 2)
	return []byte(b.String()), nil
}

func writeFields(b *strings.Builder, m *yaml.Node, level int) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, v := m.Content[i].Value, m.Content[i+1]
		fmt.Fprintf(b, "\n<!-- field: %s -->\n%s %s\n\n", key, strings.Repeat("#", level), heading(key))
		writeValue(b, key, v, level)
	}
}

func writeValue(b *strings.Builder, key string, v *yaml.Node, level int) {
	switch {
	case v.Kind == yaml.MappingNode:
		writeFields(b, v, level+1)
	case v.Kind == yaml.SequenceNode && len(v.Content) == 0:
		b.WriteString("None\n")
	case v.Kind == yaml.SequenceNode && flatRecords(v):
		var cols []string
		for _, r := range v.Content {
			for i := 0; i < len(r.Content); i += 2 {
				if !contains(cols, r.Content[i].Value) {
					cols = append(cols, r.Content[i].Value)
				}
			}
		}
		b.WriteString("| " + strings.Join(cols, " | ") + " |\n|" + strings.Repeat(" --- |", len(cols)) + "\n")
		for _, r := range v.Content {
			cells := make([]string, len(cols))
			for i := 0; i < len(r.Content); i += 2 {
				for c, col := range cols {
					if col == r.Content[i].Value {
						cells[c] = strings.ReplaceAll(scalar(r.Content[i+1]), "|", `\|`)
					}
				}
			}
			b.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		}
	case v.Kind == yaml.SequenceNode && v.Content[0].Kind == yaml.MappingNode:
		kind := strings.TrimSuffix(key, "s")
		for _, item := range v.Content {
			fmt.Fprintf(b, "<!-- %s: %s -->\n%s %s\n", kind, recordName(item), strings.Repeat("#", level+1), recordName(item))
			rest := &yaml.Node{Kind: yaml.MappingNode}
			for i := 0; i+1 < len(item.Content); i += 2 {
				if item.Content[i].Value != "name" {
					rest.Content = append(rest.Content, item.Content[i], item.Content[i+1])
				}
			}
			writeFields(b, rest, level+2)
			b.WriteString("\n")
		}
	case v.Kind == yaml.SequenceNode:
		for _, item := range v.Content {
			b.WriteString("- " + scalar(item) + "\n")
		}
	default:
		b.WriteString(scalar(v) + "\n")
	}
}

// recordName is the name a record of a list is written under.
func recordName(m *yaml.Node) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == "name" {
			return scalar(m.Content[i+1])
		}
	}
	return "None"
}

func flatRecords(seq *yaml.Node) bool {
	for _, r := range seq.Content {
		if r.Kind != yaml.MappingNode {
			return false
		}
		for i := 1; i < len(r.Content); i += 2 {
			if r.Content[i].Kind != yaml.ScalarNode {
				return false
			}
		}
	}
	return true
}

func scalar(n *yaml.Node) string {
	if n.Tag == "!!null" || n.Value == "" {
		return "None"
	}
	return strings.Join(strings.Fields(n.Value), " ")
}

func heading(key string) string {
	s := strings.ReplaceAll(key, "_", " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
