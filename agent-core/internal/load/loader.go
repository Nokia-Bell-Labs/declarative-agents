// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package load resolves one agent profile and its declaration closure.
package load

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

// Options reserves caller-owned path configuration. The caller applies
// CoreRoot through the process-scoped core path mapper before loading.
type Options struct {
	CoreRoot         string
	ProfileLoaded    func(string, catalog.AgentProfile) error
	MachineOverride  string
	ResolveSelection func(
		catalog.AgentProfile, core.MachineSpec, []catalog.ToolDef, catalog.FileVisitor,
	) ([]string, error)
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
	Assets       map[string][]byte
	// FileRoots names the library root each rooted file was reached through,
	// recorded while the closure's declared roots were in force, because the
	// roots are restored when the load returns (srd056 R3.2).
	FileRoots map[string]string
	// Types indexes every named type the closure reached. Tool schemas are
	// already resolved against it by the time a consumer sees them, so the
	// registry is here for the dump and for later typed checks rather than
	// for resolution (srd051 R4.1).
	Types *typesys.Registry
}

// LoadClosure loads and validates the complete declaration closure once.
func LoadClosure(profilePath string, options Options) (*Closure, error) {
	profilePath = canonicalPath(profilePath)
	var visited []string
	assets := make(map[string][]byte)
	visit := func(path string, data []byte) error {
		path = canonicalPath(path)
		visited = append(visited, path)
		assets[path] = append([]byte(nil), data...)
		return nil
	}
	profile, err := catalog.LoadProfileWithVisitor(profilePath, visit)
	if err != nil {
		return nil, fmt.Errorf("load profile: %w", err)
	}
	// The profile's roots, with any --library override, are in force for the
	// rest of the load and restored after it. The same lock serializes every
	// closure load, so a request-scoped load never sees these roots nor this
	// one its (srd056 R2.2, R2.3; GH-2251).
	var closure *Closure
	err = corepath.WithLibraryRoots(profile.Libraries, func() error {
		if options.ProfileLoaded != nil {
			if err := options.ProfileLoaded(profilePath, profile); err != nil {
				return err
			}
		}
		resolved, err := loadResolvedConfig(profile, filepath.Dir(profilePath), options, visit)
		if err != nil {
			return err
		}
		closure, err = assembleClosure(profilePath, profile, resolved, visited, assets, options.ResolveSelection == nil)
		return err
	})
	if err != nil {
		return nil, err
	}
	return closure, nil
}

// assembleClosure fixes the closure's file inventory and captured bytes once
// every declaration has loaded, while the profile's library roots are still in
// force.
func assembleClosure(
	profilePath string, profile catalog.AgentProfile, resolved resolvedConfig,
	visited []string, assets map[string][]byte, includeSelections bool,
) (*Closure, error) {
	files, err := programFiles(profilePath, resolved.machinePath, profile, visited, includeSelections)
	if err != nil {
		return nil, err
	}
	if err := readMissingAssets(files, assets); err != nil {
		return nil, err
	}
	return &Closure{
		ProfilePath: profilePath, Profile: profile,
		ToolUniverse: resolved.universe, Selection: resolved.selection, Selected: resolved.selected,
		Rest: resolved.rest, Machine: resolved.machine, Files: files, Assets: assets,
		FileRoots: fileRoots(files), Types: resolved.types,
	}, nil
}

func fileRoots(files []string) map[string]string {
	roots := map[string]string{}
	for _, file := range files {
		if root, ok := corepath.LibraryRootOf(file); ok {
			roots[file] = root
		}
	}
	return roots
}

type resolvedConfig struct {
	universe    []catalog.ToolDef
	selection   []string
	selected    []catalog.ToolDef
	rest        toolrest.Collection
	machine     core.MachineSpec
	machinePath string
	types       *typesys.Registry
}

