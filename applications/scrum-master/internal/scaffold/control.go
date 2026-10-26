// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package scaffold

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/beads"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/program"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// labelAxes are the axes of the labels config format in its order, with the
// attributes its example gives each. state carries no prefix: its values are
// whole label names.
var labelAxes = []struct{ name, prefix, exclusive, required string }{
	{"plane", "plane::", "true", "all"},
	{"type", "type::", "true", "none"},
	{"kind", "kind::", "true", "data"},
	{"op", "op::", "true", "none"},
	{"state", "", "false", "none"},
}

// LabelsDocument renders labels as control/labels.md in the labels config
// format, so scrum-master issue labels reads back exactly labels.
func LabelsDocument(labels []tracker.Label) string {
	var b strings.Builder
	b.WriteString("---\nid: labels\ntitle: Label schema\n---\n\n")
	b.WriteString("<!-- field: version -->\n## Version\n\n1\n\n<!-- field: axes -->\n## Axes\n")
	for _, a := range labelAxes {
		b.WriteString(axisSection(a.name, labels))
		if a.name == "kind" {
			b.WriteString("\n<!-- field: component -->\n### Component\n\n<!-- field: prefix -->\n#### Prefix\n\ncomponent::\n\n" +
				"<!-- field: exclusive -->\n#### Exclusive\n\ntrue\n\n<!-- field: required -->\n#### Required\n\nall\n\n" +
				"<!-- field: values_from -->\n#### Values from\n\nsubsystem SRDs (srd001 R21.1)\n\n<!-- field: color -->\n#### Color\n\n#7f8c8d\n")
		}
	}
	return b.String()
}

// axisOf is the axis of labelAxes a label belongs to by its prefix; a
// label with none is a state.
func axisOf(name string) string {
	for _, a := range labelAxes {
		if a.prefix != "" && strings.HasPrefix(name, a.prefix) {
			return a.name
		}
	}
	return "state"
}

// axisSection renders one axis of labelAxes as control/labels.md holds it:
// its prefix, exclusive, and required attributes and a values table of the
// labels on it.
func axisSection(name string, labels []tracker.Label) string {
	var b strings.Builder
	for _, a := range labelAxes {
		if a.name != name {
			continue
		}
		fmt.Fprintf(&b, "\n<!-- field: %s -->\n### %s\n\n", a.name, strings.ToUpper(a.name[:1])+a.name[1:])
		prefix := a.prefix
		if prefix == "" {
			prefix = "None"
		}
		fmt.Fprintf(&b, "<!-- field: prefix -->\n#### Prefix\n\n%s\n\n", prefix)
		fmt.Fprintf(&b, "<!-- field: exclusive -->\n#### Exclusive\n\n%s\n\n", a.exclusive)
		fmt.Fprintf(&b, "<!-- field: required -->\n#### Required\n\n%s\n\n", a.required)
		b.WriteString("<!-- field: values -->\n#### Values\n\n| name | color | description |\n| --- | --- | --- |\n")
		for _, l := range labels {
			if axisOf(l.Name) == a.name {
				fmt.Fprintf(&b, "| %s | %s | %s |\n", strings.TrimPrefix(l.Name, a.prefix), l.Color, l.Description)
			}
		}
	}
	return b.String()
}

// labelsFile is what ExtendLabels reads of control/labels.md: each axis's
// prefix and value names.
type labelsFile struct {
	Axes map[string]struct {
		Prefix string `yaml:"prefix"`
		Values []struct {
			Name string `yaml:"name"`
		} `yaml:"values"`
	} `yaml:"axes"`
}

// axisMarker finds an axis section of control/labels.md: its field marker
// over a level 3 heading.
var axisMarker = regexp.MustCompile(`(?m)^<!-- field: ([a-z_]+)[^\n]*-->\n### `)

