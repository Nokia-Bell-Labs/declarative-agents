// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package typesys

import (
	"fmt"
	"sort"
	"strings"
)

// Registry holds every declared type addressed as unit.Name (srd051 R1.4).
type Registry struct {
	types map[string]TypeDecl
	paths map[string]string
}

// Build indexes the types of every unit, rejecting a schema outside the closed
// subset and a name its own unit declares twice. The unit segment of an
// address is a namespace: two units may each declare Text, because every
// reference names the unit it means (srd051 R1.4, R4.3).
func Build(units ...TypeUnitFile) (*Registry, error) {
	registry := &Registry{types: map[string]TypeDecl{}, paths: map[string]string{}}
	for _, unit := range sortedUnits(units) {
		for _, declared := range unit.Types {
			if err := registry.add(unit, declared); err != nil {
				return nil, err
			}
		}
	}
	return registry, nil
}

func (r *Registry) add(unit TypeUnitFile, declared TypeDecl) error {
	if declared.Name == "" {
		return fmt.Errorf("unit %q declares a type with no name", unit.Unit)
	}
	ref := unit.Ref(declared.Name)
	if _, exists := r.types[ref]; exists {
		return fmt.Errorf("unit %q declares type %q twice%s",
			unit.Unit, declared.Name, declaringFiles(r.paths[ref], unit.Path))
	}
	if err := ValidateSubset(declared.Schema); err != nil {
		return fmt.Errorf("unit %q type %q: %w", unit.Unit, declared.Name, err)
	}
	r.types[ref] = declared
	r.paths[ref] = unit.Path
	return nil
}

// declaringFiles names the two files a duplicate came from. A unit may span
// files, so which file declared the first one is the part an author cannot
// work out from the unit name alone.
func declaringFiles(first, second string) string {
	if first == "" || second == "" {
		return ""
	}
	if first == second {
		return fmt.Sprintf(" in %s", first)
	}
	return fmt.Sprintf(": %s and %s", first, second)
}

// Resolve returns the type a unit.Name reference addresses.
func (r *Registry) Resolve(ref string) (TypeDecl, bool) {
	if r == nil {
		return TypeDecl{}, false
	}
	declared, ok := r.types[ref]
	return declared, ok
}

// Refs lists every declared type address in sorted order, which is the order
// the canonical dump renders them in (srd051 R5.3).
func (r *Registry) Refs() []string {
	if r == nil {
		return nil
	}
	refs := make([]string, 0, len(r.types))
	for ref := range r.types {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return refs
}

// ResolveSchema expands every reference in a schema, recursively and at any
// depth. It returns a new schema and leaves its input untouched, so a
// declaration stays readable after resolution and repeated resolution yields
// the same result (srd051 R4.1, R4.5).
func (r *Registry) ResolveSchema(schema map[string]any) (map[string]any, error) {
	resolved, _, err := r.ResolveSchemaRefs(schema)
	return resolved, err
}

// ResolveSchemaRefs resolves a schema and reports every type it referenced,
// transitively and in sorted order. Closure usedness needs that set: a type
// unit contributes no tools, so the only evidence its import earns its place
// is a schema that reached one of its types.
func (r *Registry) ResolveSchemaRefs(schema map[string]any) (map[string]any, []string, error) {
	referenced := map[string]struct{}{}
	resolved, err := r.resolveSchemaTracking(schema, nil, referenced)
	if err != nil {
		return nil, nil, err
	}
	refs := make([]string, 0, len(referenced))
	for ref := range referenced {
		refs = append(refs, ref)
	}
	sort.Strings(refs)
	return resolved, refs, nil
}

// PathOf returns the declaration file a type came from, empty when unknown.
func (r *Registry) PathOf(ref string) string {
	if r == nil {
		return ""
	}
	return r.paths[ref]
}

func (r *Registry) resolveSchemaTracking(
	schema map[string]any, expanding []string, seen map[string]struct{},
) (map[string]any, error) {
	if schema == nil {
		return nil, nil
	}
	if ref, isRef := referenceOf(schema); isRef {
		seen[ref] = struct{}{}
		return r.expandReference(ref, expanding, seen)
	}
	resolved := make(map[string]any, len(schema))
	for key, value := range schema {
		converted, err := r.resolveValue(key, value, expanding, seen)
		if err != nil {
			return nil, err
		}
		resolved[key] = converted
	}
	return resolved, nil
}

func (r *Registry) resolveValue(
	key string, value any, expanding []string, seen map[string]struct{},
) (any, error) {
	switch key {
	case "properties":
		properties, ok := asSchema(value)
		if !ok {
			return value, nil
		}
		resolved := make(map[string]any, len(properties))
		for name, nested := range properties {
			converted, err := r.resolveNested(nested, expanding, seen)
			if err != nil {
				return nil, fmt.Errorf("properties.%s: %w", name, err)
			}
			resolved[name] = converted
		}
		return resolved, nil
	case "items":
		converted, err := r.resolveNested(value, expanding, seen)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		return converted, nil
	default:
		return cloneValue(value), nil
	}
}

func (r *Registry) resolveNested(
	value any, expanding []string, seen map[string]struct{},
) (any, error) {
	nested, ok := asSchema(value)
	if !ok {
		return cloneValue(value), nil
	}
	return r.resolveSchemaTracking(nested, expanding, seen)
}

// expandReference resolves one reference, refusing a chain that re-enters a
// reference it is already expanding. A type referring to itself is such a
// chain (srd051 R4.4).
func (r *Registry) expandReference(
	ref string, expanding []string, seen map[string]struct{},
) (map[string]any, error) {
	for _, active := range expanding {
		if active == ref {
			return nil, fmt.Errorf("type reference cycle: %s",
				strings.Join(append(append([]string{}, expanding...), ref), " -> "))
		}
	}
	declared, ok := r.Resolve(ref)
	if !ok {
		unit, name := splitRef(ref)
		return nil, fmt.Errorf("unknown type reference %q: no type %q in unit %q", ref, name, unit)
	}
	return r.resolveSchemaTracking(declared.Schema, append(append([]string{}, expanding...), ref), seen)
}

func referenceOf(schema map[string]any) (string, bool) {
	raw, present := schema[TypeRefKey]
	if !present {
		return "", false
	}
	ref, ok := raw.(string)
	return ref, ok
}

func splitRef(ref string) (unit, name string) {
	index := strings.LastIndex(ref, ".")
	if index <= 0 || index == len(ref)-1 {
		return ref, ref
	}
	return ref[:index], ref[index+1:]
}

// cloneValue deep-copies the parts of a schema resolution does not rewrite, so
// a resolved schema shares no map or slice with the declaration it came from.
func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, nested := range typed {
			cloned[key] = cloneValue(nested)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for i, nested := range typed {
			cloned[i] = cloneValue(nested)
		}
		return cloned
	default:
		return value
	}
}

func sortedUnits(units []TypeUnitFile) []TypeUnitFile {
	sorted := append([]TypeUnitFile{}, units...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Unit < sorted[j].Unit })
	return sorted
}
