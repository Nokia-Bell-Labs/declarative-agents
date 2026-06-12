// Copyright (c) 2026 Nokia. All rights reserved.

// Package spec parses specification artifacts (SRDs, use cases, test
// suites, roadmaps) into typed structs and builds a labeled property
// graph for cross-artifact validation.
package spec

import (
	"sort"

	"gopkg.in/yaml.v3"
)

// SRD represents a parsed Software Requirements Document.
type SRD struct {
	ID                 string                      `yaml:"id"`
	Title              string                      `yaml:"title"`
	Problem            string                      `yaml:"problem"`
	Goals              []string                    `yaml:"-"`
	Requirements       map[string]RequirementGroup `yaml:"-"`
	NonGoals           []string                    `yaml:"-"`
	AcceptanceCriteria []AcceptanceCriterion        `yaml:"acceptance_criteria"`
	DependsOn          []Dependency                `yaml:"depends_on"`

	OrderedGroups []string `yaml:"-"`
}

// RequirementGroup is a named group of requirement items (e.g. R1, R2).
type RequirementGroup struct {
	Title string
	Items []RequirementItem
}

// RequirementItem is a single requirement within a group (e.g. R1.1).
type RequirementItem struct {
	ID     string
	Text   string
	Weight int
}

// Dependency records an inter-SRD dependency.
type Dependency struct {
	SRDID       string   `yaml:"srd_id"`
	SymbolsUsed []string `yaml:"symbols_used"`
}

// AcceptanceCriterion ties a testable criterion to requirement traces.
type AcceptanceCriterion struct {
	ID        string   `yaml:"id"`
	Criterion string   `yaml:"criterion"`
	Traces    []string `yaml:"traces"`
}

// AllItems returns all requirement items from the SRD in group order.
func (s *SRD) AllItems() []RequirementItem {
	var all []RequirementItem
	for _, gk := range s.OrderedGroups {
		g := s.Requirements[gk]
		all = append(all, g.Items...)
	}
	return all
}

// ItemIDs returns a sorted list of all requirement item IDs in this SRD.
func (s *SRD) ItemIDs() []string {
	items := s.AllItems()
	ids := make([]string, len(items))
	for i, it := range items {
		ids[i] = it.ID
	}
	sort.Strings(ids)
	return ids
}

// UseCase represents a parsed use case specification.
type UseCase struct {
	ID              string             `yaml:"id"`
	Title           string             `yaml:"title"`
	Summary         string             `yaml:"summary"`
	Actor           string             `yaml:"actor"`
	Trigger         string             `yaml:"trigger"`
	Flow            []string           `yaml:"-"`
	Touchpoints     []string           `yaml:"-"`
	SuccessCriteria []SuccessCriterion `yaml:"success_criteria"`
	OutOfScope      []string           `yaml:"-"`
	TestSuite       string             `yaml:"test_suite"`
}

// SuccessCriterion is one success criterion in a use case.
type SuccessCriterion struct {
	ID        string   `yaml:"id"`
	Criterion string   `yaml:"criterion"`
	Traces    []string `yaml:"traces"`
}

// TestSuite represents a parsed test suite specification.
type TestSuite struct {
	ID            string     `yaml:"id"`
	Title         string     `yaml:"title"`
	Release       string     `yaml:"release"`
	Traces        []string   `yaml:"traces"`
	Preconditions []string   `yaml:"preconditions"`
	TestCases     []TestCase `yaml:"test_cases"`
}

// TestCase is one test case within a test suite.
type TestCase struct {
	Name        string   `yaml:"name"`
	UseCase     string   `yaml:"use_case"`
	Description string   `yaml:"description"`
	Traces      []string `yaml:"traces"`
}

// Roadmap is the parsed road-map.yaml.
type Roadmap struct {
	ID       string    `yaml:"id"`
	Title    string    `yaml:"title"`
	Releases []Release `yaml:"releases"`
}

// Release describes one release in the roadmap.
type Release struct {
	Version     string       `yaml:"version"`
	Name        string       `yaml:"name"`
	Description string       `yaml:"description"`
	Status      string       `yaml:"status"`
	UseCases    []UseCaseRef `yaml:"use_cases"`
}

// UseCaseRef is a brief reference to a use case within a release.
type UseCaseRef struct {
	ID      string `yaml:"id"`
	Summary string `yaml:"summary"`
	Status  string `yaml:"status"`
}

// ReleaseVersions returns the ordered list of release version strings.
func (rm *Roadmap) ReleaseVersions() []string {
	versions := make([]string, len(rm.Releases))
	for i, r := range rm.Releases {
		versions[i] = r.Version
	}
	return versions
}

