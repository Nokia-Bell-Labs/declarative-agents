// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
	AcceptanceCriteria []AcceptanceCriterion       `yaml:"acceptance_criteria"`
	DependsOn          []Dependency                `yaml:"depends_on"`

	// Retirement fields (design.yaml D6). An SRD that no longer applies keeps
	// its id with status retired, the release that retired it, and the ids
	// that replace it. Absent means in force.
	Status     string   `yaml:"status"`
	RetiredIn  string   `yaml:"retired_in"`
	ReplacedBy []string `yaml:"replaced_by"`

	// NeedsClarification lists decisions nobody has made (design.yaml
	// clarification_marker). A document with a non-empty list is not ready.
	NeedsClarification []ClarificationEntry `yaml:"needs_clarification"`

	// Assumptions is the tagged list (A1, A2) of premises the requirements
	// rest on.
	Assumptions []string `yaml:"-"`

	// DerivedFrom records document-level provenance: the parent requirements
	// this SRD exists to satisfy (design.yaml external_references).
	DerivedFrom []ExternalReference `yaml:"derived_from"`

	// Extensions keeps every top-level key the model does not type, in
	// source order, so the document survives a parse unchanged.
	Extensions []Extension `yaml:"-"`

	OrderedGroups []string `yaml:"-"`
}

// Extension is one source key the model does not type, kept with its value
// in source order. The model does not interpret it; a printer reproduces it.
type Extension struct {
	Key   string
	Value yaml.Node
}

// ClarificationEntry is one unmade decision: the id it blocks and the
// question to answer. Resolving it means making the decision, editing the
// blocked entry in place, and deleting the entry.
type ClarificationEntry struct {
	ID       string `yaml:"id"`
	Question string `yaml:"question"`
}

// ExternalReference is one provenance backlink: the parent repository
// requirement this entry's carrier derives from, validated against a parent
// release. Release absent means the entry resolves against the cache HEAD,
// a co-development state the release gate refuses (design.yaml
// external_references).
type ExternalReference struct {
	Repo    string `yaml:"repo"`
	Release string `yaml:"release"`
	SRD     string `yaml:"srd"`
	Item    string `yaml:"item"`
	Reason  string `yaml:"reason"`
}

// RequirementGroup is a named group of requirement items (e.g. R1, R2).
type RequirementGroup struct {
	Title string
	Items []RequirementItem

	// Extensions keeps the group's untyped keys (e.g. applicability).
	Extensions []Extension
}

// RequirementItem is a single requirement within a group (e.g. R1.1).
type RequirementItem struct {
	ID     string
	Text   string
	Weight int

	// Retirement fields (design.yaml D6), set when the item is retired in
	// place rather than renumbered.
	Status     string
	RetiredIn  string
	ReplacedBy []string

	// DerivedFrom records why this item exists: the parent requirements it
	// derives from, each validated against a parent release.
	DerivedFrom []ExternalReference

	// Evidence says how the requirement is shown to hold; object-form items
	// carry it beside their text.
	Evidence string

	// Extensions keeps the item's untyped keys (e.g. endpoints).
	Extensions []Extension
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

	// Retirement fields (design.yaml D6). Absent means in force.
	Status     string   `yaml:"status"`
	RetiredIn  string   `yaml:"retired_in"`
	ReplacedBy []string `yaml:"replaced_by"`

	// DerivedFrom records why this criterion exists (design.yaml
	// external_references).
	DerivedFrom []ExternalReference `yaml:"derived_from"`

	// Extensions keeps the criterion's untyped keys (e.g. tests,
	// applicability).
	Extensions []Extension `yaml:"-"`
}

// acceptanceCriterionKeys are the keys AcceptanceCriterion types; any other
// key is kept as an extension.
var acceptanceCriterionKeys = map[string]bool{
	"id": true, "criterion": true, "traces": true, "status": true,
	"retired_in": true, "replaced_by": true, "derived_from": true,
}

// UnmarshalYAML decodes the typed fields and keeps every other key, in
// source order, as an extension.
func (ac *AcceptanceCriterion) UnmarshalYAML(value *yaml.Node) error {
	type plain AcceptanceCriterion
	var decoded plain
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*ac = AcceptanceCriterion(decoded)
	ac.Extensions = untypedKeys(value, acceptanceCriterionKeys)
	return nil
}

