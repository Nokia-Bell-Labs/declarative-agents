// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolrest "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest"
	"gopkg.in/yaml.v3"
)

type loadedProgram struct {
	Config          runtimeConfig
	Definitions     []catalog.ToolDef
	RestDefinitions toolrest.Collection
	Ref             core.ProgramRef
}

func augmentRollbackResources(resources runResources, checkpoint core.Checkpoint) (runResources, error) {
	if !selectsRollbackTool(resources.Definitions) {
		return resources, nil
	}
	position, execution, err := checkpoint.Load()
	if err != nil {
		return runResources{}, fmt.Errorf("load target checkpoint program: %w", err)
	}
	target, err := loadReferencedProgram(position.Snapshot.Program, resources.Config)
	if err != nil {
		return runResources{}, err
	}
	target.Definitions, err = definitionsForExecution(target.Definitions, execution)
	if err != nil {
		return runResources{}, err
	}
	// Target definitions provide the originating Undo builders. The lifecycle
	// profile remains authoritative for same-named operator words.
	resources.Definitions = catalog.MergeToolDefs(target.Definitions, resources.Definitions)
	resources.RestDefinitions = mergeRESTCollections(
		target.RestDefinitions, resources.RestDefinitions,
	)
	return resources, nil
}

func definitionsForExecution(
	defs []catalog.ToolDef, execution core.Execution,
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

func selectsRollbackTool(defs []catalog.ToolDef) bool {
	for _, def := range defs {
		if def.Name == "checkpoint_rollback" || def.Init == "checkpoint_rollback" {
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
	files, err := programAssetFiles(cfg)
	if err != nil {
		return core.ProgramRef{}, err
	}
	hash := sha256.New()
	for _, path := range files {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return core.ProgramRef{}, fmt.Errorf("read program asset %s: %w", path, readErr)
		}
		_, _ = hash.Write([]byte(path))
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(data)
		_, _ = hash.Write([]byte{0})
	}
	return core.ProgramRef{
		Profile: canonicalPath(cfg.Profile),
		Digest:  hex.EncodeToString(hash.Sum(nil)),
	}, nil
}

func loadReferencedProgram(ref core.ProgramRef, runtime runtimeConfig) (loadedProgram, error) {
	if ref.Profile == "" || ref.Digest == "" {
		return loadedProgram{}, fmt.Errorf("target checkpoint has no declarative program reference")
	}
	cfg, err := runtimeConfigForProfile(ref.Profile, runtime)
	if err != nil {
		return loadedProgram{}, err
	}
	actual, err := buildProgramRef(cfg)
	if err != nil {
		return loadedProgram{}, err
	}
	if actual.Digest != ref.Digest {
		return loadedProgram{}, fmt.Errorf(
			"target program %s changed: checkpoint digest %s, current digest %s",
			ref.Profile, ref.Digest, actual.Digest,
		)
	}
	defs, restDefs, err := loadRuntimeDefinitions(cfg)
	if err != nil {
		return loadedProgram{}, fmt.Errorf("load target program %s: %w", ref.Profile, err)
	}
	return loadedProgram{
		Config: cfg, Definitions: defs, RestDefinitions: restDefs, Ref: actual,
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

func programAssetFiles(cfg runtimeConfig) ([]string, error) {
	paths := []string{cfg.Profile, cfg.Machine}
	paths = append(paths, cfg.Tools...)
	paths = append(paths, cfg.ToolDeclarations...)
	paths = append(paths, cfg.RestDefinitions...)
	files := make(map[string]bool)
	for _, path := range paths {
		if path != "" {
			files[canonicalPath(path)] = true
		}
	}
	for _, declaration := range cfg.ToolDeclarations {
		if err := addDeclarationClosure(files, declaration, nil); err != nil {
			return nil, err
		}
	}
	for _, dir := range append(
		append([]string(nil), cfg.ToolConfigDirs...), cfg.RestConfigDirs...,
	) {
		if err := addProgramDirectory(files, dir); err != nil {
			return nil, err
		}
	}
	result := make([]string, 0, len(files))
	for path := range files {
		result = append(result, path)
	}
	sort.Strings(result)
	return result, nil
}

func addDeclarationClosure(files map[string]bool, path string, stack map[string]bool) error {
	path = canonicalPath(path)
	if stack == nil {
		stack = make(map[string]bool)
	}
	if stack[path] {
		return fmt.Errorf("circular tool declaration include: %s", path)
	}
	stack[path] = true
	defer delete(stack, path)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read tool declaration %s: %w", path, err)
	}
	files[path] = true
	var bundle struct {
		Includes []string `yaml:"includes"`
	}
	if err := yaml.Unmarshal(data, &bundle); err != nil {
		return fmt.Errorf("parse tool declaration %s: %w", path, err)
	}
	for _, include := range bundle.Includes {
		if !filepath.IsAbs(include) {
			include = filepath.Join(filepath.Dir(path), include)
		}
		if err := addDeclarationClosure(files, include, stack); err != nil {
			return err
		}
	}
	return nil
}

func addProgramDirectory(files map[string]bool, dir string) error {
	return filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && entry.Type().IsRegular() {
			files[canonicalPath(path)] = true
		}
		return nil
	})
}

func mergeRESTCollections(base, override toolrest.Collection) toolrest.Collection {
	merged := base
	mergeMap(&merged.Clients, override.Clients)
	mergeMap(&merged.Servers, override.Servers)
	mergeMap(&merged.Auth, override.Auth)
	mergeMap(&merged.Limits, override.Limits)
	mergeMap(&merged.RetryPolicies, override.RetryPolicies)
	mergeMap(&merged.ResponseMappings, override.ResponseMappings)
	return merged
}

func mergeMap[K comparable, V any](target *map[K]V, values map[K]V) {
	if *target == nil {
		*target = make(map[K]V)
	}
	for key, value := range values {
		(*target)[key] = value
	}
}
