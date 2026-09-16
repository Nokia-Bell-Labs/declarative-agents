// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

// Load-time checking of selector paths against the type of the label they read
// (srd038). A label's type is the output type of the action that publishes it,
// so a signature on a tool gives every label it labels a shape, and a path
// naming a field that shape does not have is decidable before the run.
//
// Typing is gradual throughout: an action with no signature publishes an
// untyped label, and paths under an untyped label are not checked.

// LabelTypes derives the resolved schema behind every label a machine
// publishes. A label with no derivable type is absent from the result rather
// than present and empty, so a caller cannot confuse untyped with empty.
func LabelTypes(
	spec core.MachineSpec, defs []ToolDef, registry *typesys.Registry,
	operations RESTOperations,
) map[string]map[string]any {
	byName := make(map[string]ToolDef, len(defs))
	for _, def := range defs {
		byName[def.Name] = def
	}
	types := map[string]map[string]any{}
	for _, label := range spec.ExternalLabels {
		if schema := resolvedTypeRef(label.Type, registry); schema != nil {
			types[label.Name] = schema
		}
	}
	for _, transition := range spec.Transitions {
		addTransitionLabelTypes(types, transition, byName, registry, operations)
	}
	return types
}

func addTransitionLabelTypes(
	types map[string]map[string]any,
	transition core.TransitionSpec,
	byName map[string]ToolDef,
	registry *typesys.Registry,
	operations RESTOperations,
) {
	action, ok := byName[transition.Action]
	if !ok {
		return
	}
	output := actionOutputSchema(action, operations)
	if len(output) == 0 {
		return
	}
	if transition.Label != "" {
		types[transition.Label] = output
	}
	if transition.Action != "" {
		types[transition.Action] = output
	}
	if transition.ForEach == nil {
		return
	}
	// An item label takes the element type of the array it iterates, which is
	// the action's own output only when that output is the array.
	if items, isArray := arrayItems(output); isArray && transition.ForEach.As != "" {
		types[transition.ForEach.As] = items
	}
	if transition.ForEach.Join.Label != "" {
		types[transition.ForEach.Join.Label] = joinEnvelope(output)
	}
}

// joinEnvelope is the shape a for_each join publishes, mirroring
// iteratorJoinResult in internal/runtime/core: an object carrying one outcome
// per dispatched item plus the aggregate counts and the policy that produced
// them. Synthesized here because the runtime builds it; a machine never
// declares a type for it.
//
// The per-item result is the digest the join projects, with the iterated
// action's own output under result.structured_output.
func joinEnvelope(output map[string]any) map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"succeeded": map[string]any{"type": "integer"},
			"failed":    map[string]any{"type": "integer"},
			"policy":    map[string]any{"type": "string"},
			"items": map[string]any{
				"type": "array",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"index":        map[string]any{"type": "integer"},
						"input":        map[string]any{},
						"command_name": map[string]any{"type": "string"},
						"result": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"signal":            map[string]any{"type": "string"},
								"output":            map[string]any{"type": "string"},
								"error":             map[string]any{"type": "string"},
								"cost":              map[string]any{},
								"redaction_version": map[string]any{"type": "integer"},
								"redacted_paths":    map[string]any{"type": "array", "items": map[string]any{}},
								"redaction_status":  map[string]any{"type": "string"},
								"structured_output": output,
							},
						},
					},
				},
			},
		},
	}
}

// actionOutputSchema returns the type a label takes from its publishing
// action: the envelope a REST client word publishes, derived from the
// operation it names, or the type its signature states.
//
// The REST case is derived rather than declared because the runtime builds
// that envelope from the same operation, so a signature restating it could
// disagree with what the word returns (GH-2064).
func actionOutputSchema(def ToolDef, operations RESTOperations) map[string]any {
	if schema, ok := restClientLabelSchema(def, operations); ok {
		return schema
	}
	return signatureOutputSchema(def)
}

