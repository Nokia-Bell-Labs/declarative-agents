// Copyright (c) 2026 Nokia. All rights reserved.

// Package spec parses specification artifacts (SRDs, use cases, test
// suites, roadmaps) into typed structs and builds a labeled property
// graph for cross-artifact validation.
package spec

import (
	"sort"

	"gopkg.in/yaml.v3"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// SRD represents a parsed Software Requirements Document.
type SRD struct {
	ID                 string                      `yaml:"id"`
	Title              string                      `yaml:"title"`
	Problem            string                      `yaml:"problem"`
	Goals              []string                    `yaml:"-"`
	Requirements       map[string]RequirementGroup `yaml:"-"`
	NonGoals           []string                    `yaml:"-"`
	AcceptanceCriteria []AcceptanceCriterion       `yaml:"acceptance_criteria"`
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

// ToolDeclaration captures tool contract fields needed for public spec-corpus
// validation. It mirrors the runtime ToolDef fields owned by
// internal/tools/catalog without importing an internal package.
type ToolDeclaration struct {
	Name          string                `yaml:"name"`
	Type          string                `yaml:"type,omitempty"`
	Category      string                `yaml:"category,omitempty"`
	Contract      string                `yaml:"contract,omitempty"`
	Init          string                `yaml:"init,omitempty"`
	Problem       string                `yaml:"problem,omitempty"`
	Goals         []string              `yaml:"goals,omitempty"`
	Requirements  ToolDeclRequirements  `yaml:"requirements,omitempty"`
	NonGoals      []string              `yaml:"non_goals,omitempty"`
	Emits         []string              `yaml:"emits,omitempty"`
	Output        ToolDeclOutput        `yaml:"output,omitempty"`
	Metrics       core.MetricConfig     `yaml:"metrics,omitempty"`
	Visibility    string                `yaml:"visibility,omitempty"`
	Reversibility ToolDeclReversibility `yaml:"reversibility,omitempty"`
	Undo          ToolDeclUndo          `yaml:"undo,omitempty"`
	SideEffects   ToolDeclSideEffects   `yaml:"side_effects,omitempty"`
	Errors        []ToolDeclError       `yaml:"errors,omitempty"`
	Relationships ToolDeclRelationships `yaml:"relationships,omitempty"`
	SourceFile    string                `yaml:"-"`
}

// ToolDeclRequirements captures observable behavior requirements used by the
// audit without importing the runtime STL package.
type ToolDeclRequirements struct {
	Input  []string `yaml:"input,omitempty"`
	Output []string `yaml:"output,omitempty"`
	Errors []string `yaml:"errors,omitempty"`
}

// ToolDeclOutput captures the declared machine-readable result shape.
type ToolDeclOutput struct {
	Schema map[string]any `yaml:"schema,omitempty"`
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

// ToolDeclError captures a declared failure mode.
type ToolDeclError struct {
	Signal string `yaml:"signal,omitempty"`
}

// ToolDeclRelationships captures sequencing and overlap documentation.
type ToolDeclRelationships struct {
	Before   []string                  `yaml:"before,omitempty"`
	After    []string                  `yaml:"after,omitempty"`
	Overlaps []ToolDeclRelationshipRef `yaml:"overlaps,omitempty"`
}

// ToolDeclRelationshipRef captures one related tool reference.
type ToolDeclRelationshipRef struct {
	Tool string `yaml:"tool,omitempty"`
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

// DocSpec represents a parsed semantic-model or config-format YAML spec.
// It captures only the fields needed for cross-reference validation.
type DocSpec struct {
	ID                 string           `yaml:"id"`
	Title              string           `yaml:"title"`
	RequirementsSource DocSpecSources   `yaml:"requirements_source,omitempty"`
	RelatedDocuments   []string         `yaml:"related_documents,omitempty"`
	Implementation     DocSpecImpl      `yaml:"implementation,omitempty"`
	Examples           []DocSpecExample `yaml:"examples,omitempty"`
	SourceFile         string           `yaml:"-"`
}

// DocSpecSources handles both flat list and canonical/historical forms.
type DocSpecSources struct {
	Canonical            []string `yaml:"canonical,omitempty"`
	HistoricalBackground []string `yaml:"historical_background,omitempty"`
}

func (s *DocSpecSources) UnmarshalYAML(value *yaml.Node) error {
	type plain DocSpecSources
	var structured plain
	if err := value.Decode(&structured); err == nil && (len(structured.Canonical) > 0 || len(structured.HistoricalBackground) > 0) {
		*s = DocSpecSources(structured)
		return nil
	}
	var flat []string
	if err := value.Decode(&flat); err == nil {
		s.Canonical = flat
		return nil
	}
	return nil
}

// AllPaths returns all canonical and historical source paths.
func (s *DocSpecSources) AllPaths() []string {
	return append(append([]string(nil), s.Canonical...), s.HistoricalBackground...)
}

// DocSpecImpl handles implementation as either a single string or list.
type DocSpecImpl struct {
	Paths []string
}

func (d *DocSpecImpl) UnmarshalYAML(value *yaml.Node) error {
	var list []string
	if err := value.Decode(&list); err == nil {
		d.Paths = list
		return nil
	}
	var single string
	if err := value.Decode(&single); err == nil && single != "" {
		d.Paths = []string{single}
		return nil
	}
	return nil
}

// DocSpecExample is one example entry with a file path.
type DocSpecExample struct {
	File string `yaml:"file"`
}

// KnownSideEffectKinds is the canonical vocabulary for side_effects kind values.
var KnownSideEffectKinds = map[string]bool{
	"filesystem_read":           true,
	"filesystem_write":          true,
	"command_state":             true,
	"state_mutation":            true,
	"state_read":                true,
	"child_tool_execution":      true,
	"child_agent_execution":     true,
	"child_process":             true,
	"nested_machine_execution":  true,
	"external_api":              true,
	"external_api_call":         true,
	"network_listen":            true,
	"network_listener_shutdown": true,
	"human_boundary":            true,
	"stderr_write":              true,
	"none":                      true,
}

// SRDIDs returns all SRD IDs from the spec index.
func (si *SpecIndex) SRDIDs() []string {
	ids := make([]string, len(si.SRDIndex))
	for i, e := range si.SRDIndex {
		ids[i] = e.ID
	}
	return ids
}
