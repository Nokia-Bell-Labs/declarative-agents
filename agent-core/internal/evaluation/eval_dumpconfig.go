// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gopkg.in/yaml.v3"
)

const SigConfigDumped core.Signal = "ConfigDumped"

// dumpConfigCmd writes a materialized experiment.yaml into the point
// directory, capturing the full configuration used for this experiment.
type dumpConfigCmd struct {
	pc *PointContext
}

func (c *dumpConfigCmd) Name() string      { return "dump_config" }
func (c *dumpConfigCmd) Undo() core.Result { return core.NoopUndo(c.Name()) }

func (c *dumpConfigCmd) Execute() core.Result {
	pc := c.pc

	exp := experimentConfig{
		Harness: experimentHarness{
			Name:   pc.Harness.Name,
			Binary: pc.Harness.Binary,
		},
		Model:     pc.Model,
		OllamaURL: pc.OllamaURL,
		Timeout:   pc.Timeout.String(),
		Sample: experimentSample{
			Name: pc.Sample.Name,
		},
	}

	if pc.ProfilePath != "" {
		exp.Profile = pc.ProfilePath
	}

	if v, ok := pc.GridPoint["rep"]; ok {
		exp.Repetition = fmt.Sprintf("%v", v)
	}

	exp.AgentCommit = gitCommitHash()

	out, err := yaml.Marshal(exp)
	if err != nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         fmt.Errorf("marshal experiment config: %w", err),
			Output:      err.Error(),
		}
	}

	dst := filepath.Join(pc.PointDir, ArtifactExperiment)
	if err := os.WriteFile(dst, out, 0o644); err != nil {
		return core.Result{
			CommandName: c.Name(),
			Signal:      core.CommandError,
			Err:         fmt.Errorf("write experiment.yaml: %w", err),
			Output:      err.Error(),
		}
	}

	return core.Result{
		CommandName: c.Name(),
		Signal:      SigConfigDumped,
		Output:      fmt.Sprintf("experiment config written to %s", dst),
	}
}

type experimentConfig struct {
	AgentCommit string            `yaml:"agent_commit,omitempty"`
	Profile     string            `yaml:"profile,omitempty"`
	Harness     experimentHarness `yaml:"harness"`
	Model       string            `yaml:"model"`
	OllamaURL   string            `yaml:"ollama_url,omitempty"`
	Timeout     string            `yaml:"timeout,omitempty"`
	Repetition  string            `yaml:"repetition,omitempty"`
	Sample      experimentSample  `yaml:"sample"`
}

type experimentHarness struct {
	Name   string `yaml:"name"`
	Binary string `yaml:"binary"`
}

type experimentSample struct {
	Name string `yaml:"name"`
}

func gitCommitHash() string {
	cmd := exec.Command("git", "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// DumpConfigBuilder creates dumpConfigCmd instances.
type DumpConfigBuilder struct {
	ES *EvalState
}

func (b *DumpConfigBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("dump_config: EvalState.PC not initialized")}
	}
	return &dumpConfigCmd{pc: b.ES.PC}
}
