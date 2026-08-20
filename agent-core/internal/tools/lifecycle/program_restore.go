// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package lifecycle

import (
	"fmt"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

// ProgramResources contains the declarations needed to rebuild the originating
// rollback registry.
type ProgramResources struct {
	Definitions     []catalog.ToolDef
	RestDefinitions toolrest.Collection
}

// ReferencedProgram is a loaded program plus its freshly calculated identity.
type ReferencedProgram struct {
	ProgramResources
	Ref core.ProgramRef
}

// ProgramLoader loads the current assets for a persisted program reference.
type ProgramLoader func(core.ProgramRef) (ReferencedProgram, error)

// ReconstructProgramResources verifies and merges the originating program
// resources needed by a fresh checkpoint_rollback profile (srd026 R3.5, R3.12).
func ReconstructProgramResources(
	ref core.ProgramRef,
	execution core.Execution,
	current ProgramResources,
	load ProgramLoader,
) (ProgramResources, error) {
	if ref.Profile == "" || ref.Digest == "" {
		return ProgramResources{}, fmt.Errorf("target checkpoint has no declarative program reference")
	}
	if load == nil {
		return ProgramResources{}, fmt.Errorf("target program loader is unavailable")
	}
	target, err := load(ref)
	if err != nil {
		return ProgramResources{}, err
	}
	if target.Ref.Digest != ref.Digest {
		return ProgramResources{}, fmt.Errorf(
			"target program %s changed: checkpoint digest %s, current digest %s",
			ref.Profile, ref.Digest, target.Ref.Digest,
		)
	}
	target.Definitions, err = definitionsForExecution(target.Definitions, execution)
	if err != nil {
		return ProgramResources{}, err
	}
	return ProgramResources{
		Definitions:     catalog.MergeToolDefs(target.Definitions, current.Definitions),
		RestDefinitions: mergeRESTCollections(target.RestDefinitions, current.RestDefinitions),
	}, nil
}

func definitionsForExecution(
	defs []catalog.ToolDef,
	execution core.Execution,
) ([]catalog.ToolDef, error) {
	needed := make(map[string]bool)
	for _, entry := range execution {
		needed[entry.CommandName] = true
	}
	found := make(map[string]bool)
	filtered := make([]catalog.ToolDef, 0, len(needed))
	for _, def := range defs {
		if needed[def.Name] {
			filtered = append(filtered, def)
			found[def.Name] = true
		}
	}
	for _, entry := range execution {
		if entry.Receipt != "" && !found[entry.CommandName] {
			return nil, fmt.Errorf(
				"target program does not declare receipt-bearing command %q",
				entry.CommandName,
			)
		}
	}
	return filtered, nil
}

func mergeRESTCollections(base, override toolrest.Collection) toolrest.Collection {
	return toolrest.Collection{
		Clients:          mergeMap(base.Clients, override.Clients),
		Servers:          mergeMap(base.Servers, override.Servers),
		Auth:             mergeMap(base.Auth, override.Auth),
		Limits:           mergeMap(base.Limits, override.Limits),
		RetryPolicies:    mergeMap(base.RetryPolicies, override.RetryPolicies),
		ResponseMappings: mergeMap(base.ResponseMappings, override.ResponseMappings),
	}
}

func mergeMap[K comparable, V any](base, override map[K]V) map[K]V {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	merged := make(map[K]V, len(base)+len(override))
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}