// signatureOutputSchema returns the type a label takes from its publishing
// action, and only for an action that declares a signature.
//
// A legacy output.schema does not serve: it describes the payload a tool
// produces, not the output a selector reads. A REST client word declares the
// shape of its mapped body while the runtime publishes that inside an envelope
// carrying mapped, body, headers, and status, so typing a label from the
// declared schema rejects $from(L).mapped.pods, which is correct authoring.
// A signature is authored knowing it types the label, so only a signature
// types one, and everything else stays untyped (srd051 R6, gradual).
func signatureOutputSchema(def ToolDef) map[string]any {
	if def.Signature == nil || def.Signature.Output == "" {
		return nil
	}
	return def.Output.Schema
}

func arrayItems(schema map[string]any) (map[string]any, bool) {
	if name, _ := schema["type"].(string); name != "array" {
		return nil, false
	}
	items, ok := schema["items"].(map[string]any)
	return items, ok
}

func resolvedTypeRef(ref string, registry *typesys.Registry) map[string]any {
	if ref == "" || registry == nil {
		return nil
	}
	schema, err := registry.ResolveSchema(map[string]any{typesys.TypeRefKey: ref})
	if err != nil {
		return nil
	}
	return schema
}

// ValidateSelectorPaths reports every selector whose path the label's type does
// not have. Selectors into untyped labels are skipped, so a machine converts to
// signatures one tool at a time.
func ValidateSelectorPaths(
	spec core.MachineSpec, defs []ToolDef, registry *typesys.Registry,
	operations RESTOperations,
) []core.MachineDiagnostic {
	types := LabelTypes(spec, defs, registry, operations)
	var diagnostics []core.MachineDiagnostic
	seen := map[string]struct{}{}
	for i, transition := range spec.Transitions {
		if transition.ForEach == nil {
			continue
		}
		selector := transition.ForEach.Items
		if _, repeated := seen[selector]; repeated {
			continue
		}
		seen[selector] = struct{}{}
		diagnostics = append(diagnostics,
			pathDiagnostics(types, selector, "", transition.State, i)...)
	}
	for _, def := range reachableToolDefs(spec, defs) {
		for _, ref := range def.SelectorRefs() {
			diagnostics = append(diagnostics, pathDiagnostics(types, ref, def.Name, "", -1)...)
		}
	}
	return diagnostics
}

func pathDiagnostics(
	types map[string]map[string]any,
	selector, tool, state string,
	transitionIndex int,
) []core.MachineDiagnostic {
	parsed, ok := core.ParseSelector(selector)
	if !ok || parsed.Label == "" {
		return nil
	}
	schema, typed := types[parsed.Label]
	if !typed {
		return nil
	}
	err := typesys.CheckPath(schema, parsed.Path)
	if err == nil {
		return nil
	}
	where := "machine"
	if tool != "" {
		where = fmt.Sprintf("tool %q", tool)
	}
	return []core.MachineDiagnostic{{
		Severity: core.MachineDiagnosticWarning,
		Code:     core.DiagnosticSelectorPathMismatch,
		Message: fmt.Sprintf("%s selector %q does not resolve against the type of label %q: %v",
			where, selector, parsed.Label, err),
		State:           state,
		Tool:            tool,
		TransitionIndex: transitionIndex,
	}}
}

// ValidateSelectorPathsStrict is the load-time gate, reported as one error
// naming every mismatch.
func ValidateSelectorPathsStrict(
	spec core.MachineSpec, defs []ToolDef, registry *typesys.Registry,
	operations RESTOperations,
) error {
	diagnostics := ValidateSelectorPaths(spec, defs, registry, operations)
	if len(diagnostics) == 0 {
		return nil
	}
	messages := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		messages = append(messages, diagnostic.Message)
	}
	return fmt.Errorf("selector path mismatches: %s", joinMessages(messages))
}

func joinMessages(messages []string) string {
	joined := messages[0]
	for _, message := range messages[1:] {
		joined += "; " + message
	}
	return joined
}
