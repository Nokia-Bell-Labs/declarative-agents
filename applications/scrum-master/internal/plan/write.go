// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/backlog"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/tracker"
)

// Release is the release plan writes into the road map: its
// objective items in deliverables and its milestone and tag in
// release.publication (design.md schema.roadmap, srd031 R4.1).
type Release struct {
	Version   string   `json:"version"`
	Items     []string `json:"items"`
	Milestone string   `json:"milestone"`
	Tag       string   `json:"tag"`
}

// Returned is an item plan would not place, and why (srd031 R4.4).
type Returned struct {
	ID      string   `json:"id"`
	Missing []string `json:"missing"`
}

// Written is what scrum-master plan write reports.
type Written struct {
	Path     string     `json:"path"`
	Release  Release    `json:"release"`
	Action   string     `json:"action"` // added or replaced
	Returned []Returned `json:"returned,omitempty"`
}

var releaseMarker = regexp.MustCompile(`(?m)^<!-- release: ([^ |]+)( \|[^>]*)? -->$`)

// WriteRelease puts r into the road map in dir, replacing the release of
// the same version or adding it after the last one. An item the
// candidates hold back for a missing test or environment is not placed:
// it comes back in Returned, and on issue, when given, a comment names it
// and what it lacks so its owner can fix the objective document.
func WriteRelease(dir string, r Release, c Candidates, t tracker.Tracker, issueProject string, issueIID int) (Written, error) {
	w := Written{Path: RoadMap}
	if err := DesignGaps(dir, r.Items); err != nil {
		return w, err
	}
	held := map[string][]string{}
	for _, it := range c.WithoutEvidence {
		held[it.ID] = it.Missing
	}
	var place []string
	for _, id := range r.Items {
		switch {
		case held[id] != nil:
			w.Returned = append(w.Returned, Returned{id, held[id]})
		case !c.agreed[id]:
			return w, fmt.Errorf("%s is not a candidate objective item of an agreed objective document", id)
		default:
			place = append(place, id)
		}
	}
	if len(place) == 0 {
		return w, fmt.Errorf("release %s has no placeable objective item", r.Version)
	}
	if r.Version == "" || r.Milestone == "" || r.Tag == "" {
		return w, fmt.Errorf("a release needs a version, a milestone, and a tag")
	}
	r.Items = place
	w.Release = r
	path := filepath.Join(dir, RoadMap)
	b, err := os.ReadFile(path)
	if err != nil {
		return w, err
	}
	out, action := putRelease(string(b), r)
	w.Action = action
	if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
		return w, err
	}
	if len(w.Returned) > 0 && t != nil && issueProject != "" {
		var body strings.Builder
		body.WriteString("plan did not place these objective items; their acceptance criteria need a test and an environment (srd031 R4.4):\n\n")
		for _, x := range w.Returned {
			fmt.Fprintf(&body, "- %s: missing %s\n", x.ID, strings.Join(x.Missing, " and "))
		}
		if err := t.CreateNote(issueProject, issueIID, body.String()); err != nil {
			return w, err
		}
	}
	return w, nil
}

// releaseFieldOrder is the order the parent schema gives a release
// record's fields (agent-core design.yaml, release_field), which scrum-master doctor
// checks: deliverables come before srds, and both before release.
var releaseFieldOrder = []string{"capabilities", "numbering_note", "proof_commands", "title", "library_srds", "parent",
	"related_agent_core_release", "application_srds", "planned_work", "relocated_to", "deliverables", "dependencies", "srds",
	"semantic_models", "shared_requirements", "migration_note", "applicability", "external_requirements", "historical_aliases",
	"specification", "config_formats", "migration", "use_cases", "implementation_sequence", "current_applications", "release",
	"test_suites", "current_application_evidence", "follow_up", "evidence_status", "proof", "evidence", "implementation_evidence",
	"evidence_boundary", "success_markers", "deferred", "post_merge", "non_runnable_modules"}

// placeholder is a sentence scrum-master init's templates leave for an owner to
// replace ("... writes here ...").
var placeholder = regexp.MustCompile(`(?m)^[^\n]* writes here [^\n]*\.\n`)

