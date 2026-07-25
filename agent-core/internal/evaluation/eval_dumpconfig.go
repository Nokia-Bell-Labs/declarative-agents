// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"gopkg.in/yaml.v3"
)

const (
	SigAgentCommitRecorded core.Signal = "AgentCommitRecorded"
	SigConfigDumped        core.Signal = "ConfigDumped"
)

type recordAgentCommitCmd struct {
	pc    *PointContext
	prior core.Result
}

func (c *recordAgentCommitCmd) Name() string { return "record_agent_commit" }
func (c *recordAgentCommitCmd) Undo(prior core.Result) core.Result {
	return (&evaluatorReceiptCmd{inner: c, point: c.pc}).Undo(prior)
}

func (c *recordAgentCommitCmd) Execute() core.Result {
	commit := ""
	if c.prior.Signal == core.ToolDone {
		commit = strings.TrimSpace(c.prior.Output)
	}
	if commit == "" {
		commit = "unknown"
	}
	c.pc.AgentCommit = commit
	return core.Result{
		CommandName: c.Name(),
		Signal:      SigAgentCommitRecorded,
		Output:      commit,
	}
}

// dumpConfigCmd writes a materialized experiment.yaml into the point
// directory, capturing the full configuration used for this experiment.
type dumpConfigCmd struct {
	pc *PointContext
}

func (c *dumpConfigCmd) Name() string                   { return "dump_config" }
func (c *dumpConfigCmd) Undo(_ core.Result) core.Result { return core.NoopUndo(c.Name()) }

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

	exp.AgentCommit = pc.AgentCommit

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

// DumpConfigBuilder creates dumpConfigCmd instances.
type DumpConfigBuilder struct {
	ES *EvalState
}

func (b *DumpConfigBuilder) Build(_ core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("dump_config: EvalState.PC not initialized")}
	}
	pc := b.ES.PC
	return &evaluatorReceiptCmd{
		inner: &dumpConfigCmd{pc: pc}, point: pc,
		removePaths: func() []string { return []string{filepath.Join(pc.PointDir, ArtifactExperiment)} },
	}
}

func (b *DumpConfigBuilder) BuildReverser() core.Command {
	return buildPointCommand(b.ES, "dump_config", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{inner: &dumpConfigCmd{pc: pc}, point: pc}
	})
}

// RecordAgentCommitBuilder maps a configured rev_parse result into point state.
type RecordAgentCommitBuilder struct {
	ES *EvalState
}

func (b *RecordAgentCommitBuilder) Build(res core.Result) core.Command {
	if b.ES == nil || b.ES.PC == nil {
		return &failCmd{err: fmt.Errorf("record_agent_commit: EvalState.PC not initialized")}
	}
	return &evaluatorReceiptCmd{inner: &recordAgentCommitCmd{pc: b.ES.PC, prior: res}, point: b.ES.PC}
}

func (b *RecordAgentCommitBuilder) BuildReverser() core.Command {
	return buildPointCommand(b.ES, "record_agent_commit", func(pc *PointContext) core.Command {
		return &evaluatorReceiptCmd{inner: &recordAgentCommitCmd{pc: pc}, point: pc}
	})
}