// ExtendLabels adds to the control/labels.md text doc every axis and value
// of defaults it lacks, and returns the text and the labels it added (srd043
// R4.2). A missing axis gets its whole section, before the next axis of
// labelAxes the file has; a missing value gets one row at the end of its
// axis's values table, with the default's color and description. Nothing
// that exists is changed, reordered, or removed: a person's recolor and a
// program's own values stay. Values are matched under the file's own prefix
// for the axis.
func ExtendLabels(doc []byte, defaults []tracker.Label) ([]byte, []string, error) {
	var f labelsFile
	if err := corpus.DecodeConfig(doc, &f); err != nil {
		return doc, nil, err
	}
	text := string(doc)
	var added []string
	for _, a := range labelAxes {
		var want []tracker.Label
		for _, l := range defaults {
			if axisOf(l.Name) == a.name {
				want = append(want, l)
			}
		}
		if len(want) == 0 {
			continue
		}
		have, ok := f.Axes[a.name]
		if !ok {
			text = insertAxis(text, a.name, axisSection(a.name, want))
			for _, l := range want {
				added = append(added, l.Name)
			}
			continue
		}
		prefix := have.Prefix
		if prefix == "None" {
			prefix = ""
		}
		names := map[string]bool{}
		for _, v := range have.Values {
			names[prefix+v.Name] = true
		}
		for _, l := range want {
			value := strings.TrimPrefix(l.Name, a.prefix)
			if names[prefix+value] {
				continue
			}
			var err error
			if text, err = appendRow(text, a.name, []string{value, l.Color, l.Description}); err != nil {
				return doc, nil, err
			}
			added = append(added, prefix+value)
		}
	}
	return []byte(text), added, nil
}

// insertAxis puts section before the first axis after name in labelAxes
// that text has, or at its end.
func insertAxis(text, name, section string) string {
	after := false
	for _, a := range labelAxes {
		if a.name == name {
			after = true
			continue
		}
		if !after {
			continue
		}
		for _, m := range axisMarker.FindAllStringSubmatchIndex(text, -1) {
			if text[m[2]:m[3]] == a.name {
				return strings.TrimRight(text[:m[0]], "\n") + "\n" + section + "\n" + text[m[0]:]
			}
		}
	}
	return strings.TrimRight(text, "\n") + "\n" + section
}

// appendRow adds a row after the last row of axis's values table, padded
// to the table's columns.
func appendRow(text, axis string, cells []string) (string, error) {
	marks := axisMarker.FindAllStringSubmatchIndex(text, -1)
	for i, m := range marks {
		if text[m[2]:m[3]] != axis {
			continue
		}
		end := len(text)
		if i+1 < len(marks) {
			end = marks[i+1][0]
		}
		section := text[m[0]:end]
		lines := strings.SplitAfter(section, "\n")
		last, columns := -1, 0
		for j, line := range lines {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "|") {
				if last < 0 || j == last+1 {
					if last < 0 {
						columns = strings.Count(t, "|") - 1
					}
					last = j
				}
			}
		}
		if last < 0 {
			return text, fmt.Errorf("the %s axis has no values table to add to", axis)
		}
		for len(cells) < columns {
			cells = append(cells, "")
		}
		row := "| " + strings.Join(cells, " | ") + " |\n"
		if !strings.HasSuffix(lines[last], "\n") {
			row = "\n" + row
		}
		lines = append(lines[:last+1], append([]string{row}, lines[last+1:]...)...)
		return text[:m[0]] + strings.Join(lines, "") + text[end:], nil
	}
	return text, fmt.Errorf("control/labels.md has no %s axis section", axis)
}

