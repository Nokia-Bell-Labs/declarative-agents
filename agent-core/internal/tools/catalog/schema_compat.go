// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"fmt"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// ValidateResultSchemaCompatibility checks deterministic action data flow.
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
		findings = append(findings, checkActionSuccessors(from, defsByName, nextByInput, tr, opts)...)
	}
	return findings
}

func checkActionSuccessors(from ToolDef, defsByName map[string]ToolDef, nextByInput map[core.TransitionInput]core.TransitionSpec, tr core.TransitionSpec, opts ContractValidationOptions) []ContractFinding {
	var findings []ContractFinding
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
		if to, ok := defsByName[next.Action]; ok {
			findings = append(findings, compareResultToParameters(from, to, emit, opts)...)
		}
	}
	return findings
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
	return compareObjectSchemas(from, to, emit, field, opts)
}

func compareObjectSchemas(from, to ToolDef, emit, field string, opts ContractValidationOptions) []ContractFinding {
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
	return compareRequiredFields(from, to, emit, field, opts)
}

func compareRequiredFields(from, to ToolDef, emit, field string, opts ContractValidationOptions) []ContractFinding {
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
