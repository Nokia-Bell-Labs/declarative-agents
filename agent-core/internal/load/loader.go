// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package load resolves one agent profile and its declaration closure.
package load

import (
	"fmt"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
)

// Options reserves caller-owned path configuration. The caller applies
// CoreRoot through the process-scoped core path mapper before loading.
type Options struct {
	CoreRoot      string
	ProfileLoaded func(string, catalog.AgentProfile) error
}

// Closure is the resolved configuration consumed by one agent start.
type Closure struct {
	ProfilePath  string
	Profile      catalog.AgentProfile
	ToolUniverse []catalog.ToolDef
	Selection    []string
	Selected     []catalog.ToolDef
	Rest         toolrest.Collection
	Machine      core.MachineSpec
	Files        []string
}

// LoadClosure loads and validates the complete declaration closure once.
func LoadClosure(profilePath string, options Options) (*Closure, error) {
	profilePath = canonicalPath(profilePath)
	profile, err := catalog.LoadProfile(profilePath)
	if err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	if options.ProfileLoaded != nil {
		if err := options.ProfileLoaded(profilePath, profile); err != nil {
			return nil, err
		}
	}

	var visited []string
	visit := func(path string, _ []byte) error {
		visited = append(visited, path)
		return nil
	}
	tools, err := loadTools(profile, visit)
	if err != nil {
		return nil, err
	}
	rest, err := toolrest.LoadDefinitionsWithVisitor(profile.RestDefinitions, profile.RestConfigDirs, visit)
	if err != nil {
		return nil, fmt.Errorf("load REST definitions: %w", err)
	}
	machine, err := core.LoadMachineSpec(profile.Machine)
	if err != nil {
		return nil, fmt.Errorf("load machine spec: %w", err)
	}
	files, err := programFiles(profilePath, profile, visited)
	if err != nil {
		return nil, err
	}
	return &Closure{
		ProfilePath: profilePath, Profile: profile,
		ToolUniverse: tools.universe, Selection: tools.selection, Selected: tools.selected,
		Rest: rest, Machine: machine, Files: files,
	}, nil
}

type loadedTools struct {
	universe  []catalog.ToolDef
	selection []string
	selected  []catalog.ToolDef
}

func loadTools(profile catalog.AgentProfile, visit catalog.FileVisitor) (loadedTools, error) {
	fromDirs, err := catalog.LoadToolDeclarationsFromDirsWithVisitor(profile.ToolConfigDirs, visit)
	if err != nil {
		return loadedTools{}, fmt.Errorf("load tool config dirs: %w", err)
	}
	explicit, err := catalog.LoadToolDeclarationsWithVisitor(profile.ToolDeclarations, visit)
	if err != nil {
		return loadedTools{}, fmt.Errorf("load tool declarations: %w", err)
	}
	universe := catalog.MergeToolDefs(fromDirs, explicit)
	selection, err := catalog.LoadToolSelectionsWithVisitor(profile.Tools, visit)
	if err != nil {
		return loadedTools{}, fmt.Errorf("load tool selection: %w", err)
	}
	selected, err := catalog.SelectTools(universe, selection)
	if err != nil {
		return loadedTools{}, fmt.Errorf("select tools: %w", err)
	}
	return loadedTools{universe: universe, selection: selection, selected: selected}, nil
}

func programFiles(
	profilePath string, profile catalog.AgentProfile, visited []string,
) ([]string, error) {
	paths := catalog.ProgramPaths{
		Profile:          profilePath,
		Machine:          profile.Machine,
		ToolSelections:   profile.Tools,
		ToolDeclarations: profile.ToolDeclarations,
		ToolConfigDirs:   profile.ToolConfigDirs,
		RESTDefinitions:  profile.RestDefinitions,
		RESTConfigDirs:   profile.RestConfigDirs,
	}
	files, err := catalog.ProgramAssetFilesFromVisited(paths, visited)
	if err != nil {
		return nil, fmt.Errorf("resolve program assets: %w", err)
	}
	return files, nil
}

func canonicalPath(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}
