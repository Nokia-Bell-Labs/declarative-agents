// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// SuiteConfig defines a complete evaluation suite.
type SuiteConfig struct {
	Name      string           `yaml:"name"`
	Harnesses []Harness        `yaml:"harnesses"`
	Models    []string         `yaml:"models"`
	Grid      map[string][]any `yaml:"grid,omitempty"`
	Samples   []Sample         `yaml:"-"`
	Timeout   time.Duration    `yaml:"-"`
	OllamaURL string           `yaml:"-"`
	Reps      int              `yaml:"-"`
}

// SessionResult holds the outcome of an evaluation session.
type SessionResult struct {
	TotalPoints int
	Passed      int
	Failed      int
	TimedOut    int
	Duration    time.Duration
	Points      []PointResult
}

// PointResult captures the result of a single evaluation point.
type PointResult struct {
	PointID     string
	Sample      string
	Harness     string
	Model       string
	TestsPassed bool
	TimedOut    bool
	ExitCode    int
	Tokens      int
	Duration    time.Duration
}

// expandGrid generates all combinations of grid parameters.
func expandGrid(grid map[string][]any) []GridPoint {
	if len(grid) == 0 {
		return nil
	}

	keys := sortedStringKeys(grid)
	return cartesian(keys, grid, 0, GridPoint{})
}

func cartesian(keys []string, grid map[string][]any, idx int, current GridPoint) []GridPoint {
	if idx >= len(keys) {
		cp := make(GridPoint, len(current))
		for k, v := range current {
			cp[k] = v
		}
		return []GridPoint{cp}
	}

	key := keys[idx]
	var result []GridPoint
	for _, val := range grid[key] {
		current[key] = val
		result = append(result, cartesian(keys, grid, idx+1, current)...)
	}
	return result
}

func sortedStringKeys(m map[string][]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sortStrings(keys)
	return keys
}

// DiscoverSamples finds evaluation samples in the given directory.
// Each sample is a subdirectory containing a prompt.yaml and a workspace/ dir.
func DiscoverSamples(dir string) ([]Sample, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("discover samples in %s: %w", dir, err)
	}

	// Check for a shared prompt.yaml at the samples root level.
	sharedPrompt := filepath.Join(dir, "prompt.yaml")
	if _, err := os.Stat(sharedPrompt); err != nil {
		sharedPrompt = ""
	}

	var samples []Sample
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sampleDir := filepath.Join(dir, e.Name())
		workspaceDir := filepath.Join(sampleDir, "workspace")

		if _, err := os.Stat(workspaceDir); err != nil {
			continue
		}

		promptPath := filepath.Join(sampleDir, "prompt.yaml")
		if _, err := os.Stat(promptPath); err != nil {
			if sharedPrompt != "" {
				promptPath = sharedPrompt
			} else {
				continue
			}
		}

		sample := Sample{
			Name:         e.Name(),
			PromptPath:   promptPath,
			WorkspaceDir: workspaceDir,
		}

		docDir := filepath.Join(sampleDir, "doc")
		if _, err := os.Stat(docDir); err == nil {
			sample.DocDir = docDir
		}

		samples = append(samples, sample)
	}

	if len(samples) == 0 {
		return nil, fmt.Errorf("no valid samples found in %s", dir)
	}

	return samples, nil
}

// LoadSuite reads a suite YAML file and resolves its samples.
func LoadSuite(path string) (SuiteConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SuiteConfig{}, fmt.Errorf("read suite: %w", err)
	}

	return ParseSuite(data, filepath.Dir(path))
}

// ParseSuite parses suite YAML and resolves samples relative to baseDir.
func ParseSuite(data []byte, baseDir string) (SuiteConfig, error) {
	var raw struct {
		Name       string           `yaml:"name"`
		Harnesses  []Harness        `yaml:"harnesses"`
		Models     []string         `yaml:"models"`
		Grid       map[string][]any `yaml:"grid,omitempty"`
		SamplesDir string           `yaml:"samples_dir"`
		Timeout    string           `yaml:"timeout,omitempty"`
		OllamaURL  string           `yaml:"ollama_url,omitempty"`
		Reps       int              `yaml:"repetitions,omitempty"`
	}

	if err := yaml.Unmarshal(data, &raw); err != nil {
		return SuiteConfig{}, fmt.Errorf("parse suite: %w", err)
	}

	if raw.Name == "" {
		return SuiteConfig{}, fmt.Errorf("suite: missing name")
	}
	if len(raw.Harnesses) == 0 {
		return SuiteConfig{}, fmt.Errorf("suite %q: missing harnesses", raw.Name)
	}
	for i, h := range raw.Harnesses {
		if h.Name == "" {
			return SuiteConfig{}, fmt.Errorf("suite %q: harness[%d]: missing name", raw.Name, i)
		}
		if h.Binary == "" {
			return SuiteConfig{}, fmt.Errorf("suite %q: harness %q: missing binary", raw.Name, h.Name)
		}
	}
	if len(raw.Models) == 0 {
		return SuiteConfig{}, fmt.Errorf("suite %q: missing models", raw.Name)
	}

	samplesDir := raw.SamplesDir
	if samplesDir == "" {
		samplesDir = "samples"
	}
	if !filepath.IsAbs(samplesDir) {
		samplesDir = filepath.Join(baseDir, samplesDir)
	}

	samples, err := DiscoverSamples(samplesDir)
	if err != nil {
		return SuiteConfig{}, fmt.Errorf("suite %q: %w", raw.Name, err)
	}

	var timeout time.Duration
	if raw.Timeout != "" {
		timeout, _ = time.ParseDuration(raw.Timeout)
	}

	return SuiteConfig{
		Name:      raw.Name,
		Harnesses: raw.Harnesses,
		Models:    raw.Models,
		Grid:      raw.Grid,
		Samples:   samples,
		Timeout:   timeout,
		OllamaURL: raw.OllamaURL,
		Reps:      raw.Reps,
	}, nil
}