func loadResolvedConfig(
	profile catalog.AgentProfile, profileDir string, options Options, visit catalog.FileVisitor,
) (resolvedConfig, error) {
	universe, toolImports, toolTypeIndex, err := loadToolUniverse(profile, visit)
	if err != nil {
		return resolvedConfig{}, err
	}
	rest, err := toolrest.LoadDefinitionsWithVisitor(
		profile.RestDefinitions, profile.RestConfigDirs, toolrest.FileVisitor(visit),
	)
	if err != nil {
		return resolvedConfig{}, fmt.Errorf("load REST definitions: %w", err)
	}
	machinePath := profile.Machine
	if options.MachineOverride != "" {
		machinePath = options.MachineOverride
	}
	machine, err := loadMachine(machinePath, visit)
	if err != nil {
		return resolvedConfig{}, err
	}
	selection, selected, err := resolveSelectedTools(profile, machine, universe, options, visit)
	if err != nil {
		return resolvedConfig{}, err
	}
	if err := validateClosureUsedness(
		selected, universe, rest, toolImports, toolTypeIndex.usedPaths(universe),
		machine, machinePath, profileDir,
	); err != nil {
		return resolvedConfig{}, err
	}
	return resolvedConfig{
		universe: universe, selection: selection, selected: selected,
		rest: rest, machine: machine, machinePath: machinePath,
		types: toolTypeIndex.registry,
	}, nil
}

func resolveSelectedTools(
	profile catalog.AgentProfile,
	machine core.MachineSpec,
	universe []catalog.ToolDef,
	options Options,
	visit catalog.FileVisitor,
) ([]string, []catalog.ToolDef, error) {
	var selection []string
	var err error
	if options.ResolveSelection != nil {
		selection, err = options.ResolveSelection(profile, machine, universe, visit)
	} else {
		selection, err = catalog.LoadToolSelectionsWithVisitor(profile.Tools, visit)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load tool selection: %w", err)
	}
	selected, err := catalog.SelectTools(universe, selection)
	if err != nil {
		return nil, nil, fmt.Errorf("select tools: %w", err)
	}
	return selection, selected, nil
}

// loadMachine loads the machine and every stage fragment it expands
// through the closure's visitor, so the fragments land in Files and Assets
// like any other declaration (srd052 R4).
func loadMachine(path string, visit catalog.FileVisitor) (core.MachineSpec, error) {
	machine, err := core.LoadMachineClosure(path, func(file string, data []byte) error {
		return visit(file, data)
	})
	if err != nil {
		return core.MachineSpec{}, fmt.Errorf("load machine spec: %w", err)
	}
	return machine, nil
}

func readMissingAssets(files []string, assets map[string][]byte) error {
	for _, path := range files {
		path = canonicalPath(path)
		if _, ok := assets[path]; ok {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read program asset %s: %w", path, err)
		}
		assets[path] = data
	}
	return nil
}

func loadToolUniverse(
	profile catalog.AgentProfile, visit catalog.FileVisitor,
) ([]catalog.ToolDef, []catalog.ToolImport, *toolTypes, error) {
	closure, err := catalog.LoadToolDeclarationClosureWithImports(
		profile.ToolConfigDirs, profile.ToolDeclarations, visit,
	)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load tool declarations: %w", err)
	}
	registry, err := typesys.Build(closure.TypeUnits...)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("load declaration types: %w", err)
	}
	universe, referenced, err := catalog.ResolveToolSchemas(
		catalog.MergeToolDefs(closure.FromDirs, closure.Local), registry,
	)
	if err != nil {
		return nil, nil, nil, err
	}
	return universe, closure.Imports, &toolTypes{registry: registry, referenced: referenced}, nil
}

// toolTypes pairs the registry with the types each tool referenced, so
// usedness can credit a type unit's import once a selected tool reaches one of
// its types.
type toolTypes struct {
	registry   *typesys.Registry
	referenced map[string][]string
}

// usedPaths returns the declaration files that supplied a type the closure
// references. Selection is the wrong scope here: a root declaration escapes
// selection-based usedness (srd050 R5.7) while its own imports do not.
func (t *toolTypes) usedPaths(universe []catalog.ToolDef) map[string]bool {
	used := map[string]bool{}
	if t == nil {
		return used
	}
	for _, tool := range universe {
		for _, ref := range t.referenced[tool.Name] {
			if path := t.registry.PathOf(ref); path != "" {
				used[path] = true
			}
		}
	}
	return used
}

func programFiles(
	profilePath, machinePath string,
	profile catalog.AgentProfile,
	visited []string,
	includeProfileSelections bool,
) ([]string, error) {
	selections := profile.Tools
	if !includeProfileSelections {
		selections = nil
	}
	paths := catalog.ProgramPaths{
		Profile:          profilePath,
		Machine:          machinePath,
		ToolSelections:   selections,
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
