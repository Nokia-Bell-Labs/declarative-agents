// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"fmt"
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/envexpand"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/yamlstrict"
	"gopkg.in/yaml.v3"
)

// FileVisitor observes a REST or OpenAPI declaration after it is read.
type FileVisitor func(string, []byte) error

// LoadDefinition reads a REST definition YAML file and compiles OpenAPI
// imports. It does not validate; rest.LoadDefinition composes this with
// validation.ValidateDefinition.
func LoadDefinition(path string) (Definition, error) {
	return LoadDefinitionWithVisitor(path, nil)
}

// LoadDefinitionWithVisitor reads a REST definition and reports every local
// source used to compile it.
func LoadDefinitionWithVisitor(path string, visit FileVisitor) (Definition, error) {
	return LoadDefinitionClosure([]string{path}, visit)
}

func readDefinitionFile(path string, visit FileVisitor) (DefinitionFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefinitionFile{}, fmt.Errorf("load REST definition %s: %w", path, err)
	}
	if visit != nil {
		if err := visit(path, data); err != nil {
			return DefinitionFile{}, fmt.Errorf("visit REST definition %s: %w", path, err)
		}
	}
	file, err := parseDefinitionFileRaw(data)
	if err != nil {
		return DefinitionFile{}, fmt.Errorf("parse REST definition %s: %w", path, err)
	}
	return file, nil
}

// ParseDefinition parses REST definition YAML bytes. It does not validate;
// rest.ParseDefinition composes this with validation.ValidateDefinition.
func ParseDefinition(data []byte) (Definition, error) {
	return parseDefinitionRaw(data)
}

// parseDefinitionRaw decodes a trusted REST definition with strict field
// checking. REST definitions are trusted, chart-mounted config, so an unknown
// field is an authoring error, not data to ignore: KnownFields(true) rejects it
// loudly instead of silently dropping it. This closes the gap where documented
// but unimplemented machine_request fields (error_responses, trace, and the
// like) were accepted and then had no effect (GH-486).
func parseDefinitionRaw(data []byte) (Definition, error) {
	file, err := parseDefinitionFileRaw(data)
	return file.Rest, err
}

func parseDefinitionFileRaw(data []byte) (DefinitionFile, error) {
	expanded := envexpand.Expand(data)
	var file DefinitionFile
	if err := yamlstrict.Unmarshal(expanded, &file); err != nil {
		return DefinitionFile{}, fmt.Errorf("parse REST definition: %w", err)
	}
	var document yaml.Node
	if err := yaml.Unmarshal(expanded, &document); err != nil {
		return DefinitionFile{}, fmt.Errorf("parse REST definition shape: %w", err)
	}
	root := &document
	if document.Kind == yaml.DocumentNode && len(document.Content) > 0 {
		root = document.Content[0]
	}
	file.hasRest = yamlstrict.FieldPresent(root, "rest")
	file.hasImports = yamlstrict.FieldPresent(root, "imports")
	if !file.hasRest {
		return DefinitionFile{}, fmt.Errorf("parse REST definition: top-level rest field is required")
	}
	return file, nil
}
