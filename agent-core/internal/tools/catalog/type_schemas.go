// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

// ResolveToolSchemas expands every $type reference in a tool's output schema
// and parameters, so ToToolSpec, schema compatibility, and every other reader
// downstream sees a plain schema and needs no knowledge of the type registry
// (srd051 R3.3, R4.1).
// It also reports, per tool name, the types that tool referenced, so closure
// usedness can credit the import that supplied them.
func ResolveToolSchemas(
	defs []ToolDef, registry *typesys.Registry,
) ([]ToolDef, map[string][]string, error) {
	resolved := make([]ToolDef, 0, len(defs))
	referenced := map[string][]string{}
	for _, def := range defs {
		expanded, refs, err := resolveToolSchemas(def, registry)
		if err != nil {
			return nil, nil, err
		}
		if len(refs) > 0 {
			referenced[def.Name] = refs
		}
		resolved = append(resolved, expanded)
	}
	return resolved, referenced, nil
}

func resolveToolSchemas(def ToolDef, registry *typesys.Registry) (ToolDef, []string, error) {
	def, signatureRefs, err := applySignatureTypes(def, registry)
	if err != nil {
		return ToolDef{}, nil, err
	}
	output, outputRefs, err := registry.ResolveSchemaRefs(def.Output.Schema)
	if err != nil {
		return ToolDef{}, nil, fmt.Errorf("tool %q output schema: %w", def.Name, err)
	}
	parameters, paramRefs, err := registry.ResolveSchemaRefs(def.Parameters)
	if err != nil {
		return ToolDef{}, nil, fmt.Errorf("tool %q parameters: %w", def.Name, err)
	}
	if output != nil {
		def.Output.Schema = output
	}
	if parameters != nil {
		def.Parameters = parameters
	}
	return def, append(append(signatureRefs, outputRefs...), paramRefs...), nil
}

// applySignatureTypes turns a signature's input and output type references into
// the schemas the legacy fields carry, so ToToolSpec and every other reader
// sees one shape whichever form the author used (srd051 R6.3, R6.4). The
// references resolve under the ordinary rules, so an unknown one fails with
// R4.2's error rather than one of its own (R6.5).
func applySignatureTypes(def ToolDef, registry *typesys.Registry) (ToolDef, []string, error) {
	if def.Signature == nil {
		return def, nil, nil
	}
	def = applyContractDefaults(def)
	var refs []string
	if ref := def.Signature.Output; ref != "" {
		schema, used, err := registry.ResolveSchemaRefs(map[string]any{typesys.TypeRefKey: ref})
		if err != nil {
			return ToolDef{}, nil, fmt.Errorf("tool %q signature output: %w", def.Name, err)
		}
		def.Output.Schema = schema
		refs = append(refs, used...)
	}
	if ref := def.Signature.Input; ref != "" {
		schema, used, err := registry.ResolveSchemaRefs(map[string]any{typesys.TypeRefKey: ref})
		if err != nil {
			return ToolDef{}, nil, fmt.Errorf("tool %q signature input: %w", def.Name, err)
		}
		def.Parameters = schema
		refs = append(refs, used...)
	}
	return def, refs, nil
}
