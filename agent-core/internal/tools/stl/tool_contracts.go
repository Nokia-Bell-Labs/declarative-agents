// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
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

// ValidateResultSchemaCompatibility prototypes static data-flow checks between
// deterministic machine actions. When a named action emits a signal that selects
// another named action, the first tool's output.schema is compared with the next
// tool's parameters schema. The prototype intentionally skips dynamic dispatch
// and prose-only outputs because those paths need richer machine annotations.
func ValidateResultSchemaCompatibility(spec core.MachineSpec, defs []ToolDef, opts ContractValidationOptions) []ContractFinding {
	defsByName := make(map[string]ToolDef, len(defs))
	for _, def := range defs {
		defsByName[def.Name] = def
	}
	nextByInput := make(map[core.TransitionInput]core.TransitionSpec, len(spec.Transitions))
	for _, tr := range spec.Transitions {
		nextByInput[core.TransitionInput{State: core.State(tr.State), Signal: core.Signal(tr.Signal)}] = tr
	}

	var findings []ContractFinding
	for _, tr := range spec.Transitions {
		if tr.Action == "" || tr.Action == "$tool" {
			continue
		}
		from, ok := defsByName[tr.Action]
		if !ok {
			continue
		}
		for _, emit := range from.Emits {
			next, ok := nextByInput[core.TransitionInput{State: core.State(tr.Next), Signal: core.Signal(emit)}]
			if !ok || next.Action == "" {
				continue
			}
			if next.Action == "$tool" {
				findings = appendIfIncluded(findings, compatibilityFinding(from.Name, "$tool", ContractSeverityInfo, "dynamic_dispatch",
					fmt.Sprintf("tool %q emits %q into dynamic $tool dispatch at %s/%s; static result-to-parameter compatibility is not checked", from.Name, emit, next.State, next.Signal),
					"dynamic LLM-selected dispatch requires runtime ToolRequest validation and cannot be proven from one successor schema"), opts)
				continue
			}
			to, ok := defsByName[next.Action]
			if !ok {
				continue
			}
			findings = append(findings, compareResultToParameters(from, to, emit, opts)...)
		}
	}
	return findings
}

const (
	ContractAuditComplete = "complete"
	ContractAuditPartial  = "partial"
	ContractAuditMissing  = "missing"
)

// ContractAuditEntry summarizes Grammar Machine contract coverage for one tool.
type ContractAuditEntry struct {
	ToolName        string
	Category        string
	Status          string
	MissingFields   []string
	MigrationTarget string
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
		effective := effectiveContractOptions(def, opts)

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

		if def.SideEffects.LegacyText != "" {
			findings = appendIfIncluded(findings, ContractFinding{
				ToolName:    def.Name,
				Field:       "side_effects",
				Severity:    severity(effective),
				Category:    category,
				Message:     fmt.Sprintf("tool %q uses legacy scalar side_effects", def.Name),
				Remediation: "replace scalar side_effects with structured entries declaring kind, target, paths, state, and description",
			}, opts)
		}
		if effective.RequireStructuredSideEffects && len(def.SideEffects.Items) == 0 {
			findings = appendIfIncluded(findings, ContractFinding{
				ToolName:    def.Name,
				Field:       "side_effects",
				Severity:    severity(effective),
				Category:    category,
				Message:     fmt.Sprintf("tool %q does not declare structured side_effects", def.Name),
				Remediation: "add side_effects as a structured list; use kind: none for read-only tools",
			}, opts)
		}
	}
	return findings
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

// AuditToolContracts classifies each tool as complete, partial, or missing for
// the core Grammar Machine word contract fields. It is meant for migration
// planning; enforcement belongs in ValidateToolContracts strict modes.
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

