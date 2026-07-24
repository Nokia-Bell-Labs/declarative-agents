// Copyright (c) 2026 Nokia. All rights reserved.

package plan

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

type issueDescription struct {
	DeliverableType    string            `yaml:"deliverable_type"`
	RequiredReading    []string          `yaml:"required_reading"`
	Files              []PlanFile        `yaml:"files"`
	Requirements       []PlanRequirement `yaml:"requirements"`
	DesignDecisions    []PlanDecision    `yaml:"design_decisions,omitempty"`
	AcceptanceCriteria []PlanCriterion   `yaml:"acceptance_criteria"`
}

// FormatIssueDescription produces issue-format YAML from an implementation
// plan without choosing or invoking an issue tracker.
func FormatIssueDescription(p ImplementationPlan) (string, error) {
	reading := make([]string, len(p.Files))
	for i, file := range p.Files {
		reading[i] = file.Path
	}
	data, err := yaml.Marshal(issueDescription{
		DeliverableType:    "code",
		RequiredReading:    reading,
		Files:              p.Files,
		Requirements:       p.Requirements,
		DesignDecisions:    p.DesignDecisions,
		AcceptanceCriteria: p.AcceptanceCriteria,
	})
	if err != nil {
		return "", fmt.Errorf("format issue description: %w", err)
	}
	return string(data), nil
}
