// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"fmt"
	"strings"
)

const (
	ContractSeverityInfo    = "info"
	ContractSeverityWarning = "warning"
	ContractSeverityError   = "error"
)

const (
	ToolContractStrict   = "strict"
	ToolContractMigrated = "migrated"
	ToolContractLegacy   = "legacy"
)

const (
	ContractAuditComplete = "complete"
	ContractAuditPartial  = "partial"
	ContractAuditMissing  = "missing"
)

// ContractValidationOptions controls contract validation strictness.
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

// ContractAuditEntry summarizes contract coverage for one tool.
type ContractAuditEntry struct {
	ToolName        string
	Category        string
	Status          string
	MissingFields   []string
	MigrationTarget string
}

// ValidateToolContracts reports missing or legacy contract metadata for loaded
// runtime ToolDef declarations. Public spec-corpus validation mirrors only the
// fields it needs in pkg/spec so pkg/spec does not import internal packages.
func ValidateToolContracts(defs []ToolDef, opts ContractValidationOptions) []ContractFinding {
	var findings []ContractFinding
	for _, def := range defs {
		category := contractCategory(def)
		if category == "internal" && !opts.IncludeInternal {
			continue
		}
		effective := effectiveContractOptions(def, opts)
		findings = append(findings, requiredContractFindings(def, category, effective, opts)...)
		findings = appendIfIncluded(findings, legacySideEffectsFinding(def, category, effective), opts)
		findings = appendIfIncluded(findings, structuredSideEffectsFinding(def, category, effective), opts)
	}
	return findings
}

// ReviewToolAuthoring runs authoring-time checks that decide whether a new
// ToolDef is a composable vocabulary word or should be split/reclassified before
// it is selected by a machine.
func ReviewToolAuthoring(defs []ToolDef, opts ContractValidationOptions) []ContractFinding {
	findings := ValidateToolContracts(defs, opts)
	for _, def := range defs {
		category := contractCategory(def)
		if category == "internal" && !opts.IncludeInternal {
			continue
		}
		effective := effectiveContractOptions(def, opts)
		findings = appendIfIncluded(findings, workflowShapeFinding(def, category, effective), opts)
		findings = appendIfIncluded(findings, irreversibleConfirmationFinding(def, category, effective), opts)
		findings = appendIfIncluded(findings, overlappingRelationshipsFinding(def, category), opts)
	}
	return findings
}

func requiredContractFindings(def ToolDef, category string, effective, opts ContractValidationOptions) []ContractFinding {
	var findings []ContractFinding
	findings = appendIfIncluded(findings, missingString(def.Name, category, "problem", def.Problem, effective), opts)
	findings = appendIfIncluded(findings, missingList(def.Name, category, "goals", len(def.Goals), effective), opts)
	for _, finding := range missingRequirements(def, category, effective) {
		findings = appendIfIncluded(findings, finding, opts)
	}
	findings = appendIfIncluded(findings, missingList(def.Name, category, "non_goals", len(def.NonGoals), effective), opts)
	findings = appendIfIncluded(findings, missingList(def.Name, category, "emits", len(def.Emits), effective), opts)
	findings = appendIfIncluded(findings, missingOutputSchema(def, category, effective), opts)
	findings = appendIfIncluded(findings, missingReversibility(def, category, effective), opts)
	findings = appendIfIncluded(findings, missingUndo(def, category, effective), opts)
	findings = appendIfIncluded(findings, missingRelationships(def, category, effective), opts)
	return findings
}

func legacySideEffectsFinding(def ToolDef, category string, opts ContractValidationOptions) ContractFinding {
	if def.SideEffects.LegacyText == "" {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    def.Name,
		Field:       "side_effects",
		Severity:    severity(opts),
		Category:    category,
		Message:     fmt.Sprintf("tool %q uses legacy scalar side_effects", def.Name),
		Remediation: "replace scalar side_effects with structured entries declaring kind, target, paths, state, and description",
	}
}

