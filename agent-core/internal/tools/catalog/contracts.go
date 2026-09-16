// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

// Contract findings for the catalog's receipt contract check
// (receipt_contract.go). Contract completeness itself is checked once, by the
// corpus audit in pkg/spec (srd051 R6); the authoring-time mirror that lived
// here had no caller and drifted from it (GH-2023, GH-2071). The
// result-to-parameter schema check went the same way: its producer-output to
// consumer-parameters comparison matched no edge shape the corpus uses, since
// a word's parameters arrive through $tool dispatch or $from selectors that
// typed labels already check (GH-2087). Severity filtering and strict mode left
// with it; every remaining finding is an error.

// ContractSeverityError marks a contract violation.
const ContractSeverityError = "error"

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
