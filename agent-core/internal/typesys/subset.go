// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package typesys

import (
	"fmt"
	"sort"
)

// The closed subset of JSON Schema a declaration type may use (srd051 R2.1,
// R2.2). It is closed so that a selector path stays decidable against a
// shape; widening it is a later decision and is not inferred from a keyword
// happening to parse (R2.4).
var (
	allowedKeywords = map[string]bool{
		"type": true, "properties": true, "required": true,
		"items": true, "description": true, "enum": true,
	}
	allowedTypes = map[string]bool{
		"object": true, "array": true, "string": true,
		"number": true, "integer": true, "boolean": true,
	}
)

// ValidateSubset reports the first keyword outside the closed subset, naming
// where it sits. It descends properties and items, so a violation nested at
// any depth is reported rather than ignored (srd051 R2.3). A reference object
// is left to the registry, which resolves it.
func ValidateSubset(schema map[string]any) error {
	return validateSubsetAt(schema, "")
}

func validateSubsetAt(schema map[string]any, path string) error {
	if schema == nil {
		return nil
	}
	if _, isRef := schema[TypeRefKey]; isRef {
		return validateRefObject(schema, path)
	}
	for _, keyword := range sortedKeys(schema) {
		if !allowedKeywords[keyword] {
			return fmt.Errorf("%s: keyword %q is outside the declaration type subset",
				describePath(path), keyword)
		}
	}
	if err := validateTypeKeyword(schema, path); err != nil {
		return err
	}
	return validateNested(schema, path)
}

// validateRefObject enforces that a reference carries $type and nothing else,
// so a reference mixed with sibling keywords has no undefined meaning
// (srd051 R3.2).
func validateRefObject(schema map[string]any, path string) error {
	if len(schema) == 1 {
		return nil
	}
	for _, keyword := range sortedKeys(schema) {
		if keyword != TypeRefKey {
			return fmt.Errorf("%s: %s reference cannot carry sibling keyword %q",
				describePath(path), TypeRefKey, keyword)
		}
	}
	return nil
}

func validateTypeKeyword(schema map[string]any, path string) error {
	raw, present := schema["type"]
	if !present {
		return nil
	}
	name, ok := raw.(string)
	if !ok {
		return fmt.Errorf("%s: type must be a string", describePath(path))
	}
	if !allowedTypes[name] {
		return fmt.Errorf("%s: type %q is outside the declaration type subset",
			describePath(path), name)
	}
	return nil
}

func validateNested(schema map[string]any, path string) error {
	if properties, ok := asSchema(schema["properties"]); ok {
		for _, name := range sortedKeys(properties) {
			nested, ok := asSchema(properties[name])
			if !ok {
				continue
			}
			if err := validateSubsetAt(nested, join(path, "properties."+name)); err != nil {
				return err
			}
		}
	}
	if items, ok := asSchema(schema["items"]); ok {
		return validateSubsetAt(items, join(path, "items"))
	}
	return nil
}

func asSchema(value any) (map[string]any, bool) {
	schema, ok := value.(map[string]any)
	return schema, ok
}

func sortedKeys(schema map[string]any) []string {
	keys := make([]string, 0, len(schema))
	for key := range schema {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func join(path, segment string) string {
	if path == "" {
		return segment
	}
	return path + "." + segment
}

func describePath(path string) string {
	if path == "" {
		return "schema"
	}
	return "schema " + path
}
