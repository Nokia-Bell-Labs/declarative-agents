// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// LoadDefinition reads and validates a REST definition YAML file.
func LoadDefinition(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("load REST definition %s: %w", path, err)
	}
	def, err := parseDefinitionRaw(data)
	if err != nil {
		return Definition{}, fmt.Errorf("parse REST definition %s: %w", path, err)
	}
	if err := CompileOpenAPIImports(&def, filepath.Dir(path)); err != nil {
		return Definition{}, fmt.Errorf("compile OpenAPI imports %s: %w", path, err)
	}
	if err := ValidateDefinition(def); err != nil {
		return Definition{}, err
	}
	return def, nil
}

// ParseDefinition parses and validates REST definition YAML bytes.
func ParseDefinition(data []byte) (Definition, error) {
	def, err := parseDefinitionRaw(data)
	if err != nil {
		return Definition{}, err
	}
	if err := ValidateDefinition(def); err != nil {
		return Definition{}, err
	}
	return def, nil
}

func parseDefinitionRaw(data []byte) (Definition, error) {
	var file DefinitionFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return Definition{}, fmt.Errorf("parse REST definition: %w", err)
	}
	return file.Rest, nil
}

// LoadDefinitions reads REST definition files and directories.
func LoadDefinitions(paths, dirs []string) (Collection, error) {
	files, err := definitionFiles(paths, dirs)
	if err != nil {
		return Collection{}, err
	}
	collection := NewCollection()
	for _, path := range files {
		def, err := LoadDefinition(path)
		if err != nil {
			return Collection{}, err
		}
		if err := collection.Add(def); err != nil {
			return Collection{}, fmt.Errorf("merge REST definition %s: %w", path, err)
		}
	}
	return collection, nil
}

func definitionFiles(paths, dirs []string) ([]string, error) {
	files := append([]string(nil), paths...)
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("scan REST config dir %s: %w", dir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
				continue
			}
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}
