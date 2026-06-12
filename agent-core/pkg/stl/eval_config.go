// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import "fmt"

// Sample represents a discovered evaluation sample.
type Sample struct {
	Name         string
	PromptPath   string
	DocDir       string
	WorkspaceDir string
}

// Harness defines a harness binary and its flag template.
// Deprecated: use SuiteProfile with --profile instead.
type Harness struct {
	Name    string                 `yaml:"name"`
	Binary  string                 `yaml:"binary"`
	Module  string                 `yaml:"module"`
	Version string                 `yaml:"version"`
	Flags   map[string]interface{} `yaml:"flags"`
}

// SuiteProfile is a resolved profile entry in a suite configuration.
// It bundles the profile path with derived metadata for labeling.
type SuiteProfile struct {
	Path    string `yaml:"path"`
	Name    string `yaml:"-"`
	Model   string `yaml:"-"`
	Binary  string `yaml:"-"`
	Profile AgentProfile `yaml:"-"`
}

// GridPoint is a single point in the parameter space.
type GridPoint map[string]interface{}

// FormatGridPoint returns a stable string representation for directory naming.
func FormatGridPoint(point GridPoint) string {
	if len(point) == 0 {
		return ""
	}
	names := sortedKeys(point)
	s := ""
	for _, name := range names {
		if s != "" {
			s += "_"
		}
		s += fmt.Sprintf("%s=%v", name, point[name])
	}
	return s
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
