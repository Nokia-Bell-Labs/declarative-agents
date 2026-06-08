// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import "fmt"

// Sample represents a discovered evaluation sample.
type Sample struct {
	Name         string
	PromptPath   string
	DocDir       string
	WorkspaceDir string
}

// Harness defines a harness binary and its flag template.
type Harness struct {
	Name    string            `yaml:"name"`
	Binary  string            `yaml:"binary"`
	Module  string            `yaml:"module"`
	Version string            `yaml:"version"`
	Flags   map[string]string `yaml:"flags"`
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
