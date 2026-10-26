// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package hierarchy

import (
	"fmt"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/links"
)

// Finding is one gate failure in one document.
type Finding struct {
	File    string `json:"file"`
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (f Finding) String() string { return fmt.Sprintf("%s: %s: %s", f.File, f.Field, f.Message) }

var (
	idPattern   = regexp.MustCompile(`^(th|in|ep|ft|st|obj)\d{3}-[a-z0-9-]+$`)
	itemPattern = regexp.MustCompile(`^R\d+\.\d+$`)
)

// Validate checks every document of one repository against its type and the
// links between them (srd045 R6.1, srd007 R8.2): the required fields and
// context items, a parent of the right kind that resolves, link targets
// that resolve, no parent or blocked_by cycle, a story's statement and
// points, and a criterion that names its test and environment. A target in
// another repository is a pinned URL, checked for its pin but not read.
func Validate(docs []*Doc) []Finding {
	byID := map[string]*Doc{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	var out []Finding
	for _, d := range docs {
		out = append(out, validateDoc(d, byID)...)
	}
	return append(out, cycles(docs, byID)...)
}

func validateDoc(d *Doc, byID map[string]*Doc) []Finding {
	v := checker{file: d.Path}
	if d.ContextError != "" {
		// One finding for the cause, not one per item it hides.
		v.add("context", "does not parse (%s)", d.ContextError)
		return v.findings
	}
	v.require("title", d.Title)
	v.require("problem", d.Problem)
	v.require("owner", d.Owner)
	switch {
	case !idPattern.MatchString(d.ID):
		v.add("id", "%q is not a hierarchy id (thNNN, inNNN, epNNN, ftNNN, stNNN, or objNNN, then -short-name)", d.ID)
	case d.ID != strings.TrimSuffix(filepath.Base(d.Path), ".md"):
		v.add("id", "%q does not match the file name", d.ID)
	}
	if !slices.Contains(Statuses, d.Status) {
		v.add("status", "%q is not one of %s", d.Status, strings.Join(Statuses, ", "))
	}
	if d.Kind == Theme || d.Kind == Initiative || d.Kind == Epic {
		v.require("horizon", d.Horizon)
	}
	if len(d.Goals) == 0 {
		v.add("goals", "missing")
	}
	if len(d.NonGoals) == 0 {
		v.add("non_goals", "missing")
	}
	items := map[string]bool{}
	if len(d.Groups) == 0 {
		v.add("requirements", "missing")
	}
	for group, ids := range d.Groups {
		if len(ids) == 0 {
			v.add("requirements."+group, "no items")
		}
		for _, id := range ids {
			if !itemPattern.MatchString(id) || !strings.HasPrefix(id, group+".") {
				v.add("requirements."+group, "%q is not numbered %s.n", id, group)
			}
			items[id] = true
		}
		if d.Kind == Initiative {
			if _, err := time.Parse("2006-01-02", d.Due[group]); err != nil {
				v.add("requirements."+group+".due", "missing or not a YYYY-MM-DD date")
			}
		}
	}
	if len(d.Criteria) == 0 {
		v.add("acceptance_criteria", "missing")
	}
	for _, c := range d.Criteria {
		field := "acceptance_criteria." + c.ID
		if len(c.Tests) == 0 {
			v.add(field+".tests", "missing")
		}
		if c.Environment == "" {
			v.add(field+".applicability", "names no environment (D8)")
		}
		if len(c.Traces) == 0 {
			v.add(field+".traces", "missing")
		}
		for _, t := range c.Traces {
			if !items[t] {
				v.add(field+".traces", "%q names no item of this document", t)
			}
		}
	}
	v.parent(d, byID)
	v.architecture(d)
	for _, k := range LinkKeys {
		for _, target := range d.Links[k] {
			v.target("context."+k, target, byID)
		}
	}
	if d.Kind == Story {
		v.require("as_a", d.AsA)
		v.require("i_want", d.IWant)
		v.require("so_that", d.SoThat)
		if d.PointsValue() <= 0 {
			v.add("points", "missing or not a positive number")
		}
		if d.Rank != "" && d.RankValue() == int(^uint(0)>>1) {
			v.add("rank", "%q is not a number", d.Rank)
		}
		for _, c := range d.Cites {
			if err := links.CheckPinned(c); err != nil {
				v.add("cites", "%v", err)
			}
		}
	}
	return v.findings
}

// architecture checks the components a document cites (GH-55): one in
// this repository must be a component its ARCHITECTURE declares, one in
// another repository a URL pinned to a release tag. The item is optional.
func (v *checker) architecture(d *Doc) {
	for _, a := range d.Architecture {
		switch {
		case strings.Contains(a, "://"):
			if err := links.CheckPinned(a); err != nil {
				v.add("architecture", "%v", err)
			}
		case d.components == nil:
			v.add("architecture", "%q names a component, but %s is missing or does not parse, so it declares none", a, ArchitectureFile)
		case len(d.components) == 0:
			v.add("architecture", "%q is no component %s declares: it declares none yet", a, ArchitectureFile)
		case !d.components[a]:
			v.add("architecture", "%q is no component %s declares (%s)", a, ArchitectureFile, strings.Join(slices.Sorted(maps.Keys(d.components)), ", "))
		}
	}
}

// parent checks a document's parent: a theme has none, a feature must have
// one, and a parent in this repository must be of the kind srd007 R7.2
// allows.
func (v *checker) parent(d *Doc, byID map[string]*Doc) {
	switch {
	case d.Parent == "" && d.Kind == Feature:
		v.add("parent", "a feature must name its epic")
		return
	case d.Parent == "":
		return
	case d.Kind == Theme:
		v.add("parent", "a theme has no parent")
		return
	}
	if strings.Contains(d.Parent, "://") {
		if err := links.CheckPinnedDocument(d.Parent); err != nil {
			v.add("parent", "%v", err)
		}
		return
	}
	p, ok := byID[d.Parent]
	if !ok {
		v.add("parent", "%q names no document in this repository", d.Parent)
		return
	}
	if !slices.Contains(parentKinds[d.Kind], p.Kind) {
		v.add("parent", "a %s's parent is %s, not the %s %q", d.Kind, kindsText(parentKinds[d.Kind]), p.Kind, d.Parent)
	}
}

// target checks one link: a pinned URL to another repository, or the id of a
// document in this one.
func (v *checker) target(field, target string, byID map[string]*Doc) {
	if strings.Contains(target, "://") {
		if err := links.CheckPinned(target); err != nil {
			v.add(field, "%v", err)
		}
		return
	}
	if _, ok := byID[target]; !ok {
		v.add(field, "%q names no document in this repository", target)
	}
}

// cycles reports every cycle the parent and blocked_by links form among the
// documents of this repository, once each, listing its documents.
func cycles(docs []*Doc, byID map[string]*Doc) []Finding {
	edges := map[string][]string{}
	for _, d := range docs {
		for _, t := range append([]string{d.Parent}, d.Links["blocked_by"]...) {
			if _, ok := byID[t]; ok {
				edges[d.ID] = append(edges[d.ID], t)
			}
		}
	}
	const (
		unseen = iota
		open
		done
	)
	state := map[string]int{}
	seen := map[string]bool{}
	var out []Finding
	var stack []string
	var walk func(id string)
	walk = func(id string) {
		state[id] = open
		stack = append(stack, id)
		for _, next := range edges[id] {
			switch state[next] {
			case unseen:
				walk(next)
			case open:
				i := slices.Index(stack, next)
				members := slices.Clone(stack[i:])
				key := slices.Clone(members)
				sort.Strings(key)
				if !seen[strings.Join(key, " ")] {
					seen[strings.Join(key, " ")] = true
					out = append(out, Finding{File: byID[next].Path, Field: "context", Message: "parent and blocked_by links form a cycle: " + strings.Join(append(members, next), " -> ")})
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = done
	}
	for _, d := range docs {
		if state[d.ID] == unseen {
			walk(d.ID)
		}
	}
	return out
}

func kindsText(kinds []Kind) string {
	s := make([]string, len(kinds))
	for i, k := range kinds {
		s[i] = "a " + string(k)
	}
	return strings.Join(s, " or ")
}

type checker struct {
	file     string
	findings []Finding
}

func (v *checker) add(field, format string, args ...any) {
	v.findings = append(v.findings, Finding{File: v.file, Field: field, Message: fmt.Sprintf(format, args...)})
}

func (v *checker) require(field, value string) {
	if strings.TrimSpace(value) == "" {
		v.add(field, "missing")
	}
}
