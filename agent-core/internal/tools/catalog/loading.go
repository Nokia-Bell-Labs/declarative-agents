// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// LoadToolSelection reads a YAML file listing tool names.
func LoadToolSelection(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load tool selection %s: %w", path, err)
	}
	var sel ToolSelectionFile
	if err := yaml.Unmarshal(data, &sel); err != nil {
		return nil, fmt.Errorf("parse tool selection %s: %w", path, err)
	}
	return sel.Tools, nil
}

// LoadToolSelections reads multiple selection files and deduplicates names.
func LoadToolSelections(paths []string) ([]string, error) {
	seen := map[string]bool{}
	var merged []string
	for _, p := range paths {
		names, err := LoadToolSelection(p)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				merged = append(merged, n)
			}
		}
	}
	return merged, nil
}

// LoadToolDeclarations loads multiple declaration files and merges them.
func LoadToolDeclarations(paths []string) ([]ToolDef, error) {
	var all []ToolDef
	for _, p := range paths {
		defs, err := LoadToolDefs(p)
		if err != nil {
			return nil, err
		}
		all = MergeToolDefs(all, defs)
	}
	return all, nil
}

// LoadToolDeclarationsFromDirs scans directories for sorted *.yaml files.
func LoadToolDeclarationsFromDirs(dirs []string) ([]ToolDef, error) {
	var paths []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("scan tool config dir %s: %w", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
				continue
			}
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	return LoadToolDeclarations(paths)
}

// SelectTools filters declarations to selected names.
func SelectTools(declarations []ToolDef, selection []string) ([]ToolDef, error) {
	index := make(map[string]ToolDef, len(declarations))
	for _, d := range declarations {
		index[d.Name] = d
	}
	var result []ToolDef
	for _, name := range selection {
		d, ok := index[name]
		if !ok {
			return nil, fmt.Errorf("tool %q is selected but not declared", name)
		}
		result = append(result, d)
	}
	return result, nil
}

// LoadToolDefs reads one declaration file and resolves includes.
func LoadToolDefs(path string) ([]ToolDef, error) {
	return loadToolDefsRecursive(path, nil)
}

func loadToolDefsRecursive(path string, seen map[string]bool) ([]ToolDef, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve path %s: %w", path, err)
	}
	if seen == nil {
		seen = make(map[string]bool)
	}
	if seen[abs] {
		return nil, fmt.Errorf("circular include detected: %s", abs)
	}
	seen[abs] = true

	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("load tool defs %s: %w", abs, err)
	}
	var file ToolDefsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse tool defs %s: %w", abs, err)
	}

	var base []ToolDef
	dir := filepath.Dir(abs)
	for _, inc := range file.Includes {
		incPath := inc
		if !filepath.IsAbs(incPath) {
			incPath = filepath.Join(dir, incPath)
		}
		incDefs, err := loadToolDefsRecursive(incPath, seen)
		if err != nil {
			return nil, fmt.Errorf("include %s from %s: %w", inc, abs, err)
		}
		base = MergeToolDefs(base, incDefs)
	}
	if err := validateToolDefs(file.Tools); err != nil {
		return nil, err
	}
	return MergeToolDefs(base, file.Tools), nil
}

// ParseToolDefs parses YAML bytes into tool definitions without resolving includes.
func ParseToolDefs(data []byte) ([]ToolDef, error) {
	var file ToolDefsFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse tool defs: %w", err)
	}
	return file.Tools, validateToolDefs(file.Tools)
}

func validateToolDefs(defs []ToolDef) error {
	for i, td := range defs {
		if td.Name == "" {
			return fmt.Errorf("tool at index %d has no name", i)
		}
		switch td.Type {
		case "builtin":
			if td.Init == "" {
				return fmt.Errorf("builtin tool %q has no init field", td.Name)
			}
		case "exec", "":
			if td.Binary == "" {
				return fmt.Errorf("tool %q has no binary", td.Name)
			}
		default:
			return fmt.Errorf("tool %q: unknown type %q", td.Name, td.Type)
		}
	}
	return nil
}
