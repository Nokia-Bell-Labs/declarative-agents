// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package objectives loads and validates initiative documents (srd034 R1),
// and objective documents, their name before release 04.0, as initiatives.
// An initiative is an SRD in the pinned parent release's Markdown format:
// owner, horizon, and parent are items of its context, a group's due and
// delivered_by are items of the group's applicability, and a criterion's
// environment is its applicability (design.md schema.initiative).
package objectives

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
)

// Dir is where a system repository keeps its initiatives; LegacyDir is where
// it kept objective documents, which are read as initiatives (srd034 R1.1).
const (
	Dir       = "docs/specs/initiatives"
	LegacyDir = "docs/specs/objectives"
)

// DirFor is the directory a document id lives in: objNNN ids in LegacyDir,
// every other id in Dir.
func DirFor(id string) string {
	if strings.HasPrefix(id, "obj") {
		return LegacyDir
	}
	return Dir
}

// Document is one objective document.
type Document struct {
	Path               string
	ID                 string
	Title              string
	Problem            string
	Goals              []map[string]string
	Requirements       map[string]Objective
	NonGoals           []string
	AcceptanceCriteria []Criterion
	Owner              string
	Horizon            string
	Parent             string
	Status             string
}

// Objective is one requirement group: one objective of the program.
type Objective struct {
	Title       string
	Due         string
	DeliveredBy []string
	Items       []map[string]string
}

// Criterion is one acceptance criterion; it names its evidence (D8).
type Criterion struct {
	ID        string
	Criterion string
	Traces    []string
	// Tests are the criterion's test names. A test entry written
	// "name (R1.1, R1.2)" names the items it proves, kept in TestItems; a
	// test without items proves every item the criterion traces (GH-20).
	Tests       []string
	TestItems   map[string][]string
	Environment string
}

