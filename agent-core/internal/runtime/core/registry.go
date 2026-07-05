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
	PhaseScoped bool
}

type registryEntry struct {
	spec    ToolSpec
	builder Builder
}

// ExternalToolAvailability classifies why an external tool can or cannot be
// used in a state-scoped manifest or dynamic dispatch path.
type ExternalToolAvailability int

const (
	ExternalToolUnknown ExternalToolAvailability = iota
	ExternalToolInternal
	ExternalToolUnavailableInState
	ExternalToolAvailable
)

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
	if err := r.RegisterChecked(spec, builder); err != nil {
		panic(err.Error())
	}
}

// RegisterChecked adds a ToolSpec-Builder pair and returns configuration
// errors instead of panicking.
func (r *Registry) RegisterChecked(spec ToolSpec, builder Builder) error {
	if r.frozen {
		return fmt.Errorf("registry: Register called after Freeze")
	}
	if spec.Name == "" {
		return fmt.Errorf("registry: ToolSpec.Name must not be empty")
	}
	if _, exists := r.entries[spec.Name]; exists {
		return fmt.Errorf("registry: duplicate tool name %q", spec.Name)
	}
	r.entries[spec.Name] = registryEntry{spec: spec, builder: builder}
	return nil
}

// Override replaces the builder (and optionally the spec) for an
// existing entry, or inserts a new one if absent.
func (r *Registry) Override(spec ToolSpec, builder Builder) {
	if err := r.OverrideChecked(spec, builder); err != nil {
		panic(err.Error())
	}
}

// OverrideChecked replaces the builder (and optionally the spec) for an
// existing entry, or inserts a new one if absent. It returns configuration
// errors instead of panicking.
func (r *Registry) OverrideChecked(spec ToolSpec, builder Builder) error {
	if r.frozen {
		return fmt.Errorf("registry: Override called after Freeze")
	}
	if spec.Name == "" {
		return fmt.Errorf("registry: ToolSpec.Name must not be empty")
	}
	r.entries[spec.Name] = registryEntry{spec: spec, builder: builder}
	return nil
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

// ResolveExternalTool returns the ToolSpec, Builder, and availability status for
// an LLM-visible tool in a state. It is the shared rule behind manifests,
// parse-time validation, and dynamic $tool dispatch.
func (r *Registry) ResolveExternalTool(name string, state State) (ToolSpec, Builder, ExternalToolAvailability) {
	e, ok := r.entries[name]
	if !ok {
		return ToolSpec{}, nil, ExternalToolUnknown
	}
	if e.spec.Visibility != External {
		return e.spec, e.builder, ExternalToolInternal
	}
	if !e.spec.AvailableIn(state) {
		return e.spec, e.builder, ExternalToolUnavailableInState
	}
	return e.spec, e.builder, ExternalToolAvailable
}

// Manifest returns a copy of all External ToolSpecs available in the
// given state.
func (r *Registry) Manifest(state State) []ToolSpec {
	var out []ToolSpec
	for name := range r.entries {
		spec, _, availability := r.ResolveExternalTool(name, state)
		if availability != ExternalToolAvailable {
			continue
		}
		out = append(out, spec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// AvailableExternalToolNames returns the manifest-visible tool names in state.
func (r *Registry) AvailableExternalToolNames(state State) []string {
	manifest := r.Manifest(state)
	names := make([]string, 0, len(manifest))
	for _, spec := range manifest {
		names = append(names, spec.Name)
	}
	return names
}

// AvailableIn reports whether a ToolSpec is available in a state-scoped
// manifest. Unscoped empty Phases preserve the legacy "all states" behavior.
func (ts ToolSpec) AvailableIn(state State) bool {
	if len(ts.Phases) == 0 {
		return !ts.PhaseScoped
	}
	return PhaseMatch(ts.Phases, state)
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
