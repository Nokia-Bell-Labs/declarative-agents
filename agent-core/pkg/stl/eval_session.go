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
	Name       string           `yaml:"name"`
	Harnesses  []Harness        `yaml:"harnesses"`  // Deprecated: use Profiles
	Models     []string         `yaml:"models"`      // Deprecated: use Profiles
	Profiles   []SuiteProfile   `yaml:"-"`
	Grid       map[string][]any `yaml:"grid,omitempty"`
	SamplesDir string           `yaml:"-"`
	Samples    []Sample         `yaml:"-"`
	Timeout    time.Duration    `yaml:"-"`
	OllamaURL  string           `yaml:"-"`
	Reps       int              `yaml:"-"`
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
	suite, err := ParseSuiteConfig(data, baseDir)
	if err != nil {
		return SuiteConfig{}, err
	}
	samples, err := DiscoverSamples(suite.SamplesDir)
	if err != nil {
		return SuiteConfig{}, fmt.Errorf("suite %q: %w", suite.Name, err)
	}
	suite.Samples = samples
	return suite, nil
}

// ParseSuiteConfig parses suite YAML and validates metadata without discovering
// samples. Runtime evaluator machines compose sample discovery as a separate
// word after this parser.
//
// The suite YAML supports two formats:
//   - Profile-based (preferred): profiles: [path1, path2]
//   - Legacy: harnesses: [...] + models: [...]
func ParseSuiteConfig(data []byte, baseDir string) (SuiteConfig, error) {
	var raw struct {
		Name       string           `yaml:"name"`
		Profiles   []string         `yaml:"profiles"`
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

	samplesDir := raw.SamplesDir
	if samplesDir == "" {
		samplesDir = "samples"
	}
	if !filepath.IsAbs(samplesDir) {
		samplesDir = filepath.Join(baseDir, samplesDir)
	}

	var timeout time.Duration
	if raw.Timeout != "" {
		timeout, _ = time.ParseDuration(raw.Timeout)
	}

	suite := SuiteConfig{
		Name:       raw.Name,
		Grid:       raw.Grid,
		SamplesDir: samplesDir,
		Timeout:    timeout,
		OllamaURL:  raw.OllamaURL,
		Reps:       raw.Reps,
	}

	if len(raw.Profiles) > 0 {
		if len(raw.Harnesses) > 0 || len(raw.Models) > 0 {
			return SuiteConfig{}, fmt.Errorf("suite %q: profiles and harnesses/models are mutually exclusive", raw.Name)
		}
		profiles, err := resolveSuiteProfiles(raw.Profiles, baseDir)
		if err != nil {
			return SuiteConfig{}, fmt.Errorf("suite %q: %w", raw.Name, err)
		}
		suite.Profiles = profiles
	} else {
		if len(raw.Harnesses) == 0 {
			return SuiteConfig{}, fmt.Errorf("suite %q: missing profiles or harnesses", raw.Name)
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
		suite.Harnesses = raw.Harnesses
		suite.Models = raw.Models
	}

	return suite, nil
}

// resolveSuiteProfiles loads each profile path (relative to baseDir),
// extracts name and model, and resolves the harness binary.
func resolveSuiteProfiles(paths []string, baseDir string) ([]SuiteProfile, error) {
	var result []SuiteProfile
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		profile, err := LoadProfile(p)
		if err != nil {
			return nil, fmt.Errorf("load profile %s: %w", p, err)
		}

		sp := SuiteProfile{
			Path:    p,
			Name:    profile.Name,
			Binary:  "agent",
			Profile: profile,
		}

		sp.Model = extractModelFromProfile(profile)
		result = append(result, sp)
	}
	return result, nil
}

// extractModelFromProfile reads the model name from the first invoke_llm
// tool declaration in the profile's tool declarations and config dirs.
func extractModelFromProfile(p AgentProfile) string {
	var paths []string
	paths = append(paths, p.ToolDeclarations...)

	for _, dir := range p.ToolConfigDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
				paths = append(paths, filepath.Join(dir, e.Name()))
			}
		}
	}

	for _, path := range paths {
		defs, err := LoadToolDefs(path)
		if err != nil {
			continue
		}
		for _, td := range defs {
			if td.Init != "invoke_llm" {
				continue
			}
			var cfg LLMToolConfig
			if err := DecodeToolConfig(td, &cfg); err == nil && cfg.Model != "" {
				return cfg.Model
			}
		}
	}
	return "unknown"
}