func structuredSideEffectsFinding(def ToolDef, category string, opts ContractValidationOptions) ContractFinding {
	if !opts.RequireStructuredSideEffects || len(def.SideEffects.Items) > 0 {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    def.Name,
		Field:       "side_effects",
		Severity:    severity(opts),
		Category:    category,
		Message:     fmt.Sprintf("tool %q does not declare structured side_effects", def.Name),
		Remediation: "add side_effects as a structured list; use kind: none for read-only tools",
	}
}

func workflowShapeFinding(def ToolDef, category string, opts ContractValidationOptions) ContractFinding {
	if category != "word" && category != "stateful_internal" {
		return ContractFinding{}
	}
	if !looksWorkflowShaped(def.Description) && !looksWorkflowShaped(def.Problem) {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    def.Name,
		Field:       "description",
		Severity:    severity(opts),
		Category:    category,
		Message:     fmt.Sprintf("tool %q appears to describe multiple observable verbs", def.Name),
		Remediation: "split workflow-shaped behavior into separate words or classify the tool as an explicit boundary",
	}
}

func irreversibleConfirmationFinding(def ToolDef, category string, opts ContractValidationOptions) ContractFinding {
	if category != "boundary" || def.Reversibility.Classification != "irreversible" || def.Reversibility.RequiresConfirmation {
		return ContractFinding{}
	}
	if !hasExternalOrUserEffect(def) {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    def.Name,
		Field:       "reversibility.requires_confirmation",
		Severity:    severity(opts),
		Category:    category,
		Message:     fmt.Sprintf("irreversible boundary tool %q affects external or user state without confirmation", def.Name),
		Remediation: "set reversibility.requires_confirmation or redesign the tool with a compensating action",
	}
}

func overlappingRelationshipsFinding(def ToolDef, category string) ContractFinding {
	if len(def.Relationships.Overlaps) == 0 {
		return ContractFinding{}
	}
	if len(def.Relationships.Before) > 0 && len(def.Relationships.After) > 0 && overlapsExplainDifferences(def.Relationships.Overlaps) {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    def.Name,
		Field:       "relationships",
		Severity:    ContractSeverityInfo,
		Category:    category,
		Message:     fmt.Sprintf("tool %q overlaps existing vocabulary without complete relationship guidance", def.Name),
		Remediation: "add before and after neighbors and explain each relationships.overlaps difference",
	}
}

func effectiveContractOptions(def ToolDef, opts ContractValidationOptions) ContractValidationOptions {
	effective := opts
	switch def.Contract {
	case ToolContractStrict, ToolContractMigrated:
		effective.Strict = true
		effective.RequireStructuredSideEffects = true
	case ToolContractLegacy:
		effective.Strict = false
	}
	return effective
}

