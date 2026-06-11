// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import "fmt"

const (
	ContractSeverityInfo    = "info"
	ContractSeverityWarning = "warning"
	ContractSeverityError   = "error"
)

// ContractValidationOptions controls how strictly tool vocabulary contracts
// are checked. The validator is intentionally side-effect free so it can run in
// warn-only migration mode over existing declarations.
type ContractValidationOptions struct {
	Strict                       bool
	IncludeInternal              bool
	RequireStructuredSideEffects bool
	MinimumLevel                 string
}

// ContractFinding is one actionable tool contract validation result.
type ContractFinding struct {
	ToolName    string
	Field       string
	Severity    string
	Category    string
	Message     string
	Remediation string
}

// ValidateToolContracts reports missing or legacy vocabulary contract metadata
// for tool declarations. It does not mutate definitions or affect runtime tool
// execution.
func ValidateToolContracts(defs []ToolDef, opts ContractValidationOptions) []ContractFinding {
	var findings []ContractFinding
	for _, def := range defs {
		category := contractCategory(def)
		if category == "internal" && !opts.IncludeInternal {
			continue
		}

		findings = appendIfIncluded(findings, missingString(def.Name, category, "problem", def.Problem, opts), opts)
		findings = appendIfIncluded(findings, missingList(def.Name, category, "goals", len(def.Goals), opts), opts)
		for _, finding := range missingRequirements(def, category, opts) {
			findings = appendIfIncluded(findings, finding, opts)
		}
		findings = appendIfIncluded(findings, missingList(def.Name, category, "non_goals", len(def.NonGoals), opts), opts)
		findings = appendIfIncluded(findings, missingOutputSchema(def, category, opts), opts)
		findings = appendIfIncluded(findings, missingReversibility(def, category, opts), opts)
		findings = appendIfIncluded(findings, missingRelationships(def, category, opts), opts)

		if def.SideEffects.LegacyText != "" {
			findings = appendIfIncluded(findings, ContractFinding{
				ToolName:    def.Name,
				Field:       "side_effects",
				Severity:    severity(opts),
				Category:    category,
				Message:     fmt.Sprintf("tool %q uses legacy scalar side_effects", def.Name),
				Remediation: "replace scalar side_effects with structured entries declaring kind, target, paths, state, and description",
			}, opts)
		}
		if opts.RequireStructuredSideEffects && len(def.SideEffects.Items) == 0 {
			findings = appendIfIncluded(findings, ContractFinding{
				ToolName:    def.Name,
				Field:       "side_effects",
				Severity:    severity(opts),
				Category:    category,
				Message:     fmt.Sprintf("tool %q does not declare structured side_effects", def.Name),
				Remediation: "add side_effects as a structured list; use kind: none for read-only tools",
			}, opts)
		}
	}
	return findings
}

func contractCategory(def ToolDef) string {
	if def.Visibility == "internal" {
		return "internal"
	}
	if def.Type == "builtin" {
		return "external_builtin"
	}
	return "exec"
}

func severity(opts ContractValidationOptions) string {
	if opts.Strict {
		return ContractSeverityError
	}
	return ContractSeverityWarning
}

func missingString(toolName, category, field, value string, opts ContractValidationOptions) ContractFinding {
	if value != "" {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    toolName,
		Field:       field,
		Severity:    severity(opts),
		Category:    category,
		Message:     fmt.Sprintf("tool %q is missing %s", toolName, field),
		Remediation: fmt.Sprintf("add %s to explain this tool's vocabulary contract", field),
	}
}

func missingList(toolName, category, field string, count int, opts ContractValidationOptions) ContractFinding {
	if count > 0 {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    toolName,
		Field:       field,
		Severity:    severity(opts),
		Category:    category,
		Message:     fmt.Sprintf("tool %q is missing %s", toolName, field),
		Remediation: fmt.Sprintf("add at least one %s entry", field),
	}
}

func missingRequirements(def ToolDef, category string, opts ContractValidationOptions) []ContractFinding {
	required := []struct {
		field string
		count int
	}{
		{"requirements.input", len(def.Requirements.Input)},
		{"requirements.output", len(def.Requirements.Output)},
		{"requirements.errors", len(def.Requirements.Errors)},
	}
	var findings []ContractFinding
	for _, req := range required {
		if req.count > 0 {
			continue
		}
		findings = append(findings, ContractFinding{
			ToolName:    def.Name,
			Field:       req.field,
			Severity:    severity(opts),
			Category:    category,
			Message:     fmt.Sprintf("tool %q is missing %s", def.Name, req.field),
			Remediation: fmt.Sprintf("add observable must-statements under %s", req.field),
		})
	}
	return findings
}

func missingOutputSchema(def ToolDef, category string, opts ContractValidationOptions) ContractFinding {
	if len(def.Output.Schema) > 0 {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    def.Name,
		Field:       "output.schema",
		Severity:    severity(opts),
		Category:    category,
		Message:     fmt.Sprintf("tool %q is missing output.schema", def.Name),
		Remediation: "add a JSON Schema-compatible output.schema describing machine-readable output",
	}
}

func missingReversibility(def ToolDef, category string, opts ContractValidationOptions) ContractFinding {
	if def.Reversibility.Classification != "" {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    def.Name,
		Field:       "reversibility.classification",
		Severity:    severity(opts),
		Category:    category,
		Message:     fmt.Sprintf("tool %q is missing reversibility.classification", def.Name),
		Remediation: "classify reversibility as reversible, compensatable, or irreversible",
	}
}

func missingRelationships(def ToolDef, category string, opts ContractValidationOptions) ContractFinding {
	if len(def.Relationships.Before) > 0 || len(def.Relationships.After) > 0 || len(def.Relationships.Overlaps) > 0 {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    def.Name,
		Field:       "relationships",
		Severity:    ContractSeverityInfo,
		Category:    category,
		Message:     fmt.Sprintf("tool %q does not document relationships", def.Name),
		Remediation: "add before, after, or overlaps guidance when this tool has common neighbors or similar tools",
	}
}

func appendIfIncluded(findings []ContractFinding, finding ContractFinding, opts ...ContractValidationOptions) []ContractFinding {
	if finding.ToolName == "" {
		return findings
	}
	var opt ContractValidationOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if !severityAtLeast(finding.Severity, opt.MinimumLevel) {
		return findings
	}
	return append(findings, finding)
}

func severityAtLeast(severity, minimum string) bool {
	if minimum == "" {
		return true
	}
	return severityRank(severity) >= severityRank(minimum)
}

func severityRank(severity string) int {
	switch severity {
	case ContractSeverityError:
		return 3
	case ContractSeverityWarning:
		return 2
	case ContractSeverityInfo:
		return 1
	default:
		return 0
	}
}
