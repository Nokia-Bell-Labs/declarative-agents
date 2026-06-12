// Copyright (c) 2026 Nokia. All rights reserved.

package prompt

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// rawPromptFile mirrors the YAML schema. Both structured and simple
// fields coexist; mode detection picks which branch applies.
type rawPromptFile struct {
	Prompt       *string `yaml:"prompt"`
	Task         *string `yaml:"task"`
	Role         *string `yaml:"role"`
	Constraints  *string `yaml:"constraints"`
	OutputFormat *string `yaml:"output_format"`
}

// LoadResult bundles a loaded Prompt with metadata about the load
// operation. Callers can trace these values after telemetry is
// initialized.
type LoadResult struct {
	Prompt   Prompt
	Mode     string
	FileSize int
	TaskLen  int
	Sections int
}

// LoadPrompt reads a YAML prompt file, detects simple vs. structured
// mode, maps fields to a Prompt, applies compiled defaults, and
// validates that Task is non-empty. It returns a fully populated
// LoadResult or an error; never a partial struct.
func LoadPrompt(path string) (LoadResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LoadResult{}, fmt.Errorf("read prompt file %s: %w", path, err)
	}

	var raw rawPromptFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return LoadResult{}, fmt.Errorf("parse prompt YAML %s: %w", path, err)
	}

	hasTask := raw.Task != nil
	hasPrompt := raw.Prompt != nil

	if hasTask && hasPrompt {
		return LoadResult{}, fmt.Errorf(
			"prompt file %s: both \"task\" and \"prompt\" fields present; use one or the other",
			path,
		)
	}

	var p Prompt
	var mode string
	if hasTask {
		p = structuredMode(raw)
		mode = "structured"
	} else if hasPrompt {
		p = simpleMode(*raw.Prompt)
		mode = "simple"
	} else {
		return LoadResult{}, fmt.Errorf(
			"prompt file %s: missing \"task\" or \"prompt\" field", path,
		)
	}

	if strings.TrimSpace(p.Task) == "" {
		return LoadResult{}, fmt.Errorf(
			"prompt file %s: task is empty after loading", path,
		)
	}

	return LoadResult{
		Prompt:   p,
		Mode:     mode,
		FileSize: len(data),
		TaskLen:  len(p.Task),
		Sections: p.SectionCount(),
	}, nil
}

// LoadPromptFromString creates a LoadResult from a raw task string,
// using simple mode with compiled defaults. This supports inline
// prompt specification as an alternative to loading from a YAML file.
func LoadPromptFromString(task string) (LoadResult, error) {
	trimmed := strings.TrimSpace(task)
	if trimmed == "" {
		return LoadResult{}, fmt.Errorf("prompt string is empty")
	}

	p := simpleMode(trimmed)
	return LoadResult{
		Prompt:   p,
		Mode:     "string",
		FileSize: len(task),
		TaskLen:  len(trimmed),
		Sections: p.SectionCount(),
	}, nil
}

func structuredMode(raw rawPromptFile) Prompt {
	p := Prompt{
		Role:         DefaultRole,
		Constraints:  DefaultConstraints,
		OutputFormat: DefaultOutputFormat,
	}
	if raw.Task != nil {
		p.Task = *raw.Task
	}
	if raw.Role != nil {
		p.Role = *raw.Role
	}
	if raw.Constraints != nil {
		p.Constraints = *raw.Constraints
	}
	if raw.OutputFormat != nil {
		p.OutputFormat = *raw.OutputFormat
	}
	return p
}

func simpleMode(promptValue string) Prompt {
	return Prompt{
		Role:         DefaultRole,
		Task:         promptValue,
		Constraints:  DefaultConstraints,
		OutputFormat: DefaultOutputFormat,
	}
}
