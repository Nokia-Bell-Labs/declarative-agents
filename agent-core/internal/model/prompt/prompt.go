// Copyright (c) 2026 Nokia. All rights reserved.

// Package prompt defines the Prompt struct and loading functions for
// agents built on agent-core. A Prompt carries the four sections of a
// system-role message (Role, Task, Constraints, OutputFormat) and can
// be loaded from a YAML file or an inline string.
//
// Agents override the compiled defaults with domain-specific values.
// The generator uses a Go coding persona; the planner uses a planning
// persona. Both share the same struct and assembly logic.
package prompt

import "strings"

// Prompt carries the four sections of a system-role message.
type Prompt struct {
	Role         string
	Task         string
	Constraints  string
	OutputFormat string
}

// Assemble concatenates Role, Task, Constraints, and OutputFormat in
// order, separated by blank lines. Sections that are empty after
// trimming are omitted. The result is deterministic for a given Prompt.
func (p Prompt) Assemble() string {
	sections := [4]string{p.Role, p.Task, p.Constraints, p.OutputFormat}
	var parts []string
	for _, s := range sections {
		if strings.TrimSpace(s) != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "\n\n")
}

// SectionCount returns how many non-empty sections the prompt has.
func (p Prompt) SectionCount() int {
	count := 0
	for _, s := range [4]string{p.Role, p.Task, p.Constraints, p.OutputFormat} {
		if strings.TrimSpace(s) != "" {
			count++
		}
	}
	return count
}
