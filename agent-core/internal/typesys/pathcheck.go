// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package typesys

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Selector path checking against a resolved schema (srd038). A path that names
// a field the producing type does not have used to surface at runtime as an
// UnresolvedPathError; with a type behind the label it is decidable at load.

// CheckPath walks a dotted selector path through a resolved schema. A nil or
// empty schema is untyped: nothing is known about it, so nothing is reported.
func CheckPath(schema map[string]any, path []string) error {
	// A lone "$" is srd038's whole-output selector: it reads the value itself
	// rather than a field of it, so every type satisfies it.
	if len(path) == 1 && path[0] == "$" {
		return nil
	}
	return checkPathAt(schema, path, nil)
}

// SchemaAt returns the schema a selector path reaches, and false wherever the
// type does not decide it: an untyped or undeclared value, a field the type
// lacks, a scalar walked past, or a field name applied to an array. The last
// is a path CheckPath accepts, but which value it yields depends on how the
// runtime spreads a field across elements, and a guessed shape is worse than
// none for a label a selector will be checked against.
func SchemaAt(schema map[string]any, path []string) (map[string]any, bool) {
	if len(path) == 1 && path[0] == "$" {
		path = nil
	}
	for _, component := range path {
		switch schemaType(schema) {
		case "object":
			properties, _ := schema["properties"].(map[string]any)
			next, ok := properties[component].(map[string]any)
			if !ok {
				return nil, false
			}
			schema = next
		case "array":
			index, err := strconv.Atoi(component)
			if err != nil || index < 0 {
				return nil, false
			}
			items, ok := schema["items"].(map[string]any)
			if !ok {
				return nil, false
			}
			schema = items
		default:
			return nil, false
		}
	}
	if len(schema) == 0 {
		return nil, false
	}
	return schema, true
}

func checkPathAt(schema map[string]any, path, walked []string) error {
	if len(schema) == 0 || len(path) == 0 {
		return nil
	}
	component, rest := path[0], path[1:]
	switch schemaType(schema) {
	case "object":
		next, err := objectField(schema, component, walked)
		if err != nil {
			return err
		}
		return checkPathAt(next, rest, append(walked, component))
	case "array":
		return checkArrayPath(schema, component, rest, walked)
	case "":
		// A schema that declares no type decides nothing about its fields, the
		// same as a nil or empty one. Describing a value without constraining
		// it is how a declaration says the shape is the producer's to decide,
		// and reporting "untyped, so it has no field" contradicted that.
		return nil
	default:
		return fmt.Errorf("%s is %s, so it has no field %q",
			describeWalked(walked), describeScalar(schema), component)
	}
}

func objectField(schema map[string]any, component string, walked []string) (map[string]any, error) {
	properties, _ := schema["properties"].(map[string]any)
	if field, ok := properties[component].(map[string]any); ok {
		return field, nil
	}
	if _, ok := properties[component]; ok {
		// Declared but with no shape of its own; nothing further is decidable.
		return nil, nil
	}
	return nil, fmt.Errorf("%s has no field %q%s",
		describeWalked(walked), component, suggestField(properties, component))
}

// checkArrayPath accepts an index into the array or a path applied to its
// items, so $from(rows).0.text and $from(rows).text both resolve where the
// shape allows.
func checkArrayPath(schema map[string]any, component string, rest, walked []string) error {
	items, _ := schema["items"].(map[string]any)
	if index, err := strconv.Atoi(component); err == nil {
		if index < 0 {
			return fmt.Errorf("%s is an array, so index %q must not be negative",
				describeWalked(walked), component)
		}
		return checkPathAt(items, rest, append(walked, component))
	}
	return checkPathAt(items, append([]string{component}, rest...), walked)
}

func schemaType(schema map[string]any) string {
	name, _ := schema["type"].(string)
	return name
}

// describeScalar names the declared type a path tried to walk past. Only a
// declared type reaches it: a schema that declares none decides nothing.
func describeScalar(schema map[string]any) string {
	return "a " + schemaType(schema)
}

func describeWalked(walked []string) string {
	if len(walked) == 0 {
		return "the selector's type"
	}
	return strings.Join(walked, ".")
}

// suggestField names the closest declared field, so a misspelling reads as one.
func suggestField(properties map[string]any, component string) string {
	names := make([]string, 0, len(properties))
	for name := range properties {
		names = append(names, name)
	}
	sort.Strings(names)
	best, bestDistance := "", len([]rune(component))/2+1
	for _, name := range names {
		if distance := EditDistance(component, name); distance < bestDistance {
			best, bestDistance = name, distance
		}
	}
	if best == "" {
		if len(names) == 0 {
			return ""
		}
		return fmt.Sprintf("; declared fields are %s", strings.Join(names, ", "))
	}
	return fmt.Sprintf("; closest declared field is %q", best)
}

// EditDistance is the Levenshtein distance between two names. It lives here
// because both the label check and the path check suggest a near miss, and one
// implementation is easier to trust than two.
func EditDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	previous := make([]int, len(br)+1)
	current := make([]int, len(br)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		current[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			current[j] = min(min(current[j-1]+1, previous[j]+1), previous[j-1]+cost)
		}
		previous, current = current, previous
	}
	return previous[len(br)]
}