// putRelease replaces the release record of r's version, keeping its
// heading line, or appends a new one after the last release record. The
// record's fields follow the parent schema's order, and a placeholder
// sentence, in the record or in the road map's overview, becomes text that
// says what the record holds (srd031 R4.1).
func putRelease(doc string, r Release) (string, string) {
	var deliverables strings.Builder
	deliverables.WriteString("<!-- field: deliverables -->\n### Deliverables\n\n")
	for _, id := range r.Items {
		deliverables.WriteString("- " + id + "\n")
	}
	owned := []recordPart{
		{"deliverables", strings.TrimRight(deliverables.String(), "\n")},
		{"release", fmt.Sprintf("<!-- field: release -->\n### Release\n\n<!-- field: publication | form: \"tree\" -->\n#### Publication\n\n- tag: %s\n- milestone: %s", treeValue(r.Tag), treeValue(r.Milestone))},
	}
	proves := fmt.Sprintf("Release %s delivers %s, on milestone %s, tagged %s.", r.Version, strings.Join(r.Items, ", "), r.Milestone, r.Tag)
	doc = fillOverview(doc)
	marks := releaseMarker.FindAllStringSubmatchIndex(doc, -1)
	for i, m := range marks {
		if doc[m[2]:m[3]] != r.Version {
			continue
		}
		head := doc[m[0]:]
		headEnd := m[0] + strings.Index(head, "\n") + 1
		headEnd += strings.Index(doc[headEnd:], "\n") + 1 // the heading line
		end := len(doc)
		if i+1 < len(marks) {
			end = marks[i+1][0]
		} else if next := nextLevel2(doc, headEnd); next >= 0 {
			end = next
		}
		tail := doc[end:]
		if tail != "" {
			tail = "\n" + strings.TrimLeft(tail, "\n")
		}
		record := doc[headEnd:end]
		prose, fields := recordParts(record)
		prose = placeholder.ReplaceAllString(prose+"\n", proves+"\n")
		// A republish moves the tag: every mention of the old one in the
		// release's prose moves with it, a person's sentences included, so
		// no paragraph names a tag the record no longer holds (GH-20).
		if m := publicationTag.FindStringSubmatch(record); m != nil {
			if old := strings.Trim(strings.TrimSpace(m[1]), `"'`); old != "" && old != r.Tag {
				prose = strings.ReplaceAll(prose, old, r.Tag)
			}
		}
		// The sentence scrum-master wrote for an earlier write follows the record:
		// a republish moves the tag, so the prose moves with it (#188).
		own := regexp.MustCompile(`(?m)^Release ` + regexp.QuoteMeta(r.Version) + ` delivers [^\n]*, tagged [^\n]*\.\n`)
		prose = own.ReplaceAllLiteralString(prose, proves+"\n")
		return doc[:headEnd] + assemble(prose, append(fields, owned...)) + tail, "replaced"
	}
	fields := owned
	if srds := lastSRDs(doc); srds != "" {
		fields = append(fields, recordPart{"srds", strings.Trim(srds, "\n")})
	}
	record := fmt.Sprintf("<!-- release: %s | status: specified -->\n## %s - Release %s\n", r.Version, r.Version, r.Version) + assemble(proves, fields)
	if len(marks) == 0 {
		return strings.TrimRight(doc, "\n") + "\n\n" + record, "added"
	}
	last := marks[len(marks)-1]
	end := len(doc)
	if next := nextLevel2(doc, last[1]); next >= 0 {
		end = next
	}
	before := strings.TrimRight(doc[:end], "\n") + "\n\n" + record
	if rest := strings.TrimLeft(doc[end:], "\n"); rest != "" {
		return before + "\n" + rest, "added"
	}
	return before, "added"
}

// publicationTag is the tag line of a release record's publication.
var publicationTag = regexp.MustCompile(`(?m)^- tag: (.+)$`)

var recordField = regexp.MustCompile(`(?m)^<!-- field: ([a-z_]+)[^\n]*-->\n### `)

// recordPart is one field of a release record, its marker through its
// content, trimmed.
type recordPart struct{ name, text string }

// recordParts splits a release record's body into its prose and every
// field but deliverables and release, which plan owns (srd031 R4.1).
func recordParts(record string) (string, []recordPart) {
	marks := recordField.FindAllStringSubmatchIndex(record, -1)
	end := len(record)
	if len(marks) > 0 {
		end = marks[0][0]
	}
	prose := strings.Trim(record[:end], "\n")
	var fields []recordPart
	for i, m := range marks {
		stop := len(record)
		if i+1 < len(marks) {
			stop = marks[i+1][0]
		}
		if name := record[m[2]:m[3]]; name != "deliverables" && name != "release" {
			fields = append(fields, recordPart{name, strings.Trim(record[m[0]:stop], "\n")})
		}
	}
	return prose, fields
}

// assemble writes a record body: its prose, then its fields in the parent
// schema's order, a field the schema does not name keeping its place after
// the named ones.
func assemble(prose string, fields []recordPart) string {
	rank := func(name string) int {
		if i := slices.Index(releaseFieldOrder, name); i >= 0 {
			return i
		}
		return len(releaseFieldOrder)
	}
	slices.SortStableFunc(fields, func(a, b recordPart) int { return rank(a.name) - rank(b.name) })
	out := "\n" + strings.Trim(prose, "\n") + "\n"
	if strings.Trim(prose, "\n") == "" {
		out = ""
	}
	for _, f := range fields {
		out += "\n" + f.text + "\n"
	}
	return out
}