// TestsFor are the criterion's tests that prove any of items: an
// attributed test only when it names one of them, an unattributed one
// always.
func (c Criterion) TestsFor(items []string) []string {
	var out []string
	for _, t := range c.Tests {
		names, ok := c.TestItems[t]
		if !ok {
			out = append(out, t)
			continue
		}
		for _, n := range names {
			if slices.Contains(items, n) {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// testEntry splits a test entry "name (R1.1, R1.2)" into its name and the
// items it names; an entry without them names none.
var testEntry = regexp.MustCompile(`^\s*(\S+)\s*\(\s*(R\d+(?:\.\d+)?(?:\s*,\s*R\d+(?:\.\d+)?)*)\s*\)\s*$`)

func parseTests(entries []string) ([]string, map[string][]string) {
	var names []string
	items := map[string][]string{}
	for _, e := range entries {
		m := testEntry.FindStringSubmatch(e)
		if m == nil {
			names = append(names, strings.TrimSpace(e))
			continue
		}
		names = append(names, m[1])
		for _, it := range strings.Split(m[2], ",") {
			items[m[1]] = append(items[m[1]], strings.TrimSpace(it))
		}
	}
	return names, items
}

// Item is one numbered part of an objective.
type Item struct {
	ID   string
	Text string
}

// Statuses are the values design.md schema.objective allows.
var Statuses = []string{"to_be_agreed", "agreed", "retired"}

// Load reads one objective document.
func Load(path string) (*Document, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(path, data)
}

// Parse reads an objective document, an SRD in the parent's Markdown format.
func Parse(path string, data []byte) (*Document, error) {
	f, err := corpus.Decode(path, data)
	if err != nil {
		return nil, err
	}
	return FromFile(f)
}

// FromFile reads an objective document fetched through corpus.Get.
func FromFile(f corpus.File) (*Document, error) {
	d := &Document{}
	fromMarkdown(d, f.Markdown)
	d.Path = f.Path
	return d, nil
}

// fromMarkdown fills the document from the declared fields that hold it.
func fromMarkdown(d *Document, md *corpus.Document) {
	d.ID = md.ID()
	d.Title, _ = md.Front["title"].(string)
	d.Status, _ = md.Front["status"].(string)
	context := md.Field("context").Tree()
	d.Owner, d.Horizon = corpus.TreeString(context, "owner"), corpus.TreeString(context, "horizon")
	d.Parent = corpus.TreeString(context, "parent")
	d.Problem = md.Field("problem").Text()
	d.NonGoals = md.Field("non_goals").List()
	for _, g := range md.Field("goals").List() {
		id, text, _ := strings.Cut(g, ": ")
		d.Goals = append(d.Goals, map[string]string{id: text})
	}
	for _, group := range md.Sections {
		if group.Kind != "req" {
			continue
		}
		if d.Requirements == nil {
			d.Requirements = map[string]Objective{}
		}
		_, title, _ := strings.Cut(group.Heading, " - ")
		applicability := group.Field("applicability").Tree()
		o := Objective{
			Title:       title,
			Due:         corpus.TreeString(applicability, "due"),
			DeliveredBy: corpus.TreeList(applicability, "delivered_by"),
		}
		for _, item := range group.Children {
			if item.Kind == "req" {
				o.Items = append(o.Items, map[string]string{item.Key: item.Text()})
			}
		}
		d.Requirements[group.Key] = o
	}
	for _, ac := range md.Field("acceptance_criteria").Children {
		if ac.Kind != "ac" {
			continue
		}
		tests, testItems := parseTests(ac.Field("tests").List())
		d.AcceptanceCriteria = append(d.AcceptanceCriteria, Criterion{
			ID:          ac.Key,
			Criterion:   ac.Text(),
			Traces:      ac.Field("traces").List(),
			Tests:       tests,
			TestItems:   testItems,
			Environment: ac.Field("applicability").Text(),
		})
	}
}

// LoadDir reads every inNNN-*.md and objNNN-*.md document under dir, in
// name order. An absent dir holds no documents.
func LoadDir(dir string) ([]*Document, error) {
	var paths []string
	for _, pattern := range []string{"in[0-9]*.md", "obj[0-9]*.md"} {
		found, err := filepath.Glob(filepath.Join(dir, pattern))
		if err != nil {
			return nil, err
		}
		paths = append(paths, found...)
	}
	sort.Strings(paths)
	docs := make([]*Document, 0, len(paths))
	for _, p := range paths {
		d, err := Load(p)
		if err != nil {
			return nil, err
		}
		docs = append(docs, d)
	}
	return docs, nil
}

// LoadAll reads the initiatives of the repository at root: those under Dir
// and the objective documents under LegacyDir.
func LoadAll(root string) ([]*Document, error) {
	var docs []*Document
	for _, dir := range []string{Dir, LegacyDir} {
		found, err := LoadDir(filepath.Join(root, dir))
		if err != nil {
			return nil, err
		}
		docs = append(docs, found...)
	}
	return docs, nil
}

// ObjectiveIDs returns the requirement group ids in numeric order (R1, R2, R10).
func (d *Document) ObjectiveIDs() []string {
	ids := make([]string, 0, len(d.Requirements))
	for id := range d.Requirements {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return idNumber(ids[i]) < idNumber(ids[j]) })
	return ids
}

// ItemsOf returns an objective's items in document order.
func (o Objective) ItemsOf() []Item {
	items := make([]Item, 0, len(o.Items))
	for _, m := range o.Items {
		for id, text := range m {
			items = append(items, Item{ID: id, Text: strings.TrimSpace(text)})
		}
	}
	return items
}

// CriteriaFor returns the acceptance criteria that trace any item of the
// objective with the given group id.
func (d *Document) CriteriaFor(group string) []Criterion {
	var out []Criterion
	for _, c := range d.AcceptanceCriteria {
		for _, t := range c.Traces {
			if strings.HasPrefix(t, group+".") {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

func idNumber(id string) int {
	n, _ := strconv.Atoi(strings.TrimPrefix(id, "R"))
	return n
}
