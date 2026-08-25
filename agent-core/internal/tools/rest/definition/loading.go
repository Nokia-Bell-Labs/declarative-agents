// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/envexpand"
	"gopkg.in/yaml.v3"
)

// LoadDefinition reads a REST definition YAML file and compiles OpenAPI
// imports. It does not validate; rest.LoadDefinition composes this with
// validation.ValidateDefinition.
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
	return def, nil
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
	var file DefinitionFile
	decoder := yaml.NewDecoder(bytes.NewReader(envexpand.Expand(data)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return Definition{}, fmt.Errorf("parse REST definition: %w", err)
	}
	return file.Rest, nil
}
