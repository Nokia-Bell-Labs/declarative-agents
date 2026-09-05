// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package core

import "gopkg.in/yaml.v3"

// StateSpec describes a state and optional semantic metadata.
type StateSpec struct {
	Name      string    `yaml:"name"`
	Meaning   string    `yaml:"meaning,omitempty"`
	RunStatus RunStatus `yaml:"run_status,omitempty"`
	Tags      []string  `yaml:"tags,omitempty"`
}

// StateSpecs accepts both legacy scalar state lists and rich state objects.
type StateSpecs []StateSpec

func (s *StateSpecs) UnmarshalYAML(value *yaml.Node) error {
	specs, err := unmarshalNamedSpecs[StateSpec](value, "state")
	if err != nil {
		return err
	}
	*s = specs
	return nil
}

func (s StateSpecs) Names() []string {
	names := make([]string, 0, len(s))
	for _, spec := range s {
		names = append(names, spec.Name)
	}
	return names
}

func StateSpecsFromNames(names ...string) StateSpecs {
	specs := make(StateSpecs, 0, len(names))
	for _, name := range names {
		specs = append(specs, StateSpec{Name: name})
	}
	return specs
}
