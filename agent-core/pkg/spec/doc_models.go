// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// DocSpec represents a parsed semantic-model or config-format YAML spec.
// It captures only the fields needed for cross-reference validation.
type DocSpec struct {
	ID                 string           `yaml:"id"`
	Title              string           `yaml:"title"`
	RequirementsSource DocSpecSources   `yaml:"requirements_source,omitempty"`
	RelatedDocuments   []string         `yaml:"related_documents,omitempty"`
	Implementation     DocSpecImpl      `yaml:"implementation,omitempty"`
	Examples           []DocSpecExample `yaml:"examples,omitempty"`
	SourceFile         string           `yaml:"-"`
}

// DocSpecSources handles both flat list and canonical/historical forms.
type DocSpecSources struct {
	Canonical            []string `yaml:"canonical,omitempty"`
	HistoricalBackground []string `yaml:"historical_background,omitempty"`
}

func (s *DocSpecSources) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.MappingNode:
		type plain DocSpecSources
		var structured plain
		if err := value.Decode(&structured); err != nil {
			return err
		}
		*s = DocSpecSources(structured)
		return nil
	case yaml.SequenceNode:
		var flat []string
		if err := value.Decode(&flat); err != nil {
			return err
		}
		s.Canonical = flat
		return nil
	case 0:
		return nil
	default:
		return fmt.Errorf("requirements_source must be a mapping or list")
	}
}

// AllPaths returns all canonical and historical source paths.
func (s *DocSpecSources) AllPaths() []string {
	return append(append([]string(nil), s.Canonical...), s.HistoricalBackground...)
}

// DocSpecImpl handles implementation as either a single string or list.
type DocSpecImpl struct {
	Paths []string
}

func (d *DocSpecImpl) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return err
		}
		d.Paths = list
		return nil
	case yaml.ScalarNode:
		var single string
		if err := value.Decode(&single); err != nil {
			return err
		}
		if single == "" {
			return fmt.Errorf("implementation path must not be empty")
		}
		d.Paths = []string{single}
		return nil
	case 0:
		return nil
	default:
		return fmt.Errorf("implementation must be a string or list")
	}
}

// DocSpecExample is one example entry with a file path.
type DocSpecExample struct {
	File string `yaml:"file"`
}