// PolicyDocument renders p as control/policy.md in the policy config format:
// the settings scrum-master's policy reader takes from the file, the named admission
// rules among them, so removing a rule from the file disables it (srd013
// R1.1).
func PolicyDocument(p program.Policy) string {
	var b strings.Builder
	b.WriteString("---\nid: policy\ntitle: Policy\n---\n\n<!-- field: version -->\n## Version\n\n1\n\n")
	b.WriteString("<!-- field: admission -->\n## Admission\n\n")
	fmt.Fprintf(&b, "<!-- field: require_milestone -->\n### Require milestone\n\n%t\n\n", p.RequireMilestone)
	fmt.Fprintf(&b, "<!-- field: require_acceptance_criteria -->\n### Require acceptance criteria\n\n%t\n\n", len(p.RequireAC) > 0)
	fmt.Fprintf(&b, "<!-- field: require_approval -->\n### Require approval\n\n%t\n\n", p.RequireApproval)
	fmt.Fprintf(&b, "<!-- field: audit_command -->\n### Audit command\n\n%s\n\n", p.AuditCommand)
	b.WriteString("<!-- field: rules -->\n### Rules\n\n")
	admissionRules := p.AdmissionRules
	if admissionRules == nil {
		admissionRules = program.RuleNames()
	}
	for _, r := range admissionRules {
		b.WriteString("- " + r + "\n")
	}
	b.WriteString("\n")
	b.WriteString("<!-- field: escalation -->\n## Escalation\n\n")
	for _, e := range []struct {
		field string
		value int
	}{
		{"blocker_age_days", p.Escalation.BlockerAgeDays},
		{"waiting_chain_length", p.Escalation.WaitingChainLength},
		{"slip_releases", p.Escalation.SlipReleases},
		{"evidence_age_days", p.Escalation.EvidenceAgeDays},
	} {
		fmt.Fprintf(&b, "<!-- field: %s -->\n### %s\n\n%d\n\n", e.field, heading(e.field), e.value)
	}
	b.WriteString("<!-- field: releases -->\n## Releases\n\n")
	fmt.Fprintf(&b, "<!-- field: scheme -->\n### Scheme\n\n%s\n\n", p.Releases.Scheme)
	fmt.Fprintf(&b, "<!-- field: breaking -->\n### Breaking\n\n%s\n\n", p.Releases.Breaking)
	fmt.Fprintf(&b, "<!-- field: tag_format -->\n### Tag format\n\n%s\n\n", p.Releases.TagFormat)
	fmt.Fprintf(&b, "<!-- field: architecture_checks -->\n## Architecture checks\n\n%s\n\n", p.ArchitectureChecks)
	b.WriteString("<!-- field: definition_of_done -->\n## Definition of done\n")
	artifacts := make([]string, 0, len(p.DefinitionOfDone))
	for a := range p.DefinitionOfDone {
		artifacts = append(artifacts, a)
	}
	sort.Strings(artifacts)
	for _, a := range artifacts {
		fmt.Fprintf(&b, "\n<!-- field: %s -->\n### %s\n\n| kind | environment | suite | command |\n| --- | --- | --- | --- |\n", a, heading(a))
		for _, c := range p.DefinitionOfDone[a] {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", c.Kind, c.Environment, c.Suite, c.Command)
		}
	}
	return b.String()
}

func heading(field string) string {
	s := strings.ReplaceAll(field, "_", " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// StatesDocument renders rows as control/states.md in the states config
// format, so the beads tracker reads back exactly rows (srd041 R8): one
// table row per transition, from as a comma-separated list with any-open
// for every state but closed.
func StatesDocument(rows []beads.Row) string {
	var b strings.Builder
	b.WriteString("---\nid: states\ntitle: Issue states\n---\n\n")
	b.WriteString("<!-- field: version -->\n## Version\n\n1\n\n")
	b.WriteString("<!-- field: rows -->\n## Rows\n\n")
	b.WriteString("| from | to | actor | assignee | requires | kind |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, r := range rows {
		from := strings.Join(r.From, ", ")
		if slices.Equal(r.From, beads.AnyOpen) {
			from = "any-open"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s |\n", from, r.To, r.Actor, r.Assignee, r.Requires, r.Kind)
	}
	return b.String()
}
