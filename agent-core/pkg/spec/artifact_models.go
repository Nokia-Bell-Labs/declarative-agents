// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import "sort"

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

// SRDIDs returns all SRD IDs from the spec index.
func (si *SpecIndex) SRDIDs() []string {
	ids := make([]string, len(si.SRDIndex))
	for i, e := range si.SRDIndex {
		ids[i] = e.ID
	}
	return ids
}