func compareResultToParameters(from, to ToolDef, emit string, opts ContractValidationOptions) []ContractFinding {
	field := fmt.Sprintf("output.schema->%s.parameters", to.Name)
	if len(from.Output.Schema) == 0 {
		return appendIfIncluded(nil, compatibilityFinding(from.Name, to.Name, ContractSeverityInfo, "missing_output_schema",
			fmt.Sprintf("tool %q emits %q into %q but has no output.schema to compare", from.Name, emit, to.Name),
			"add output.schema for machine-readable results or document this edge as prose-only"), opts)
	}
	if len(to.Parameters) == 0 {
		return appendIfIncluded(nil, compatibilityFinding(from.Name, to.Name, ContractSeverityInfo, "missing_parameter_schema",
			fmt.Sprintf("tool %q emits %q into %q but %q has no parameters schema to compare", from.Name, emit, to.Name, to.Name),
			"add parameters schema for deterministic consumers that parse Result.Output"), opts)
	}

	fromType := schemaType(from.Output.Schema)
	toType := schemaType(to.Parameters)
	if fromType != "" && toType != "" && fromType != toType {
		return appendIfIncluded(nil, compatibilityFinding(from.Name, to.Name, severity(opts), field,
			fmt.Sprintf("tool %q output.schema type %q is incompatible with %q parameters type %q on emitted signal %q",
				from.Name, fromType, to.Name, toType, emit),
			"align the producer output.schema with the consumer parameters schema or insert an adapter word"), opts)
	}
	if fromType != "object" || toType != "object" {
		return appendIfIncluded(nil, compatibilityFinding(from.Name, to.Name, ContractSeverityInfo, "non_object_schema",
			fmt.Sprintf("tool %q emits %q into %q with non-object schema types output=%q parameters=%q; only simple object-field compatibility is checked",
				from.Name, emit, to.Name, fromType, toType),
			"document prose/scalar Result.Output limitations or model the edge with object schemas"), opts)
	}

	var findings []ContractFinding
	fromProps := schemaProperties(from.Output.Schema)
	toProps := schemaProperties(to.Parameters)
	for _, name := range schemaRequired(to.Parameters) {
		fromProp, ok := fromProps[name]
		if !ok {
			findings = appendIfIncluded(findings, compatibilityFinding(from.Name, to.Name, severity(opts), field,
				fmt.Sprintf("tool %q output.schema does not provide required field %q for %q parameters on emitted signal %q",
					from.Name, name, to.Name, emit),
				"add the field to the producer output.schema, remove it from the consumer required list, or insert an adapter word"), opts)
			continue
		}
		toProp := toProps[name]
		fromPropType := schemaType(fromProp)
		toPropType := schemaType(toProp)
		if fromPropType != "" && toPropType != "" && fromPropType != toPropType {
			findings = appendIfIncluded(findings, compatibilityFinding(from.Name, to.Name, severity(opts), field,
				fmt.Sprintf("tool %q output field %q type %q does not match %q parameter type %q on emitted signal %q",
					from.Name, name, fromPropType, to.Name, toPropType, emit),
				"align the field types or insert an adapter word"), opts)
		}
	}
	return findings
}

func compatibilityFinding(fromTool, toTool, sev, field, message, remediation string) ContractFinding {
	return ContractFinding{
		ToolName:    fromTool,
		Field:       field,
		Severity:    sev,
		Category:    "schema_compatibility",
		Message:     message,
		Remediation: remediationForPair(toTool, remediation),
	}
}

func remediationForPair(toTool, remediation string) string {
	if toTool == "" {
		return remediation
	}
	return fmt.Sprintf("%s; consumer=%s", remediation, toTool)
}

func schemaType(schema map[string]interface{}) string {
	if value, ok := schema["type"].(string); ok {
		return value
	}
	return ""
}

func schemaRequired(schema map[string]interface{}) []string {
	raw, ok := schema["required"]
	if !ok {
		return nil
	}
	switch values := raw.(type) {
	case []string:
		return append([]string(nil), values...)
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if s, ok := value.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func schemaProperties(schema map[string]interface{}) map[string]map[string]interface{} {
	raw, ok := schema["properties"]
	if !ok {
		return nil
	}
	props := make(map[string]map[string]interface{})
	switch values := raw.(type) {
	case map[string]interface{}:
		for name, value := range values {
			if prop, ok := value.(map[string]interface{}); ok {
				props[name] = prop
			}
		}
	case map[string]map[string]interface{}:
		for name, value := range values {
			props[name] = value
		}
	}
	return props
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