// untypedKeys returns a mapping node's entries whose keys are not in typed,
// in source order. A non-mapping node has none.
func untypedKeys(node *yaml.Node, typed map[string]bool) []Extension {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	var extensions []Extension
	for i := 0; i+1 < len(node.Content); i += 2 {
		key := node.Content[i].Value
		if typed[key] {
			continue
		}
		extensions = append(extensions, Extension{Key: key, Value: *node.Content[i+1]})
	}
	return extensions
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
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Status  string   `yaml:"status"`
	Summary string   `yaml:"summary"`
	Actor   string   `yaml:"actor"`
	Trigger string   `yaml:"trigger"`
	Flow    []string `yaml:"-"`
	// FlowTraces holds the requirement citations a flow step carries, keyed
	// by step id, for a step written as a record with traces.
	FlowTraces      map[string][]string `yaml:"-"`
	Touchpoints     []string            `yaml:"-"`
	SuccessCriteria []SuccessCriterion  `yaml:"success_criteria"`
	OutOfScope      []string            `yaml:"-"`
	TestSuite       string              `yaml:"test_suite"`

	// Retirement fields (design.yaml D6). ParseUseCase copies the document's
	// status only when it is the retirement marker; lifecycle statuses stay
	// unparsed so checkIndexDocumentAgreement keeps its existing silence.
	RetiredIn  string   `yaml:"retired_in"`
	ReplacedBy []string `yaml:"replaced_by"`

	// NeedsClarification lists decisions nobody has made (design.yaml
	// clarification_marker).
	NeedsClarification []ClarificationEntry `yaml:"needs_clarification"`

	// Assumptions (A1, A2) and EdgeCases (E1, E2) are tagged lists; Priority
	// ranks the use case within its release: P1 must ship, P2 should, P3
	// could, absent means unranked (design.yaml use_case priority).
	Assumptions []string `yaml:"-"`
	EdgeCases   []string `yaml:"-"`
	Priority    string   `yaml:"priority"`
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
	Status        string     `yaml:"status"`
	Release       string     `yaml:"release"`
	Traces        []string   `yaml:"traces"`
	Preconditions []string   `yaml:"preconditions"`
	TestCases     []TestCase `yaml:"test_cases"`

	// Retirement fields (design.yaml D6). Absent means in force.
	RetiredIn  string   `yaml:"retired_in"`
	ReplacedBy []string `yaml:"replaced_by"`

	// NeedsClarification lists decisions nobody has made (design.yaml
	// clarification_marker).
	NeedsClarification []ClarificationEntry `yaml:"needs_clarification"`
}

// TestCase is one test case within a test suite.
type TestCase struct {
	Name        string   `yaml:"name"`
	UseCase     string   `yaml:"use_case"`
	Description string   `yaml:"description"`
	Traces      []string `yaml:"traces"`
	// GoTest is the formal evidence string naming the Go test(s) that validate
	// this case: a bare test name, a comma-separated list, a "go test <pkgs>
	// [-run <regex>]" command, or a Mage/descriptive label. ValidateGoTestEvidence
	// checks the executable forms against the real test inventory, except on a
	// planned case, where it keeps syntax checks and skips inventory resolution.
	GoTest string `yaml:"go_test"`
	// Status is the case's own claim about its evidence. The field is optional --
	// the test_suite format rule does not require it -- so an absent status is
	// read as a live claim by the jurist evidence reducer and the go_test
	// resolver: a case that names a test and says nothing else is asserting that
	// test as its proof. Only "planned" withholds the claim.
	Status string `yaml:"status"`
}

// Roadmap is the parsed road-map.md.
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

// SpecIndex is the parsed SPECIFICATIONS.md file.
type SpecIndex struct {
	ID             string           `yaml:"id"`
	Title          string           `yaml:"title"`
	Overview       string           `yaml:"overview"`
	RoadmapSummary []RoadmapEntry   `yaml:"roadmap_summary"`
	SRDIndex       []SRDEntry       `yaml:"srd_index"`
	UseCaseIndex   []UseCaseEntry   `yaml:"use_case_index"`
	TestSuiteIndex []TestSuiteEntry `yaml:"test_suite_index"`

	FoundationDocumentIndex []DocumentEntry `yaml:"foundation_document_index"`
	ConfigFormatIndex       []DocumentEntry `yaml:"config_format_index"`
	SemanticModelIndex      []DocumentEntry `yaml:"semantic_model_index"`
}

// DocumentEntry is a row of an index table that names a document by id and
// path: the foundation document, config format, and semantic model indexes.
type DocumentEntry struct {
	ID      string `yaml:"id"`
	Title   string `yaml:"title"`
	Summary string `yaml:"summary"`
	Path    string `yaml:"path"`
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

// SRDIDs returns all SRD IDs from the spec index.
func (si *SpecIndex) SRDIDs() []string {
	ids := make([]string, len(si.SRDIndex))
	for i, e := range si.SRDIndex {
		ids[i] = e.ID
	}
	return ids
}
