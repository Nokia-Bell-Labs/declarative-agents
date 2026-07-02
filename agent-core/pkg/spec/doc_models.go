// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import "gopkg.in/yaml.v3"

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
	type plain DocSpecSources
	var structured plain
	if err := value.Decode(&structured); err == nil && (len(structured.Canonical) > 0 || len(structured.HistoricalBackground) > 0) {
		*s = DocSpecSources(structured)
		return nil
	}
	var flat []string
	if err := value.Decode(&flat); err == nil {
		s.Canonical = flat
		return nil
	}
	return nil
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
	var list []string
	if err := value.Decode(&list); err == nil {
		d.Paths = list
		return nil
	}
	var single string
	if err := value.Decode(&single); err == nil && single != "" {
		d.Paths = []string{single}
		return nil
	}
	return nil
}

// DocSpecExample is one example entry with a file path.
type DocSpecExample struct {
	File string `yaml:"file"`
}