// overviewFill is the overview scrum-master writes in place of init's placeholder.
const overviewFill = "Each release below names the objective items it delivers, in deliverables, and its milestone and tag, in release; the plan-write skill keeps them.\n"

// fillOverview replaces the overview field's placeholder sentence.
func fillOverview(doc string) string {
	start := strings.Index(doc, "<!-- field: overview -->")
	if start < 0 {
		return doc
	}
	end := len(doc)
	if next := nextLevel2(doc, start+1); next >= 0 {
		end = next
	}
	return doc[:start] + placeholder.ReplaceAllString(doc[start:end], overviewFill) + doc[end:]
}

// lastSRDs is the srds field of the road map's last release record, which a
// new record starts from, as the SRDs a release names change by decision,
// not by default; empty when there is none.
func lastSRDs(doc string) string {
	marks := releaseMarker.FindAllStringIndex(doc, -1)
	if len(marks) == 0 {
		return ""
	}
	record := doc[marks[len(marks)-1][0]:]
	if next := nextLevel2(record, len(record[:strings.Index(record, "\n")+1])); next >= 0 {
		record = record[:next]
	}
	for i, m := range recordField.FindAllStringSubmatchIndex(record, -1) {
		if record[m[2]:m[3]] != "srds" {
			continue
		}
		end := len(record)
		if all := recordField.FindAllStringIndex(record, -1); i+1 < len(all) {
			end = all[i+1][0]
		}
		return "\n" + strings.Trim(record[m[0]:end], "\n") + "\n"
	}
	return ""
}

// treeValue writes a tree item's value so it reads back as the same text:
// a value YAML would read as a number or a date, such as 01.1, is quoted,
// as the release's printer quotes it.
func treeValue(v string) string {
	b, err := yaml.Marshal(v)
	if err != nil {
		return v
	}
	return strings.TrimSuffix(string(b), "\n")
}

// nextLevel2 is the offset of the next marker above a level 2 heading at
// or after from, or -1: where the record that started before from ends.
func nextLevel2(doc string, from int) int {
	re := regexp.MustCompile(`(?m)^<!-- [a-z_]+: [^\n]*-->\n## `)
	if loc := re.FindStringIndex(doc[from:]); loc != nil {
		return from + loc[0]
	}
	return -1
}

// DesignGaps refuses a plan whose items' documents, or the epics, features,
// and stories under them (GH-90), cite a component of this repository that
// docs/ARCHITECTURE.md does not declare: the element
// needs a design decision first, which the design skill makes, rather than
// a boundary planning invents (srd031 R5.3, srd047; GH-58).
func DesignGaps(dir string, items []string) error {
	docs, err := hierarchy.Load(dir)
	if err != nil {
		return err
	}
	components, err := hierarchy.Components(dir)
	if err != nil {
		return err
	}
	named := map[string]bool{}
	for _, it := range items {
		id, _, _ := strings.Cut(strings.TrimSpace(it), "#")
		named[id] = true
	}
	byID := map[string]*hierarchy.Doc{}
	for _, d := range docs {
		byID[d.ID] = d
	}
	planned := func(d *hierarchy.Doc) bool {
		seen := map[string]bool{}
		for cur := d; cur != nil && !seen[cur.ID]; cur = byID[cur.Parent] {
			seen[cur.ID] = true
			if named[cur.ID] {
				return true
			}
		}
		return false
	}
	var gaps []string
	for _, d := range docs {
		if !planned(d) {
			continue
		}
		for _, c := range d.Architecture {
			if !strings.Contains(c, "://") && !components[c] && !slices.Contains(gaps, c+" (cited by "+d.ID+")") {
				gaps = append(gaps, c+" (cited by "+d.ID+")")
			}
		}
	}
	if len(gaps) > 0 {
		return fmt.Errorf("this needs a design decision: %s %s not in %s; run the design skill to add %s before planning the release",
			strings.Join(gaps, ", "), map[bool]string{true: "is", false: "are"}[len(gaps) == 1], hierarchy.ArchitectureFile, map[bool]string{true: "it", false: "them"}[len(gaps) == 1])
	}
	// A component the architecture moved since the release, which only
	// documents not yet agreed plan, is a plan nobody agreed: the release
	// waits for them (srd031 R5.3, GH-100).
	moves, err := backlog.ArchitectureMoves(dir, docs)
	if err != nil {
		return err
	}
	var drafts []string
	for _, d := range docs {
		if !planned(d) || d.Status == "agreed" {
			continue
		}
		for _, c := range d.Architecture {
			if covered, moved := moves[c]; moved && !covered {
				if s := c + " (cited by " + d.ID + ", " + d.Status + ")"; !slices.Contains(drafts, s) {
					drafts = append(drafts, s)
				}
			}
		}
	}
	if len(drafts) > 0 {
		return fmt.Errorf("this plan moves %s, which no agreed document plans; agree the citing documents before planning the release", strings.Join(drafts, ", "))
	}
	return nil
}
