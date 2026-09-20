// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"fmt"
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"
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

// readDefinitionFile reads one file and returns it decoded beside its
// environment-expanded bytes, which a fragment is instantiated from
// (srd052 R2.4).
func readDefinitionFile(path string, visit FileVisitor) (DefinitionFile, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return DefinitionFile{}, nil, fmt.Errorf("load REST definition %s: %w", path, err)
	}
	if visit != nil {
		if err := visit(path, data); err != nil {
			return DefinitionFile{}, nil, fmt.Errorf("visit REST definition %s: %w", path, err)
		}
	}
	expanded := envexpand.Expand(data)
	file, err := parseDefinitionFileExpanded(expanded)
	if err != nil {
		return DefinitionFile{}, nil, fmt.Errorf("parse REST definition %s: %w", path, err)
	}
	return file, expanded, nil
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
	return parseDefinitionFileExpanded(envexpand.Expand(data))
}

func parseDefinitionFileExpanded(expanded []byte) (DefinitionFile, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(expanded, &document); err != nil {
		return DefinitionFile{}, fmt.Errorf("parse REST definition shape: %w", err)
	}
	root := &document
	if document.Kind == yaml.DocumentNode && len(document.Content) > 0 {
		root = document.Content[0]
	}
	file, err := decodeDefinitionFile(expanded, yamlstrict.FieldPresent(root, "params"))
	if err != nil {
		return DefinitionFile{}, fmt.Errorf("parse REST definition: %w", err)
	}
	file.hasRest = yamlstrict.FieldPresent(root, "rest")
	file.hasImports = yamlstrict.FieldPresent(root, "imports")
	file.hasParams = yamlstrict.FieldPresent(root, "params")
	file.hasInstantiate = yamlstrict.FieldPresent(root, "expand")
	// A unit is a fragment or it is not; half of one declares nothing
	// (srd052 R1.3).
	if file.hasParams && !file.hasRest {
		return DefinitionFile{}, fmt.Errorf("parse REST definition: fragment declares params but no rest body")
	}
	if !file.hasRest {
		return DefinitionFile{}, fmt.Errorf("parse REST definition: top-level rest field is required")
	}
	if !file.hasParams && fragments.References(expanded) {
		return DefinitionFile{}, fmt.Errorf("parse REST definition: references $param but declares no params")
	}
	return file, nil
}

// decodeDefinitionFile decodes a file strictly. A fragment's body is left
// undecoded: it holds $param references where typed fields stand, and it is
// only a definition once its arguments arrive (srd052 R2.4). Its header --
// unit, imports, params, instantiate -- is still checked strictly, and an
// unknown top-level field is still rejected.
func decodeDefinitionFile(expanded []byte, fragment bool) (DefinitionFile, error) {
	if !fragment {
		var file DefinitionFile
		return file, yamlstrict.Unmarshal(expanded, &file)
	}
	var header struct {
		Unit        string                    `yaml:"unit,omitempty"`
		Imports     []string                  `yaml:"imports,omitempty"`
		Params      []fragments.Param         `yaml:"params,omitempty"`
		Instantiate []fragments.Instantiation `yaml:"expand,omitempty"`
		Rest        yaml.Node                 `yaml:"rest"`
	}
	if err := yamlstrict.Unmarshal(expanded, &header); err != nil {
		return DefinitionFile{}, err
	}
	return DefinitionFile{
		Unit: header.Unit, Imports: header.Imports,
		Params: header.Params, Instantiate: header.Instantiate,
	}, nil
}
