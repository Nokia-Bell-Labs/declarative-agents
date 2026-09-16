// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

// Contract findings shared by the catalog's remaining checks: receipt contracts
// (receipt_contract.go) and result-to-parameter schema compatibility
// (schema_compat.go). Contract completeness itself is checked once, by the
// corpus audit in pkg/spec (srd051 R6); the authoring-time mirror that lived
// here had no caller and drifted from it (GH-2023, GH-2071).

const (
	ContractSeverityInfo    = "info"
	ContractSeverityWarning = "warning"
	ContractSeverityError   = "error"
)

// ContractValidationOptions tunes how a check reports: Strict raises what it
// finds to an error, and MinimumLevel drops findings below that severity.
type ContractValidationOptions struct {
	Strict       bool
	MinimumLevel string
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

func contractCategory(def ToolDef) string {
	if def.Category != "" {
		return def.Category
	}
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
