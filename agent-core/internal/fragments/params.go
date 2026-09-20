// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package fragments holds the parameter and substitution rules of a
// parameterized declaration fragment (srd052). A fragment declares typed
// parameters; an importer expands it with arguments; substitution fills
// scalar holes and nothing else. The loader owns where this runs; this package
// owns what an argument may be and where it may land.
package fragments

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Param is one declared parameter of a fragment (srd052 R1.1).
type Param struct {
	Name        string   `yaml:"name"`
	Type        string   `yaml:"type"`
	Enum        []string `yaml:"enum,omitempty"`
	Default     *string  `yaml:"default,omitempty"`
	Description string   `yaml:"description,omitempty"`
}

// Expansion is one importer's application of a fragment (srd052 R2.1).
type Expansion struct {
	Fragment string            `yaml:"fragment"`
	As       string            `yaml:"as,omitempty"`
	Args     map[string]string `yaml:"args,omitempty"`
}

// Arg is a resolved argument: its text and the YAML tag its parameter type
// gives it, so a whole-scalar hole re-decodes as the declared type rather
// than as text.
type Arg struct {
	Value string
	Tag   string
}

var paramTags = map[string]string{
	"string": "!!str", "integer": "!!int", "number": "!!float", "boolean": "!!bool",
}

// ValidateParams checks the declarations themselves: unique names, a known
// type, and a default that satisfies its own type and enum.
func ValidateParams(params []Param) error {
	seen := map[string]bool{}
	for _, param := range params {
		if param.Name == "" {
			return fmt.Errorf("parameter declared with no name")
		}
		if seen[param.Name] {
			return fmt.Errorf("parameter %q declared twice", param.Name)
		}
		seen[param.Name] = true
		if _, ok := paramTags[param.Type]; !ok {
			return fmt.Errorf("parameter %q: type %q is not one of string, integer, number, boolean",
				param.Name, param.Type)
		}
		if param.Default != nil {
			if err := checkValue(param, *param.Default); err != nil {
				return fmt.Errorf("parameter %q: default %w", param.Name, err)
			}
		}
	}
	return nil
}

// ResolveArgs applies defaults and checks every argument against its
// parameter (srd052 R2.2). Errors name the parameter; the caller adds the
// fragment and the importer.
func ResolveArgs(params []Param, args map[string]string) (map[string]Arg, error) {
	declared := make(map[string]Param, len(params))
	for _, param := range params {
		declared[param.Name] = param
	}
	for _, name := range sortedKeys(args) {
		if _, ok := declared[name]; !ok {
			return nil, fmt.Errorf("argument %q names no declared parameter", name)
		}
	}
	resolved := make(map[string]Arg, len(params))
	for _, param := range params {
		value, supplied := args[param.Name]
		switch {
		case supplied:
		case param.Default != nil:
			value = *param.Default
		default:
			return nil, fmt.Errorf("parameter %q is required and was not supplied", param.Name)
		}
		if err := checkValue(param, value); err != nil {
			return nil, fmt.Errorf("parameter %q: %w", param.Name, err)
		}
		resolved[param.Name] = Arg{Value: value, Tag: paramTags[param.Type]}
	}
	return resolved, nil
}

func checkValue(param Param, value string) error {
	switch param.Type {
	case "integer":
		if _, err := strconv.ParseInt(value, 10, 64); err != nil {
			return fmt.Errorf("value %q is not an integer", value)
		}
	case "number":
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return fmt.Errorf("value %q is not a number", value)
		}
	case "boolean":
		if value != "true" && value != "false" {
			return fmt.Errorf("value %q is not true or false", value)
		}
	}
	if len(param.Enum) > 0 && !contains(param.Enum, value) {
		return fmt.Errorf("value %q is not one of %s", value, strings.Join(param.Enum, ", "))
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// FormatArgs renders arguments as "name=value, ..." in name order, for
// diagnostics and the dump.
func FormatArgs(args map[string]string) string {
	parts := make([]string, 0, len(args))
	for _, key := range sortedKeys(args) {
		parts = append(parts, key+"="+args[key])
	}
	return strings.Join(parts, ", ")
}
