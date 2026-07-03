// Copyright (c) 2026 Nokia. All rights reserved.

// Package plan defines the ImplementationPlan types produced by the
// planning engine and consumed by the materializer.
// Implements srd008-planning-engine R2 (implementation plan types).
package plan

// ImplementationPlan describes what files to create or modify, which
// requirements they satisfy, key design choices, and checkable outcomes.
// The struct mirrors the issue-format constitution schema so the
// materializer can map it directly to an issue description.
type ImplementationPlan struct {
	Title              string            `yaml:"title"`
	Summary            string            `yaml:"summary,omitempty"`
	Files              []PlanFile        `yaml:"files"`
	Requirements       []PlanRequirement `yaml:"requirements"`
	DesignDecisions    []PlanDecision    `yaml:"design_decisions,omitempty"`
	AcceptanceCriteria []PlanCriterion   `yaml:"acceptance_criteria"`
}

// PlanFile identifies a source file to create or modify.
type PlanFile struct {
	Path   string `yaml:"path"`
	Action string `yaml:"action"`
	Note   string `yaml:"note,omitempty"`
}

// PlanRequirement links a requirement ID to its implementation text.
type PlanRequirement struct {
	ID   string `yaml:"id"`
	Text string `yaml:"text"`
}

// PlanDecision captures a design choice with rationale.
type PlanDecision struct {
	ID   string `yaml:"id"`
	Text string `yaml:"text"`
}

// PlanCriterion defines a checkable acceptance outcome.
type PlanCriterion struct {
	ID   string `yaml:"id"`
	Text string `yaml:"text"`
}
