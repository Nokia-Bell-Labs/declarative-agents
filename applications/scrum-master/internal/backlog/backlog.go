// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
// Package backlog is scrum-master backlog: the planning hierarchy of one repository,
// read and checked (srd045 R8). It reads only the directory it is given and
// writes nothing, so a skill can call it for facts (design.md D10).
package backlog

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/cli"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/corpus"
	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
)

// Entry is one hierarchy document as scrum-master backlog list prints it.
type Entry struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Parent string `json:"parent,omitempty"`
	Path   string `json:"path"`
	Points string `json:"points,omitempty"`
	Rank   string `json:"rank,omitempty"`
	// The horizontal links the document carries (GH-20): what it is
	// blocked by, depends on, relates to, and duplicates.
	BlockedBy  []string `json:"blocked_by,omitempty"`
	DependsOn  []string `json:"depends_on,omitempty"`
	RelatesTo  []string `json:"relates_to,omitempty"`
	Duplicates []string `json:"duplicates,omitempty"`
	// Architecture is the ARCHITECTURE components the document cites
	// (GH-55).
	Architecture []string `json:"architecture,omitempty"`
}

// Report is what scrum-master backlog check prints: how many documents it read and
// every finding the gate reports on them. SpecSkipped says why the parent
// spec tool's shape check did not run, when it did not.
type Report struct {
	Documents int `json:"documents"`
	// Hierarchy is how many of the documents are hierarchy documents, the
	// count doctor's bootstrapping order names (GH-90).
	Hierarchy   int                 `json:"hierarchy"`
	Findings    []hierarchy.Finding `json:"findings"`
	SpecSkipped string              `json:"spec_check_skipped,omitempty"`
	// Unfilled are the documents that still hold scrum-master init's or onboard's
	// placeholder text: the gate passes them, since a repository is
	// onboarded before its owner writes them, and names them so a pass is
	// not taken for a written corpus.
	Unfilled []string `json:"unfilled,omitempty"`
	// Warnings are the architecture and area checks' findings under a policy that
	// warns, the default (srd045 R6.3): reported, never failing the gate.
	Warnings []hierarchy.Finding `json:"warnings"`
}

// shaped are the documents beside the hierarchy and the SRDs whose shape
// the gate checks, and counts, when the repository has them.
var shaped = []string{"docs/VISION.md", "docs/ARCHITECTURE.md"}

// placeholder is the phrase every placeholder sentence init and onboard
// write carries: "<Owner> writes here ...".
const placeholder = " writes here "