// AuditToolContracts classifies contract coverage for migration planning.
func AuditToolContracts(defs []ToolDef, opts ContractValidationOptions) []ContractAuditEntry {
	audit := make([]ContractAuditEntry, 0, len(defs))
	for _, def := range defs {
		category := contractCategory(def)
		if category == "internal" && !opts.IncludeInternal {
			continue
		}
		missing := missingAuditFields(def, category)
		audit = append(audit, ContractAuditEntry{
			ToolName:        def.Name,
			Category:        category,
			Status:          contractAuditStatus(len(missing)),
			MissingFields:   missing,
			MigrationTarget: contractMigrationTarget(def, category, missing),
		})
	}
	return audit
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

func missingAuditFields(def ToolDef, category string) []string {
	checks := []struct {
		field   string
		present bool
	}{
		{"category", def.Category != ""},
		{"parameters", len(def.Parameters) > 0 || category == "internal"},
		{"emits", len(def.Emits) > 0},
		{"output.schema", len(def.Output.Schema) > 0},
		{"side_effects", len(def.SideEffects.Items) > 0 || def.SideEffects.LegacyText != ""},
		{"reversibility.classification", def.Reversibility.Classification != ""},
		{"undo", def.Undo.Strategy != "" || def.Reversibility.Undo != "" || len(def.Requirements.Undo) > 0},
		{"errors", len(def.Errors) > 0 || len(def.Requirements.Errors) > 0},
		{"relationships", len(def.Relationships.Before) > 0 || len(def.Relationships.After) > 0 || len(def.Relationships.Overlaps) > 0},
	}
	missing := make([]string, 0, len(checks))
	for _, check := range checks {
		if !check.present {
			missing = append(missing, check.field)
		}
	}
	return missing
}

func contractAuditStatus(missingCount int) string {
	switch {
	case missingCount == 0:
		return ContractAuditComplete
	case missingCount >= 7:
		return ContractAuditMissing
	default:
		return ContractAuditPartial
	}
}

func contractMigrationTarget(def ToolDef, category string, missing []string) string {
	if len(missing) == 0 {
		return ""
	}
	switch {
	case category == "boundary":
		return "declare boundary side effects, compensation/confirmation, and child or external ownership"
	case hasSideEffectKind(def, "state_mutation") || category == "stateful_internal":
		return "declare state_mutation effects and a command/domain undo strategy"
	case hasFilesystemEffect(def):
		return "declare filesystem effects and workspace_restore or compensating undo"
	case category == "internal":
		return "classify as word, stateful_internal, or boundary and add missing internal word metadata"
	default:
		return "fill missing word contract fields so static analysis can reason about this tool"
	}
}

func hasSideEffectKind(def ToolDef, kind string) bool {
	for _, effect := range def.SideEffects.Items {
		if effect.Kind == kind {
			return true
		}
	}
	return false
}

func hasFilesystemEffect(def ToolDef) bool {
	for _, effect := range def.SideEffects.Items {
		if effect.Kind == "filesystem_write" || effect.Kind == "filesystem_delete" || effect.Kind == "filesystem_read" {
			return true
		}
	}
	return false
}

func hasExternalOrUserEffect(def ToolDef) bool {
	for _, effect := range def.SideEffects.Items {
		text := strings.ToLower(strings.Join([]string{effect.Kind, effect.Target, effect.Description}, " "))
		if strings.Contains(text, "external") || strings.Contains(text, "user") || strings.Contains(text, "service") || strings.Contains(text, "registry") {
			return true
		}
	}
	return false
}

func looksWorkflowShaped(text string) bool {
	normalized := " " + strings.ToLower(text) + " "
	if strings.Count(normalized, " and ") >= 2 {
		return true
	}
	verbHits := 0
	for _, verb := range []string{" load ", " discover ", " initialize ", " create ", " copy ", " run ", " parse ", " collect ", " summarize ", " write "} {
		if strings.Contains(normalized, verb) {
			verbHits++
		}
	}
	return verbHits >= 3
}

func overlapsExplainDifferences(overlaps []ToolOverlap) bool {
	for _, overlap := range overlaps {
		if overlap.Tool == "" || strings.TrimSpace(overlap.Difference) == "" {
			return false
		}
	}
	return true
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

func missingUndo(def ToolDef, category string, opts ContractValidationOptions) ContractFinding {
	if def.Undo.Strategy != "" || def.Reversibility.Undo != "" || len(def.Requirements.Undo) > 0 {
		return ContractFinding{}
	}
	return ContractFinding{
		ToolName:    def.Name,
		Field:       "undo",
		Severity:    severity(opts),
		Category:    category,
		Message:     fmt.Sprintf("tool %q is missing undo strategy", def.Name),
		Remediation: "add undo.strategy or reversibility.undo describing noop, state restore, workspace restore, or compensation",
	}
}

func missingRelationships(def ToolDef, category string, _ ContractValidationOptions) ContractFinding {
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
