// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"path/filepath"

	internalload "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/load"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toollifecycle "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/lifecycle"
)

func augmentRollbackResources(resources runResources, checkpoint core.Checkpoint) (runResources, error) {
	if !selectsRollbackTool(resources.Definitions) {
		return resources, nil
	}
	position, execution, err := checkpoint.Load()
	if err != nil {
		return runResources{}, fmt.Errorf("load target checkpoint program: %w", err)
	}
	restored, err := toollifecycle.ReconstructProgramResources(
		position.Snapshot.Program,
		execution,
		toollifecycle.ProgramResources{
			Definitions:     resources.Definitions,
			RestDefinitions: resources.RestDefinitions,
		},
		func(ref core.ProgramRef) (toollifecycle.ReferencedProgram, error) {
			return loadReferencedProgram(ref, resources.Config)
		},
	)
	if err != nil {
		return runResources{}, err
	}
	resources.Definitions = restored.Definitions
	resources.RestDefinitions = restored.RestDefinitions
	return resources, nil
}

func selectsRollbackTool(defs []catalog.ToolDef) bool {
	for _, def := range defs {
		if def.Name == toollifecycle.InitCheckpointRollback || def.Init == toollifecycle.InitCheckpointRollback {
			return true
		}
	}
	return false
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func buildProgramRef(cfg runtimeConfig) (core.ProgramRef, error) {
	return catalog.BuildProgramRef(catalogProgramPaths(cfg))
}

func buildClosureProgramRef(closure *internalload.Closure) (core.ProgramRef, error) {
	return catalog.BuildProgramRefFromAssets(closure.ProfilePath, closure.Assets), nil
}

func catalogProgramPaths(cfg runtimeConfig) catalog.ProgramPaths {
	return catalog.ProgramPaths{
		Profile:          cfg.Profile,
		Machine:          cfg.Machine,
		ToolSelections:   cfg.Tools,
		ToolDeclarations: cfg.ToolDeclarations,
		ToolConfigDirs:   cfg.ToolConfigDirs,
		RESTDefinitions:  cfg.RestDefinitions,
		RESTConfigDirs:   cfg.RestConfigDirs,
	}
}

func loadReferencedProgram(
	ref core.ProgramRef,
	_ runtimeConfig,
) (toollifecycle.ReferencedProgram, error) {
	closure, err := internalload.LoadClosure(ref.Profile, internalload.Options{})
	if err != nil {
		return toollifecycle.ReferencedProgram{}, err
	}
	actual, err := buildClosureProgramRef(closure)
	if err != nil {
		return toollifecycle.ReferencedProgram{}, err
	}
	return toollifecycle.ReferencedProgram{
		ProgramResources: toollifecycle.ProgramResources{
			Definitions: closure.Selected, RestDefinitions: closure.Rest,
		},
		Ref: actual,
	}, nil
}

func runtimeConfigForProfile(path string, runtime runtimeConfig) (runtimeConfig, error) {
	profile, err := catalog.LoadProfile(path)
	if err != nil {
		return runtimeConfig{}, fmt.Errorf("load target profile: %w", err)
	}
	cfg := runtime
	cfg.Profile = canonicalPath(path)
	cfg.Machine = profile.Machine
	cfg.Tools = append([]string(nil), profile.Tools...)
	cfg.ToolDeclarations = append([]string(nil), profile.ToolDeclarations...)
	cfg.ToolConfigDirs = append([]string(nil), profile.ToolConfigDirs...)
	cfg.RestDefinitions = append([]string(nil), profile.RestDefinitions...)
	cfg.RestConfigDirs = append([]string(nil), profile.RestConfigDirs...)
	return cfg, nil
}
