// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package rest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	restdef "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/definition"
	restvalidation "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/rest/validation"
)

// LoadDefinition reads and validates a REST definition YAML file.
func LoadDefinition(path string) (restdef.Definition, error) {
	def, err := restdef.LoadDefinition(path)
	if err != nil {
		return restdef.Definition{}, err
	}
	if err := restvalidation.ValidateDefinition(def); err != nil {
		return restdef.Definition{}, err
	}
	return def, nil
}

// ParseDefinition parses and validates REST definition YAML bytes.
func ParseDefinition(data []byte) (restdef.Definition, error) {
	def, err := restdef.ParseDefinition(data)
	if err != nil {
		return restdef.Definition{}, err
	}
	if err := restvalidation.ValidateDefinition(def); err != nil {
		return restdef.Definition{}, err
	}
	return def, nil
}

// ValidateDefinition validates a declarative REST definition before use.
func ValidateDefinition(def restdef.Definition) error {
	return restvalidation.ValidateDefinition(def)
}

// CompileOpenAPIImports loads OpenAPI imports into the internal REST model.
func CompileOpenAPIImports(def *restdef.Definition, baseDir string) error {
	return restdef.CompileOpenAPIImports(def, baseDir)
}

// RetryAggregateTimeout returns the conservative dispatch authority for one
// retrying HTTP operation. pkg/profileaudit calls this parent wrapper so it
// does not grow a new public-to-internal import edge onto rest/validation.
func RetryAggregateTimeout(attemptTimeout time.Duration, retry restdef.RetryPolicy) (time.Duration, error) {
	return restvalidation.RetryAggregateTimeout(attemptTimeout, retry)
}

// ValidateRuntimeInput rejects transport authority supplied at runtime.
func ValidateRuntimeInput(input map[string]interface{}) error {
	return restvalidation.ValidateRuntimeInput(input)
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
