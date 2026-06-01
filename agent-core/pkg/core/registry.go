// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Visibility controls whether a tool appears in the LLM manifest.
type Visibility int

const (
	External Visibility = iota
	Internal
)

// ToolSpec carries LLM-facing metadata for one tool.
type ToolSpec struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Visibility  Visibility
	Phases      []State
}

type registryEntry struct {
	spec    ToolSpec
	builder Builder
}

// Registry pairs ToolSpecs with their Builders and supports
// state-filtered manifest generation.
type Registry struct {
	entries map[string]registryEntry
	frozen  bool
}

// NewRegistry creates an empty, mutable Registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string]registryEntry)}
}

// Register adds a ToolSpec-Builder pair. Name must be non-empty and
// unique; duplicates panic. Must not be called after Freeze.
func (r *Registry) Register(spec ToolSpec, builder Builder) {
	if r.frozen {
		panic("registry: Register called after Freeze")
	}
	if spec.Name == "" {
		panic("registry: ToolSpec.Name must not be empty")
	}
	if _, exists := r.entries[spec.Name]; exists {
		panic(fmt.Sprintf("registry: duplicate tool name %q", spec.Name))
	}
	r.entries[spec.Name] = registryEntry{spec: spec, builder: builder}
}

// Freeze marks the registry as immutable.
func (r *Registry) Freeze() {
	r.frozen = true
}

// Resolve returns the Builder registered under name and true, or
// zero-value and false if absent.
func (r *Registry) Resolve(name string) (Builder, bool) {
	e, ok := r.entries[name]
	if !ok {
		return nil, false
	}
	return e.builder, true
}

// SpecByName returns the ToolSpec registered under name.
func (r *Registry) SpecByName(name string) (ToolSpec, bool) {
	e, ok := r.entries[name]
	if !ok {
		return ToolSpec{}, false
	}
	return e.spec, true
}

// Manifest returns a copy of all External ToolSpecs available in the
// given state.
func (r *Registry) Manifest(state State) []ToolSpec {
	var out []ToolSpec
	for _, e := range r.entries {
		if e.spec.Visibility != External {
			continue
		}
		if !PhaseMatch(e.spec.Phases, state) {
			continue
		}
		out = append(out, e.spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AllToolNames returns every registered tool name sorted alphabetically.
func (r *Registry) AllToolNames() []string {
	names := make([]string, 0, len(r.entries))
	for n := range r.entries {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ExternalToolNames returns only External tool names sorted alphabetically.
func (r *Registry) ExternalToolNames() []string {
	var names []string
	for _, e := range r.entries {
		if e.spec.Visibility == External {
			names = append(names, e.spec.Name)
		}
	}
	sort.Strings(names)
	return names
}

// PhaseMatch returns true if the state matches any phase, or if
// phases is empty (matches all states).
func PhaseMatch(phases []State, state State) bool {
	if len(phases) == 0 {
		return true
	}
	for _, p := range phases {
		if p == state {
			return true
		}
	}
	return false
}

var _ CommandResolver = (*Registry)(nil)
