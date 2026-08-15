// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package evaluation

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
)

const (
	maxEvaluatorPointTimeout    = 15 * time.Minute
	maxEvaluatorSessionDuration = 24 * time.Hour
)

// SuiteConfig defines a complete evaluation suite.
type SuiteConfig struct {
	Name       string           `yaml:"name"`
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

// DiscoverSamples finds evaluation samples using the declared layout.
func DiscoverSamples(dir string, layout SampleLayout) ([]Sample, error) {
	if layout.WorkspaceDir == "" || layout.PromptFile == "" {
		return nil, fmt.Errorf("discover samples requires workspace_dir and prompt_file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("discover samples in %s: %w", dir, err)
	}

	sharedPrompt := ""
	if layout.AllowSharedPrompt {
		sharedPrompt = filepath.Join(dir, layout.PromptFile)
		if _, err := os.Stat(sharedPrompt); err != nil {
			sharedPrompt = ""
		}
	}

	var samples []Sample
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sampleDir := filepath.Join(dir, e.Name())
		workspaceDir := filepath.Join(sampleDir, layout.WorkspaceDir)

		if _, err := os.Stat(workspaceDir); err != nil {
			continue
		}

		promptPath := filepath.Join(sampleDir, layout.PromptFile)
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

		if layout.DocDir != "" {
			docDir := filepath.Join(sampleDir, layout.DocDir)
			if _, err := os.Stat(docDir); err == nil {
				sample.DocDir = docDir
			}
		}

		samples = append(samples, sample)
	}

	if len(samples) == 0 && layout.RequireSamples {
		return nil, fmt.Errorf("no valid samples found in %s", dir)
	}

	return samples, nil
}

// ParseSuite parses suite YAML and resolves samples relative to baseDir.
func ParseSuite(data []byte, baseDir string, layout SampleLayout) (SuiteConfig, error) {
	suite, err := ParseSuiteConfig(data, baseDir)
	if err != nil {
		return SuiteConfig{}, err
	}
	samples, err := DiscoverSamples(suite.SamplesDir, layout)
	if err != nil {
		return SuiteConfig{}, fmt.Errorf("suite %q: %w", suite.Name, err)
	}
	suite.Samples = samples
	return suite, nil
}

// ParseSuiteConfig parses suite YAML and validates metadata without discovering
// samples. Runtime evaluator machines compose sample discovery as a separate
// word after this parser.
func ParseSuiteConfig(data []byte, baseDir string) (SuiteConfig, error) {
	var raw struct {
		Name       string           `yaml:"name"`
		Profiles   []string         `yaml:"profiles"`
		Grid       map[string][]any `yaml:"grid,omitempty"`
		SamplesDir string           `yaml:"samples_dir"`
		Timeout    string           `yaml:"timeout,omitempty"`
		OllamaURL  string           `yaml:"ollama_url,omitempty"`
		Reps       int              `yaml:"repetitions,omitempty"`
	}

	if err := yaml.Unmarshal(data, &raw); err != nil {
		return SuiteConfig{}, fmt.Errorf("parse suite: %w", err)
	}
	if hasLegacySuiteFields(data) {
		return SuiteConfig{}, fmt.Errorf("suite %q: profile entries are required; legacy suite fields are not supported", raw.Name)
	}

	if raw.Name == "" {
		return SuiteConfig{}, fmt.Errorf("suite: missing name")
	}
	if len(raw.Profiles) == 0 {
		return SuiteConfig{}, fmt.Errorf("suite %q: missing profiles", raw.Name)
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
		parsedTimeout, err := time.ParseDuration(raw.Timeout)
		if err != nil {
			return SuiteConfig{}, fmt.Errorf("suite %q: invalid timeout %q: %w", raw.Name, raw.Timeout, err)
		}
		timeout = parsedTimeout
		if err := validateEvaluatorPointTimeout(timeout); err != nil {
			return SuiteConfig{}, fmt.Errorf("suite %q: %w", raw.Name, err)
		}
	}

	suite := SuiteConfig{
		Name:       raw.Name,
		Grid:       raw.Grid,
		SamplesDir: samplesDir,
		Timeout:    timeout,
		OllamaURL:  raw.OllamaURL,
		Reps:       raw.Reps,
	}

	profiles, err := resolveSuiteProfiles(raw.Profiles, baseDir)
	if err != nil {
		return SuiteConfig{}, fmt.Errorf("suite %q: %w", raw.Name, err)
	}
	suite.Profiles = profiles

	return suite, nil
}

func validateEvaluatorPointTimeout(timeout time.Duration) error {
	if timeout <= 0 {
		return fmt.Errorf("point timeout must be positive")
	}
	if timeout > maxEvaluatorPointTimeout {
		return fmt.Errorf("point timeout %s exceeds maximum %s", timeout, maxEvaluatorPointTimeout)
	}
	return nil
}

func hasLegacySuiteFields(data []byte) bool {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return false
	}
	_, hasFirst := raw["har"+"nesses"]
	_, hasSecond := raw["mod"+"els"]
	return hasFirst || hasSecond
}

// resolveSuiteProfiles loads each profile path (relative to baseDir),
// extracts name and model, and resolves the harness binary.
func resolveSuiteProfiles(paths []string, baseDir string) ([]SuiteProfile, error) {
	var result []SuiteProfile
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}
		profile, err := catalog.LoadProfile(p)
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
func extractModelFromProfile(p catalog.AgentProfile) string {
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
		defs, err := catalog.LoadToolDefs(path)
		if err != nil {
			continue
		}
		for _, td := range defs {
			if td.Init != "invoke_llm" {
				continue
			}
			var cfg catalog.LLMToolConfig
			if err := catalog.DecodeToolConfig(td, &cfg); err == nil && cfg.Model != "" {
				return cfg.Model
			}
		}
	}
	return "unknown"
}
