// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package fragments

import (
	"fmt"
	"regexp"

	"gopkg.in/yaml.v3"
)

// paramPattern is the reference form. It cannot collide with environment
// expansion, whose pattern is ${NAME}, nor with selectors, which begin $. or
// $from( (srd052 R2.3).
var paramPattern = regexp.MustCompile(`\$param\(([A-Za-z_][A-Za-z0-9_]*)\)`)

// References reports whether a document mentions a parameter at all, which
// is how a file with a body and no params is told apart from one that never
// meant to be a fragment (srd052 R1.3).
func References(data []byte) bool { return paramPattern.Match(data) }

// Substitute replaces $param(name) inside scalar values only (srd052 R2.3).
// Mapping names and structure are never touched, so an argument fills a hole
// and cannot add a mapping, a list entry, or a second tool. A reference to an
// undeclared parameter is an error naming the line; a reference in a mapping
// name is left alone for Leftover to report.
func Substitute(doc *yaml.Node, args map[string]Arg) error {
	if doc == nil {
		return nil
	}
	switch doc.Kind {
	case yaml.DocumentNode, yaml.SequenceNode:
		for _, child := range doc.Content {
			if err := Substitute(child, args); err != nil {
				return err
			}
		}
	case yaml.MappingNode:
		for index := 1; index < len(doc.Content); index += 2 {
			if err := Substitute(doc.Content[index], args); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		return substituteScalar(doc, args)
	}
	return nil
}

func substituteScalar(node *yaml.Node, args map[string]Arg) error {
	match := paramPattern.FindStringSubmatch(node.Value)
	if match == nil {
		return nil
	}
	// A hole that is the whole scalar takes the argument's type, so an
	// integer parameter fills an integer field; a hole inside a longer string
	// stays text.
	if match[0] == node.Value {
		arg, ok := args[match[1]]
		if !ok {
			return undeclared(match[1], node.Line)
		}
		node.Value, node.Tag = arg.Value, arg.Tag
		node.Style = 0
		if arg.Tag == "!!str" {
			node.Style = yaml.DoubleQuotedStyle
		}
		return nil
	}
	var missing string
	node.Value = paramPattern.ReplaceAllStringFunc(node.Value, func(reference string) string {
		name := paramPattern.FindStringSubmatch(reference)[1]
		arg, ok := args[name]
		if !ok {
			missing = name
			return reference
		}
		return arg.Value
	})
	if missing != "" {
		return undeclared(missing, node.Line)
	}
	node.Tag = "!!str"
	return nil
}

func undeclared(name string, line int) error {
	return fmt.Errorf("line %d: $param(%s) names no declared parameter", line, name)
}

// Leftover returns the first $param( reference surviving substitution, in
// any position including mapping names, with its line (srd052 R2.3).
func Leftover(doc *yaml.Node) (int, bool) {
	if doc == nil {
		return 0, false
	}
	if doc.Kind == yaml.ScalarNode {
		if paramPattern.MatchString(doc.Value) {
			return doc.Line, true
		}
		return 0, false
	}
	for _, child := range doc.Content {
		if line, found := Leftover(child); found {
			return line, true
		}
	}
	return 0, false
}

// RemoveField drops one top-level mapping entry from a document, which is how
// an expanded fragment sheds its params before it is read as a unit.
func RemoveField(doc *yaml.Node, name string) {
	root := doc
	if root != nil && root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		root = root.Content[0]
	}
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	for index := 0; index+1 < len(root.Content); index += 2 {
		if root.Content[index].Value == name {
			root.Content = append(root.Content[:index], root.Content[index+2:]...)
			return
		}
	}
}
