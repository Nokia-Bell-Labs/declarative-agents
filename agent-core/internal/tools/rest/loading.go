// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// LoadDefinition reads and validates a REST definition YAML file.
func LoadDefinition(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, fmt.Errorf("load REST definition %s: %w", path, err)
	}
	def, err := ParseDefinition(data)
	if err != nil {
		return Definition{}, fmt.Errorf("parse REST definition %s: %w", path, err)
	}
	return def, nil
}

// ParseDefinition parses and validates REST definition YAML bytes.
func ParseDefinition(data []byte) (Definition, error) {
	var file DefinitionFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return Definition{}, fmt.Errorf("parse REST definition: %w", err)
	}
	if err := ValidateDefinition(file.Rest); err != nil {
		return Definition{}, err
	}
	return file.Rest, nil
}
