// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

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

// ExperimentTool describes a tool that can be invoked during a state transition.
type ExperimentTool struct {
	Type      string   `yaml:"type"`
	Binary    string   `yaml:"binary"`
	FlagsFrom string   `yaml:"flags_from"`
	Propagate []string `yaml:"propagate"`
}

// ExperimentState represents a single state in the experiment state machine.
type ExperimentState struct {
	Terminal    bool                   `yaml:"terminal"`
	Transitions []ExperimentTransition `yaml:"transitions"`
}

// ExperimentTransition describes a signal→command→next_state edge.
type ExperimentTransition struct {
	Signal    string `yaml:"signal"`
	Command   string `yaml:"command"`
	NextState string `yaml:"next_state"`
}

// ExperimentConfig defines a state-machine protocol for executing a single
// evaluation point.
type ExperimentConfig struct {
	Name         string                    `yaml:"name"`
	Description  string                    `yaml:"description"`
	Tools        map[string]ExperimentTool `yaml:"tools"`
	InitialState string                    `yaml:"initial_state"`
	States       map[string]ExperimentState `yaml:"states"`
}

// LoadExperiment reads, parses, and validates an experiment YAML file.
func LoadExperiment(path string) (ExperimentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ExperimentConfig{}, fmt.Errorf("load experiment: %w", err)
	}

	var cfg ExperimentConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ExperimentConfig{}, fmt.Errorf("load experiment: parse %s: %w", path, err)
	}

	if cfg.Name == "" {
		return ExperimentConfig{}, fmt.Errorf("load experiment: name must not be empty")
	}

	if _, ok := cfg.States[cfg.InitialState]; !ok {
		return ExperimentConfig{}, fmt.Errorf(
			"load experiment: initial_state %q does not exist in states", cfg.InitialState)
	}

	for tool, def := range cfg.Tools {
		if def.Type == "cli" && def.Binary == "" {
			return ExperimentConfig{}, fmt.Errorf(
				"load experiment: cli tool %q must have binary set", tool)
		}
	}

	hasTerminal := false
	for name, state := range cfg.States {
		if state.Terminal {
			hasTerminal = true
		}
		for i, tr := range state.Transitions {
			if _, ok := cfg.Tools[tr.Command]; !ok {
				return ExperimentConfig{}, fmt.Errorf(
					"load experiment: state %q transition[%d] references unknown tool %q",
					name, i, tr.Command)
			}
			if _, ok := cfg.States[tr.NextState]; !ok {
				return ExperimentConfig{}, fmt.Errorf(
					"load experiment: state %q transition[%d] references unknown state %q",
					name, i, tr.NextState)
			}
		}
	}

	if !hasTerminal {
		return ExperimentConfig{}, fmt.Errorf("load experiment: at least one terminal state is required")
	}

	return cfg, nil
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
