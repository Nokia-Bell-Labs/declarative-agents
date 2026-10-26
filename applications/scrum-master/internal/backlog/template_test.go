// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package backlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/scrum-master/internal/hierarchy"
)

// Every level has a template, and laid out together they pass the
// hierarchy gate, so the layout a writer copies is the one the gate reads
// (GH-20).
func TestTemplatesPassTheGate(t *testing.T) {
	dir := t.TempDir()
	folders := map[string]string{"theme": "themes", "initiative": "initiatives", "epic": "epics", "feature": "features", "story": "stories"}
	for _, k := range kinds() {
		folder, ok := folders[k]
		if !ok {
			continue
		}
		doc, err := Template(k)
		if err != nil {
			t.Fatal(err)
		}
		name := map[string]string{"theme": "th001-dependable-agents", "initiative": "in001-reliable-agents", "epic": "ep001-agent-evaluation", "feature": "ft001-grader", "story": "st001-score-trajectories"}[k]
		path := filepath.Join(dir, "docs", "specs", folder, name+".md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The templates cite the ARCHITECTURE template's component (GH-55).
	architecture, err := Template("architecture")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "docs", "ARCHITECTURE.md"), []byte(architecture), 0o644); err != nil {
		t.Fatal(err)
	}
	docs, err := hierarchy.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range docs {
		if len(d.Architecture) == 0 {
			t.Errorf("the %s template cites no architecture component", d.Kind)
		}
	}
	if len(docs) != 5 {
		t.Fatalf("loaded %d documents, want 5", len(docs))
	}
	if f := hierarchy.Validate(docs); len(f) != 0 {
		t.Fatalf("the templates fail the gate: %+v", f)
	}
	var epic *hierarchy.Doc
	for _, d := range docs {
		if d.Kind == hierarchy.Epic {
			epic = d
		}
	}
	// A nested list numbers every item "1." (GH-24).
	if epic == nil || len(epic.Links["depends_on"]) != 2 {
		t.Fatalf("the epic template's depends_on does not read as two links: %+v", epic)
	}
	if _, err := Template("no-such-kind"); err == nil {
		t.Fatal("an unknown kind must be refused")
	}
}

// VISION and ARCHITECTURE have templates too, laid out as init writes them
// but with example content, so the gate's shape check need not be learned
// from its errors (GH-22).
func TestVisionAndArchitectureTemplates(t *testing.T) {
	for kind, fields := range map[string][]string{
		"vision":       {"executive_summary", "problem", "what_this_does", "why_we_build_this", "success_criteria", "not", "canonical_documents"},
		"architecture": {"overview", "interfaces", "components", "design_decisions", "technology_choices", "project_structure", "implementation_status", "related_documents"},
	} {
		doc, err := Template(kind)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range fields {
			if !strings.Contains(doc, "<!-- field: "+f+" -->") {
				t.Errorf("%s template lacks field %s", kind, f)
			}
		}
		if strings.Contains(doc, placeholder) {
			t.Errorf("%s template reads as unfilled", kind)
		}
	}
	arch, _ := Template("architecture")
	for _, want := range []string{"<!-- interface: run_records -->\n### run_records\n", "<!-- component: grader -->\n### grader\n", "<!-- decision: 1 -->\n### 1 - "} {
		if !strings.Contains(arch, want) {
			t.Errorf("architecture template lacks %q", want)
		}
	}
}
