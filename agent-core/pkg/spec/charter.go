// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var supportedCharterKinds = map[string]bool{
	"grep_check":        true,
	"ref_check":         true,
	"consistency_check": true,
	"spec_corpus":       true,
}

// Charter is a deterministic jurist check suite loaded from YAML.
type Charter struct {
	ID     string         `yaml:"id" json:"id"`
	Title  string         `yaml:"title,omitempty" json:"title,omitempty"`
	Target CharterTarget  `yaml:"target,omitempty" json:"target,omitempty"`
	Checks []CharterCheck `yaml:"checks" json:"checks"`
	Path   string         `yaml:"-" json:"path,omitempty"`
}

type CharterTarget struct {
	Root    string   `yaml:"root,omitempty" json:"root,omitempty"`
	Include []string `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude []string `yaml:"exclude,omitempty" json:"exclude,omitempty"`
}

type CharterCheck struct {
	ID       string         `yaml:"id" json:"id"`
	Kind     string         `yaml:"kind" json:"kind"`
	Severity string         `yaml:"severity,omitempty" json:"severity,omitempty"`
	Message  string         `yaml:"message,omitempty" json:"message,omitempty"`
	Include  []string       `yaml:"include,omitempty" json:"include,omitempty"`
	Exclude  []string       `yaml:"exclude,omitempty" json:"exclude,omitempty"`
	Patterns []string       `yaml:"patterns,omitempty" json:"patterns,omitempty"`
	Mode     string         `yaml:"mode,omitempty" json:"mode,omitempty"`
	Regex    bool           `yaml:"regex,omitempty" json:"regex,omitempty"`
	Refs     map[string]any `yaml:"references,omitempty" json:"references,omitempty"`
	Extract  map[string]any `yaml:"extract,omitempty" json:"extract,omitempty"`
	Source   map[string]any `yaml:"source,omitempty" json:"source,omitempty"`
	Rule     string         `yaml:"rule,omitempty" json:"rule,omitempty"`
	Target   map[string]any `yaml:"target,omitempty" json:"target,omitempty"`
	Checks   []string       `yaml:"checks,omitempty" json:"checks,omitempty"`
}

// LoadCharters parses explicit jurist charter suite paths in deterministic order.
func LoadCharters(paths []string) ([]Charter, error) {
	paths = normalizedCharterPaths(paths)
	charters := make([]Charter, 0, len(paths))
	seenSuites := make(map[string]string, len(paths))
	for _, path := range paths {
		charter, err := LoadCharter(path)
		if err != nil {
			return nil, err
		}
		if prior, ok := seenSuites[charter.ID]; ok {
			return nil, fmt.Errorf("duplicate charter suite id %q in %s and %s", charter.ID, prior, path)
		}
		seenSuites[charter.ID] = path
		charters = append(charters, charter)
	}
	return charters, nil
}

// LoadCharter parses one jurist charter suite from disk.
func LoadCharter(path string) (Charter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Charter{}, fmt.Errorf("read charter %s: %w", path, err)
	}
	charter, err := ParseCharter(data)
	if err != nil {
		return Charter{}, fmt.Errorf("parse charter %s: %w", path, err)
	}
	charter.Path = path
	return charter, nil
}

// ParseCharter parses and validates one jurist charter suite.
func ParseCharter(data []byte) (Charter, error) {
	var charter Charter
	if err := yaml.Unmarshal(data, &charter); err != nil {
		return Charter{}, fmt.Errorf("invalid YAML: %w", err)
	}
	if err := validateCharter(&charter); err != nil {
		return Charter{}, err
	}
	return charter, nil
}

func normalizedCharterPaths(paths []string) []string {
	result := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			result = append(result, path)
		}
	}
	sort.Strings(result)
	return result
}

func validateCharter(charter *Charter) error {
	if charter.ID == "" {
		return fmt.Errorf("charter: missing id")
	}
	if len(charter.Checks) == 0 {
		return fmt.Errorf("charter %q: missing checks", charter.ID)
	}
	seenChecks := make(map[string]bool, len(charter.Checks))
	for i := range charter.Checks {
		check := &charter.Checks[i]
		if check.ID == "" {
			return fmt.Errorf("charter %q: check %d missing id", charter.ID, i+1)
		}
		if seenChecks[check.ID] {
			return fmt.Errorf("charter %q: duplicate check id %q", charter.ID, check.ID)
		}
		seenChecks[check.ID] = true
		if !supportedCharterKinds[check.Kind] {
			return fmt.Errorf("charter %q check %q: unknown check kind %q", charter.ID, check.ID, check.Kind)
		}
		if check.Severity == "" {
			check.Severity = "error"
		}
		switch check.Severity {
		case "error", "warning":
		default:
			return fmt.Errorf("charter %q check %q: unknown severity %q", charter.ID, check.ID, check.Severity)
		}
	}
	return nil
}
