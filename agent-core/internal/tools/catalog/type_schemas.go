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
	return def, append(outputRefs, paramRefs...), nil
}
