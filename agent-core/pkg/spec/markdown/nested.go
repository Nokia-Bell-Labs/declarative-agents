// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package markdown

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// The tree form (eng10, typed forms) prints a map as a bullet list, one
// "- key: value" entry per key, and a list as a numbered list, one
// "1. value" element per item; a nested collection or a multi-line text
// follows its entry on lines indented two spaces under a bullet and three
// under a number. A list element that is a collection or a text starts on its
// marker line ("1. - name: Done"): GitHub reads an empty "1." after a key as
// paragraph text, not a list. Scalars are one-line YAML flow values; a
// multi-line text is Markdown, kept verbatim, so a Mermaid fence inside a map
// draws (GH-2642).
const (
	treeBullet  = "- "
	treeNumber  = "1. "
	bulletWidth = len(treeBullet)
	numberWidth = len(treeNumber)
)

// listOpener matches a line that would open a list item.
var listOpener = regexp.MustCompile(`^(?:[-*+]|[0-9]+[.)])(?: |$)`)

// treeLines prints a map or a list in the tree form.
func treeLines(value *yaml.Node) ([]string, error) {
	if value.Kind != yaml.MappingNode && value.Kind != yaml.SequenceNode || len(value.Content) == 0 {
		return nil, errors.New("the tree form holds a non-empty map or list")
	}
	return treeBlock(value, "")
}

// treeBlock prints a collection at an indentation.
func treeBlock(value *yaml.Node, indent string) ([]string, error) {
	var lines []string
	if value.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(value.Content); i += 2 {
			key := value.Content[i].Value
			printed := key
			if !bareName(key) {
				printed = jsonString(key) // a prose key prints quoted
			}
			entry, err := treeEntry(indent+treeBullet+printed+":", value.Content[i+1], indent+strings.Repeat(" ", bulletWidth), false)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", key, err)
			}
			lines = append(lines, entry...)
		}
		return lines, nil
	}
	for i, item := range value.Content {
		entry, err := treeEntry(indent+strings.TrimRight(treeNumber, " "), item, indent+strings.Repeat(" ", numberWidth), true)
		if err != nil {
			return nil, fmt.Errorf("[%d]: %w", i, err)
		}
		lines = append(lines, entry...)
	}
	return lines, nil
}

// treeEntry prints one entry: the head with an inline flow value, or the
// nested collection or text at childIndent, after the head of a map entry and
// on the marker line of a list element.
func treeEntry(head string, value *yaml.Node, childIndent string, element bool) ([]string, error) {
	if inline, ok := inlineTreeValue(value); ok {
		if element && listOpener.MatchString(inline) {
			inline = jsonString(value.Value)
		}
		return []string{head + " " + inline}, nil
	}
	var nested []string
	var err error
	if value.Kind == yaml.ScalarNode {
		nested, err = treeText(value, childIndent)
		if err != nil && isString(value) && strings.Contains(strings.TrimRight(value.Value, "\n"), "\n") {
			nested, err = literalText(value, childIndent)
			if err == nil {
				return append([]string{head + " " + literalMarker}, nested...), nil
			}
		}
	} else {
		nested, err = treeBlock(value, childIndent)
	}
	if err != nil {
		return nil, err
	}
	if element {
		nested[0] = head + " " + strings.TrimPrefix(nested[0], childIndent)
		return nested, nil
	}
	return append([]string{head}, nested...), nil
}

// inlineTreeValue returns a value the entry's head can carry: a one-line
// flow scalar, or {} and [] for an empty collection.
func inlineTreeValue(value *yaml.Node) (string, bool) {
	switch {
	case value.Kind == yaml.MappingNode && len(value.Content) == 0:
		return "{}", true
	case value.Kind == yaml.SequenceNode && len(value.Content) == 0:
		return "[]", true
	case value.Kind == yaml.ScalarNode:
		text, err := encodeFlowScalar(value)
		return text, err == nil
	}
	return "", false
}

// treeText prints a multi-line string as indented Markdown text. Its first
// line must not read back as a list item or a flow value.
func treeText(value *yaml.Node, indent string) ([]string, error) {
	if !isString(value) {
		return nil, fmt.Errorf("%s does not print inline", value.ShortTag())
	}
	text := strings.TrimRight(value.Value, "\n")
	lines := strings.Split(text, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[0]) == "" || strings.TrimLeft(lines[0], " \t") != lines[0] ||
		listOpener.MatchString(lines[0]) {
		return nil, errors.New("text whose first line is indented or reads as a list item")
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if line != "" {
			out[i] = indent + line
		}
	}
	return out, nil
}

// literalMarker opens text the tree reads verbatim even when its first line
// is indented or reads as a list item (GH-2644).
const literalMarker = "|"

// literalText prints multi-line text after a literal marker: every line
// indented, read back verbatim.
func literalText(value *yaml.Node, indent string) ([]string, error) {
	lines := strings.Split(strings.TrimRight(value.Value, "\n"), "\n")
	if strings.TrimSpace(lines[0]) == "" {
		return nil, errors.New("text that opens with a blank line")
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if line != "" {
			out[i] = indent + line
		}
	}
	return out, nil
}

// parseTree reads a field's content in the tree form.
func parseTree(text string) (*yaml.Node, error) {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	return parseTreeBlock(lines, 0)
}

// parseTreeBlock reads the lines of one block at an indentation: a bullet
// list is a map, a numbered list a list, and anything else multi-line text.
func parseTreeBlock(lines []string, indent int) (*yaml.Node, error) {
	lines = trimBlankEdges(lines)
	if len(lines) == 0 {
		return nil, errors.New("an empty tree block")
	}
	first, ok := dedent(lines[0], indent)
	switch {
	case !ok:
		return nil, fmt.Errorf("line %q is not indented %d spaces", lines[0], indent)
	case strings.HasPrefix(first, treeBullet):
		return parseTreeEntries(lines, indent, treeBullet)
	case strings.HasPrefix(first, treeNumber) || first == strings.TrimRight(treeNumber, " "):
		return parseTreeEntries(lines, indent, treeNumber)
	}
	return treeTextNode(lines, indent)
}

