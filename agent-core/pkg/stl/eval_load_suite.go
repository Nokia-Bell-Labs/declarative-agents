// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"time"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

// LoadSuiteBuilder creates loadSuiteCmd instances.
type LoadSuiteBuilder struct {
	ES *EvalSessionState
}

func (b *LoadSuiteBuilder) Build(_ core.Result) core.Command {
	return &loadSuiteCmd{es: b.ES}
}

type loadSuiteCmd struct {
	es *EvalSessionState
}

func (c *loadSuiteCmd) Name() string { return "load_suite" }

func (c *loadSuiteCmd) Execute() core.Result {
	if c.es.SuitePath == "" {
		return core.Result{
			Signal:      core.CommandError,
			Err:         fmt.Errorf("load_suite: no suite path configured"),
			Output:      "no suite path configured",
			CommandName: "load_suite",
		}
	}

	suite, err := LoadSuite(c.es.SuitePath)
	if err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("load suite: %v", err),
			CommandName: "load_suite",
		}
	}

	c.es.Suite = suite

	reps := c.es.Reps
	if reps == 0 && suite.Reps > 0 {
		reps = suite.Reps
	}

	timeout := c.es.Timeout
	if timeout == 0 && suite.Timeout > 0 {
		timeout = suite.Timeout
	}

	ollamaURL := c.es.OllamaURL
	if ollamaURL == "" && suite.OllamaURL != "" {
		ollamaURL = suite.OllamaURL
	}

	if err := c.es.InitSession(c.es.OutputDir, reps, timeout, ollamaURL, 0); err != nil {
		return core.Result{
			Signal:      core.CommandError,
			Err:         err,
			Output:      fmt.Sprintf("init session: %v", err),
			CommandName: "load_suite",
		}
	}

	total := len(suite.Harnesses) * len(suite.Models) * len(c.es.gridPoints) * len(suite.Samples) * c.es.reps
	fmt.Fprintf(c.es.Stderr, "Suite %q: %d harnesses × %d models × %d samples × %d reps = %d points\n",
		suite.Name, len(suite.Harnesses), len(suite.Models), len(suite.Samples), c.es.reps, total)

	return core.Result{
		Signal:      SigSuiteLoaded,
		Output:      fmt.Sprintf("loaded suite %q with %d points", suite.Name, total),
		CommandName: "load_suite",
	}
}

// LoadSuiteFactory creates a BuiltinFactory for load_suite.
// Config keys: input, output_dir, reps, timeout, ollama_url.
func LoadSuiteFactory(es *EvalSessionState) BuiltinFactory {
	return func(def ToolDef, vars map[string]string) (core.Builder, error) {
		if es.SuitePath == "" {
			if v, ok := def.Config["input"].(string); ok && v != "" {
				es.SuitePath = v
			}
		}
		if es.OutputDir == "" {
			if v, ok := def.Config["output_dir"].(string); ok && v != "" {
				es.OutputDir = v
			}
		}
		if es.OutputDir == "" {
			es.OutputDir = "eval-results"
		}
		if es.Reps == 0 {
			if v := configIntFromMap(def.Config, "reps"); v > 0 {
				es.Reps = v
			}
		}
		if es.Timeout == 0 {
			if v := configIntFromMap(def.Config, "timeout"); v > 0 {
				es.Timeout = time.Duration(v) * time.Second
			}
		}
		if es.OllamaURL == "" {
			if v, ok := def.Config["ollama_url"].(string); ok && v != "" {
				es.OllamaURL = v
			}
		}
		return &LoadSuiteBuilder{ES: es}, nil
	}
}

func configIntFromMap(m map[string]interface{}, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	default:
		return 0
	}
}
