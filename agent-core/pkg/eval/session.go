// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
	"gopkg.in/yaml.v3"
)

// SessionConfig holds the configuration for an evaluation session.
type SessionConfig struct {
	OutputDir   string
	OllamaURL  string
	LLMTimeout time.Duration
	Timeout    time.Duration
	Reps       int
	Stderr     io.Writer
}

// SuiteConfig defines a complete evaluation suite.
type SuiteConfig struct {
	Name       string            `yaml:"name"`
	Harnesses  []Harness         `yaml:"harnesses"`
	Models     []string          `yaml:"models"`
	Grid       map[string][]any  `yaml:"grid,omitempty"`
	Samples    []Sample          `yaml:"-"`
	Timeout    time.Duration     `yaml:"-"`
	OllamaURL  string            `yaml:"-"`
	Reps       int               `yaml:"-"`
}

// SessionResult holds the outcome of an evaluation session.
type SessionResult struct {
	TotalPoints  int
	Passed       int
	Failed       int
	TimedOut     int
	Duration     time.Duration
	Points       []PointResult
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

// StandardSessionConfig extends SessionConfig with standard infrastructure
// references for running evaluation points through core.Loop.
type StandardSessionConfig struct {
	SessionConfig

	// LoopParams is a template for each evaluation point's loop.
	// The function clones and adjusts per-point fields (Budget, AgentName)
	// before each core.Loop call.
	LoopParams core.LoopParams

	// ES is the shared eval state whose PC field is updated before
	// each point's loop run.
	ES *EvalState
}

// RunSession executes a full evaluation session using the standard
// core.Loop infrastructure. Each evaluation point runs through the
// machine.yaml-defined state machine with standard tool resolution.
func RunSession(
	ctx context.Context,
	suite SuiteConfig,
	cfg StandardSessionConfig,
) (*SessionResult, error) {
	if cfg.Stderr == nil {
		cfg.Stderr = os.Stderr
	}

	sessionDir := filepath.Join(cfg.OutputDir, suite.Name, time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	gridPoints := expandGrid(suite.Grid)
	if len(gridPoints) == 0 {
		gridPoints = []GridPoint{{}}
	}

	reps := cfg.Reps
	if reps < 1 {
		reps = 1
	}

	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	start := time.Now()
	var result SessionResult

	for _, harness := range suite.Harnesses {
		for _, model := range suite.Models {
			for _, gp := range gridPoints {
				for _, sample := range suite.Samples {
					for rep := 0; rep < reps; rep++ {
						if ctx.Err() != nil {
							return &result, ctx.Err()
						}

						pointID := EvalPointID(sample.Name, harness.Name, model, gp, rep)

						pc := &PointContext{
							SessionDir: sessionDir,
							PointID:    pointID,
							Sample:     sample,
							Harness:    harness,
							Model:      model,
							GridPoint:  gp,
							Rep:        rep,
							Timeout:    timeout,
							LLMTimeout: cfg.LLMTimeout,
							OllamaURL:  cfg.OllamaURL,
							Stderr:     cfg.Stderr,
						}

						cfg.ES.PC = pc

						fmt.Fprintf(cfg.Stderr, "  → %s\n", pointID)

						params := cfg.LoopParams
						params.AgentName = "evaluator-point"

						_, loopErr := core.Loop(params, ctx)
						if loopErr != nil {
							fmt.Fprintf(cfg.Stderr, "    ERROR: %v\n", loopErr)
						}

						pr := PointResult{
							PointID:     pointID,
							Sample:      sample.Name,
							Harness:     harness.Name,
							Model:       model,
							TestsPassed: pc.TestsPassed,
							TimedOut:    pc.TimedOut,
							ExitCode:    pc.ExitCode,
							Tokens:      pc.Tokens,
							Duration:    pc.Duration,
						}

						result.Points = append(result.Points, pr)
						result.TotalPoints++
						if pc.TestsPassed {
							result.Passed++
						} else if pc.TimedOut {
							result.TimedOut++
						} else {
							result.Failed++
						}

						status := "PASS"
						if pc.TimedOut {
							status = "TIMEOUT"
						} else if !pc.TestsPassed {
							status = "FAIL"
						}
						fmt.Fprintf(cfg.Stderr, "    %s (exit=%d tokens=%d %s)\n",
							status, pc.ExitCode, pc.Tokens, pc.Duration.Round(time.Second))
					}
				}
			}
		}
	}

	result.Duration = time.Since(start)
	fmt.Fprintf(cfg.Stderr, "\nSession complete: %d/%d passed (%d timed out) in %s\n",
		result.Passed, result.TotalPoints, result.TimedOut, result.Duration.Round(time.Second))

	return &result, nil
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
		Name        string            `yaml:"name"`
		Harnesses   []Harness         `yaml:"harnesses"`
		Models      []string          `yaml:"models"`
		Grid        map[string][]any  `yaml:"grid,omitempty"`
		SamplesDir  string            `yaml:"samples_dir"`
		Timeout     string            `yaml:"timeout,omitempty"`
		OllamaURL   string            `yaml:"ollama_url,omitempty"`
		Reps        int               `yaml:"repetitions,omitempty"`
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

// InstallHarness installs a harness binary using go install.
func InstallHarness(ctx context.Context, h Harness) error {
	if h.Module == "" {
		return nil
	}

	installPath := h.Module
	if h.Version != "" {
		installPath = h.Module + "@" + h.Version
	}

	args := []string{"install", installPath}
	cmd := exec.Command("go", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("install harness %s: %s: %w", h.Name, strings.TrimSpace(string(out)), err)
	}
	return nil
}