// SpecIndex is the parsed SPECIFICATIONS.yaml file.
type SpecIndex struct {
	ID             string           `yaml:"id"`
	Title          string           `yaml:"title"`
	Overview       string           `yaml:"overview"`
	RoadmapSummary []RoadmapEntry   `yaml:"roadmap_summary"`
	SRDIndex       []SRDEntry       `yaml:"srd_index"`
	UseCaseIndex   []UseCaseEntry   `yaml:"use_case_index"`
	TestSuiteIndex []TestSuiteEntry `yaml:"test_suite_index"`
}

// RoadmapEntry is a release summary within the spec index.
type RoadmapEntry struct {
	Version       string `yaml:"version"`
	Name          string `yaml:"name"`
	UseCasesDone  int    `yaml:"use_cases_done"`
	UseCasesTotal int    `yaml:"use_cases_total"`
	Status        string `yaml:"status"`
}

// SRDEntry is a single SRD reference in the spec index.
type SRDEntry struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Summary string `yaml:"summary"`
	Path    string `yaml:"path"`
}

// UseCaseEntry is a single use case reference in the spec index.
type UseCaseEntry struct {
	ID        string `yaml:"id"`
	Title     string `yaml:"title"`
	Release   string `yaml:"release"`
	Status    string `yaml:"status"`
	TestSuite string `yaml:"test_suite"`
	Path      string `yaml:"path"`
}

// TestSuiteEntry is a single test suite reference in the spec index.
type TestSuiteEntry struct {
	ID            string   `yaml:"id"`
	Title         string   `yaml:"title"`
	Release       string   `yaml:"release"`
	TestCaseCount int      `yaml:"test_case_count"`
	Traces        []string `yaml:"traces"`
	Path          string   `yaml:"path"`
}

// ToolSelection is a parsed agents/*/tools.yaml file listing the tool
// names selected for a particular agent mode.
type ToolSelection struct {
	Tools []string `yaml:"tools"`
}

// ToolDeclaration captures tool contract fields needed for corpus
// validation. This is a subset of the full ToolDef in pkg/stl, defined
// here to avoid a circular dependency.
type ToolDeclaration struct {
	Name          string                `yaml:"name"`
	Type          string                `yaml:"type,omitempty"`
	Category      string                `yaml:"category,omitempty"`
	Init          string                `yaml:"init,omitempty"`
	Emits         []string              `yaml:"emits,omitempty"`
	Visibility    string                `yaml:"visibility,omitempty"`
	Reversibility ToolDeclReversibility `yaml:"reversibility,omitempty"`
	Undo          ToolDeclUndo          `yaml:"undo,omitempty"`
	SideEffects   ToolDeclSideEffects   `yaml:"side_effects,omitempty"`
	SourceFile    string                `yaml:"-"`
}

// ToolDeclReversibility captures the reversibility classification.
type ToolDeclReversibility struct {
	Classification string `yaml:"classification,omitempty"`
}

// ToolDeclUndo captures the undo contract.
type ToolDeclUndo struct {
	Strategy string   `yaml:"strategy,omitempty"`
	Payload  string   `yaml:"payload,omitempty"`
	Captures []string `yaml:"captures,omitempty"`
}

// ToolDeclSideEffects handles both structured and legacy side_effects.
type ToolDeclSideEffects struct {
	Items []ToolDeclSideEffect
}

// ToolDeclSideEffect captures one structured side-effect entry.
type ToolDeclSideEffect struct {
	Kind string `yaml:"kind"`
}

func (s *ToolDeclSideEffects) UnmarshalYAML(value *yaml.Node) error {
	var items []ToolDeclSideEffect
	if err := value.Decode(&items); err == nil {
		s.Items = items
		return nil
	}
	return nil
}

// ToolDeclFile is the top-level YAML structure for a tool declaration file.
type ToolDeclFile struct {
	Tools []ToolDeclaration `yaml:"tools"`
}

// KnownSideEffectKinds is the canonical vocabulary for side_effects kind values.
var KnownSideEffectKinds = map[string]bool{
	"filesystem_read":          true,
	"filesystem_write":         true,
	"command_state":            true,
	"state_mutation":           true,
	"state_read":               true,
	"child_tool_execution":     true,
	"child_agent_execution":    true,
	"child_process":            true,
	"nested_machine_execution": true,
	"external_api":             true,
	"human_boundary":           true,
	"stderr_write":             true,
	"none":                     true,
}

// SRDIDs returns all SRD IDs from the spec index.
func (si *SpecIndex) SRDIDs() []string {
	ids := make([]string, len(si.SRDIndex))
	for i, e := range si.SRDIndex {
		ids[i] = e.ID
	}
	return ids
}