// List reads the hierarchy documents under dir, keeping one kind when kind
// is not empty; paths are relative to dir.
func List(dir, kind string) ([]Entry, error) {
	docs, err := hierarchy.Load(dir)
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	for _, d := range docs {
		if kind != "" && string(d.Kind) != kind {
			continue
		}
		e := Entry{ID: d.ID, Kind: string(d.Kind), Title: d.Title, Status: d.Status, Parent: d.Parent, Path: relative(dir, d.Path),
			BlockedBy: d.Links["blocked_by"], DependsOn: d.Links["depends_on"], RelatesTo: d.Links["relates_to"], Duplicates: d.Links["duplicates"],
			Architecture: d.Architecture}
		if d.Kind == hierarchy.Story {
			e.Points, e.Rank = d.Points, d.Rank
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// Check runs the hierarchy gate (srd045 R6.1) on the documents under dir,
// through hierarchy.Validate, the function the mage audit gate calls; it
// also reads the SRDs, refusing a requirement no criterion traces, and runs
// the parent spec tool's shape check, so the gate refuses what scrum-master doctor
// would (srd043 R4.4).
func Check(dir string) (Report, error) {
	docs, err := hierarchy.Load(dir)
	if err != nil {
		return Report{}, err
	}
	findings := hierarchy.Validate(docs)
	srds, untraced, err := checkSRDs(dir)
	if err != nil {
		return Report{}, err
	}
	statuses, err := releaseStatuses(dir)
	if err != nil {
		return Report{}, err
	}
	untraced = append(untraced, statuses...)
	shapes, skipped := checkShapes(dir)
	findings = append(append(findings, untraced...), shapes...)
	// A document git does not track is reported as untracked and nothing
	// else: the checks that read it would speak of a file the shape check
	// cannot (GH-100).
	unread := untracked(dir)
	skip := map[string]bool{}
	for _, u := range unread {
		if abs, err := filepath.Abs(u.File); err == nil {
			skip[abs] = true
		}
	}
	findings = slices.DeleteFunc(findings, func(f hierarchy.Finding) bool {
		abs, err := filepath.Abs(f.File)
		return err == nil && skip[abs]
	})
	findings = append(findings, unread...)
	for i := range findings {
		findings[i].File = relative(dir, findings[i].File)
	}
	architecture, refuse, err := architectureChecks(dir, docs)
	if err != nil {
		return Report{}, err
	}
	warnings := []hierarchy.Finding{}
	if refuse {
		findings = append(findings, architecture...)
	} else if architecture != nil {
		warnings = architecture
	}
	area, refuseArea, err := areaChecks(dir, docs)
	if err != nil {
		return Report{}, err
	}
	for i := range area {
		area[i].File = relative(dir, area[i].File)
	}
	if refuseArea {
		findings = append(findings, area...)
	} else {
		warnings = append(warnings, area...)
	}
	if findings == nil {
		findings = []hierarchy.Finding{}
	}
	findings = append(findings, bootstrapFinding(dir, len(docs))...)
	top, unfilled, err := corpusFiles(dir)
	if err != nil {
		return Report{}, err
	}
	return Report{Documents: len(docs) + srds + top, Hierarchy: len(docs), Findings: findings, SpecSkipped: skipped, Unfilled: unfilled, Warnings: warnings}, nil
}

// corpusFiles counts the shaped documents dir holds and lists, relative to
// dir, the corpus documents that still hold placeholder text.
func corpusFiles(dir string) (top int, unfilled []string, err error) {
	for _, p := range shaped {
		if _, err := os.Stat(filepath.Join(dir, p)); err == nil {
			top++
		} else if !errors.Is(err, fs.ErrNotExist) {
			return 0, nil, err
		}
	}
	err = filepath.WalkDir(filepath.Join(dir, "docs"), func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return fs.SkipAll
		}
		if err != nil || d.IsDir() || filepath.Ext(path) != ".md" {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(b), placeholder) {
			unfilled = append(unfilled, relative(dir, path))
		}
		return nil
	})
	return top, unfilled, err
}

func relative(dir, path string) string {
	if rel, err := filepath.Rel(dir, path); err == nil {
		return rel
	}
	return path
}

func kinds() []string {
	var ks []string
	for _, l := range hierarchy.Levels {
		ks = append(ks, string(l.Kind))
	}
	return ks
}

func init() {
	var la struct{ dir, kind string }
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "list", Summary: "the repository's hierarchy documents: id, kind, title, status, parent, and path",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&la.dir, "dir", ".", "the repository's checkout")
			fs.StringVar(&la.kind, "kind", "", "keep one kind: "+strings.Join(kinds(), ", "))
		},
		Run: func(ctx *cli.Context) error {
			if la.kind != "" && !strings.Contains(" "+strings.Join(kinds(), " ")+" ", " "+la.kind+" ") {
				return fmt.Errorf("%w: --kind is one of %s", cli.ErrUsage, strings.Join(kinds(), ", "))
			}
			entries, err := List(la.dir, la.kind)
			if err != nil {
				return err
			}
			var h strings.Builder
			for _, e := range entries {
				fmt.Fprintf(&h, "%-12s %-40s %-12s parent=%s  %s\n", e.Kind, e.ID, e.Status, orNone(e.Parent), e.Title)
			}
			if len(entries) == 0 {
				h.WriteString("no hierarchy documents\n")
			}
			return ctx.Emit(entries, h.String())
		},
	})

	var ca struct{ dir string }
	cli.Default.Register(&cli.Command{
		Group: "backlog", Name: "check", Summary: "the hierarchy gate on the repository's documents; exits non-zero on any finding",
		Flags: func(fs *flag.FlagSet) {
			fs.StringVar(&ca.dir, "dir", ".", "the repository's checkout")
		},
		Run: func(ctx *cli.Context) error {
			r, err := Check(ca.dir)
			if err != nil {
				return err
			}
			var h strings.Builder
			for _, f := range r.Findings {
				fmt.Fprintln(&h, f.String())
			}
			if len(r.Findings) == 0 {
				fmt.Fprintf(&h, "hierarchy gate: pass, %d documents, %d of them hierarchy documents\n", r.Documents, r.Hierarchy)
			}
			for _, w := range r.Warnings {
				fmt.Fprintf(&h, "warning: %s\n", w.String())
			}
			if len(r.Unfilled) > 0 {
				fmt.Fprintf(&h, "unfilled: %s still hold the placeholder text scrum-master init or onboard wrote\n", strings.Join(r.Unfilled, ", "))
			}
			if r.SpecSkipped != "" {
				fmt.Fprintf(&h, "spec shape check skipped: %s\n", r.SpecSkipped)
			}
			if err := ctx.Emit(r, h.String()); err != nil {
				return err
			}
			if len(r.Findings) > 0 {
				return fmt.Errorf("hierarchy gate: %d findings in %d documents", len(r.Findings), r.Documents)
			}
			return nil
		},
	})
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// ReleaseStatuses are the values a road-map release record's status takes:
// planned, built and proven, and tagged and handed off (srd031 R4.1,
// design.md roadmap).
var ReleaseStatuses = []string{"specified", "implemented", "released"}

// releaseStatuses reports each road-map release record whose status is not
// one of ReleaseStatuses (srd045 R8.2). A repository without a road map has
// none.
func releaseStatuses(dir string) ([]hierarchy.Finding, error) {
	path := filepath.Join(dir, "docs", "road-map.md")
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	d, err := corpus.ParseMarkdown(b)
	if err != nil {
		return nil, nil // the shape check reports a road map that does not parse
	}
	var out []hierarchy.Finding
	for _, r := range d.Records("release") {
		if status := r.Attrs["status"]; !slices.Contains(ReleaseStatuses, status) {
			out = append(out, hierarchy.Finding{File: path, Field: "release " + r.Key + " status",
				Message: fmt.Sprintf("%q is not a release status; use %s", status, strings.Join(ReleaseStatuses, ", "))})
		}
	}
	return out, nil
}