// parseTreeEntries reads the entries of a map (bullets) or a list (numbers).
func parseTreeEntries(lines []string, indent int, marker string) (*yaml.Node, error) {
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	width := bulletWidth
	if marker == treeNumber {
		node = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		width = numberWidth
	}
	for start := 0; start < len(lines); {
		end := start + 1
		for end < len(lines) && !entryStart(lines[end], indent, marker) {
			end++
		}
		head, _ := dedent(lines[start], indent)
		if !entryStart(lines[start], indent, marker) {
			return nil, fmt.Errorf("line %q does not open an entry", lines[start])
		}
		value, key, err := parseTreeEntry(entryHead(head, marker), lines[start+1:end], indent+width, marker == treeBullet)
		if err != nil {
			return nil, err
		}
		if marker == treeBullet {
			node.Content = append(node.Content, stringNode(key))
		}
		node.Content = append(node.Content, value)
		start = end
	}
	return node, nil
}

// parseTreeEntry reads one entry's head and the lines nested under it.
func parseTreeEntry(head string, nested []string, childIndent int, keyed bool) (*yaml.Node, string, error) {
	key, rest := "", strings.TrimSpace(head)
	if keyed {
		var err error
		if key, rest, err = splitTreeKey(strings.TrimSpace(head)); err != nil {
			return nil, "", err
		}
	}
	if rest == literalMarker {
		value, err := literalTextNode(nested, childIndent)
		return value, key, err
	}
	if !keyed && rest != "" && (listOpener.MatchString(rest) || len(trimBlankEdges(nested)) > 0) {
		block := append([]string{strings.Repeat(" ", childIndent) + rest}, nested...)
		value, err := parseTreeBlock(block, childIndent)
		return value, key, err
	}
	if rest != "" {
		if len(trimBlankEdges(nested)) > 0 {
			return nil, "", fmt.Errorf("entry %q has a value and nested lines", head)
		}
		value, err := decodeFlow(rest)
		return value, key, err
	}
	value, err := parseTreeBlock(nested, childIndent)
	return value, key, err
}

// splitTreeKey splits a map entry's head into its key, bare or JSON-quoted,
// and the rest after the colon.
func splitTreeKey(head string) (string, string, error) {
	if strings.HasPrefix(head, `"`) {
		decoder := json.NewDecoder(strings.NewReader(head))
		var key string
		if err := decoder.Decode(&key); err != nil {
			return "", "", fmt.Errorf("entry %q: %w", head, err)
		}
		rest, found := strings.CutPrefix(head[decoder.InputOffset():], ":")
		if !found {
			return "", "", fmt.Errorf("entry %q has no colon after its key", head)
		}
		return key, strings.TrimSpace(rest), nil
	}
	key, rest, found := strings.Cut(head, ":")
	if !found || !bareName(key) {
		return "", "", fmt.Errorf("entry %q has no key", head)
	}
	return key, strings.TrimSpace(rest), nil
}

// literalTextNode reads text after a literal marker back verbatim.
func literalTextNode(lines []string, indent int) (*yaml.Node, error) {
	lines = trimBlankEdges(lines)
	out := make([]string, len(lines))
	for i, line := range lines {
		if line == "" {
			continue
		}
		text, ok := dedent(line, indent)
		if !ok {
			return nil, misindented(line, indent)
		}
		out[i] = text
	}
	return stringNode(strings.Join(out, "\n")), nil
}

// misnumbered matches a list item numbered other than "1.": the tree form
// numbers every item of a nested list "1.", so a "2." line opens no entry
// and is read as the text of the entry above it.
var misnumbered = regexp.MustCompile(`^([0-9]+)\. `)

// misindented explains a line outside its block's indentation, naming the
// number as the cause when the line is a list item numbered other than 1.
func misindented(line string, indent int) error {
	if m := misnumbered.FindStringSubmatch(strings.TrimLeft(line, " ")); m != nil && m[1] != "1" {
		return fmt.Errorf("list item %q is numbered %s.; number every item of a nested list \"1.\"", strings.TrimSpace(line), m[1])
	}
	return fmt.Errorf("text line %q is not indented %d spaces", line, indent)
}

// entryHead returns an entry line without its marker.
func entryHead(line, marker string) string {
	if strings.HasPrefix(line, marker) {
		return line[len(marker):]
	}
	return "" // the bare marker: the value is nested
}

// entryStart reports a line that opens an entry at this indentation.
func entryStart(line string, indent int, marker string) bool {
	text, ok := dedent(line, indent)
	return ok && (strings.HasPrefix(text, marker) || text == strings.TrimRight(marker, " "))
}

// treeTextNode reads indented multi-line text back verbatim.
func treeTextNode(lines []string, indent int) (*yaml.Node, error) {
	if len(lines) < 2 {
		return nil, fmt.Errorf("one-line block %q is neither an entry nor text", lines[0])
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		if line == "" {
			continue
		}
		text, ok := dedent(line, indent)
		if !ok {
			return nil, misindented(line, indent)
		}
		out[i] = text
	}
	return stringNode(strings.Join(out, "\n")), nil
}

// dedent removes exactly indent leading spaces, or reports that the line
// has fewer.
func dedent(line string, indent int) (string, bool) {
	if len(line) < indent || strings.TrimLeft(line[:indent], " ") != "" {
		return "", false
	}
	return line[indent:], true
}

func trimBlankEdges(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
