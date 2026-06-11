// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
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

	if v, ok := pc.GridPoint["rep"]; ok {
		exp.Repetition = fmt.Sprintf("%v", v)
	}

	exp.AgentCommit = gitCommitHash()

	for flag, val := range pc.Harness.Flags {
		switch flag {
		case "machine":
			exp.Harness.Machine = readFileContent(fmt.Sprintf("%v", val))
		case "tools":
			exp.Harness.Tools = readFileContent(fmt.Sprintf("%v", val))
		case "tools-declaration":
			exp.Harness.ToolDeclarations = readToolDeclarations(val)
		}
	}

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
	Harness     experimentHarness `yaml:"harness"`
	Model       string            `yaml:"model"`
	OllamaURL   string            `yaml:"ollama_url,omitempty"`
	Timeout     string            `yaml:"timeout,omitempty"`
	Repetition  string            `yaml:"repetition,omitempty"`
	Sample      experimentSample  `yaml:"sample"`
}

type experimentHarness struct {
	Name             string                   `yaml:"name"`
	Binary           string                   `yaml:"binary"`
	Machine          map[string]interface{}   `yaml:"machine,omitempty"`
	Tools            map[string]interface{}   `yaml:"tools,omitempty"`
	ToolDeclarations []map[string]interface{} `yaml:"tool_declarations,omitempty"`
}

type experimentSample struct {
	Name string `yaml:"name"`
}

func readFileContent(path string) map[string]interface{} {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"_error": fmt.Sprintf("read %s: %v", path, err)}
	}
	var content map[string]interface{}
	if err := yaml.Unmarshal(data, &content); err != nil {
		return map[string]interface{}{"_error": fmt.Sprintf("parse %s: %v", path, err)}
	}
	return content
}

func readToolDeclarations(val interface{}) []map[string]interface{} {
	var paths []string
	switch v := val.(type) {
	case string:
		paths = strings.Split(v, ",")
	case []interface{}:
		for _, elem := range v {
			paths = append(paths, fmt.Sprintf("%v", elem))
		}
	default:
		return nil
	}

	var result []map[string]interface{}
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		content := readFileContent(p)
		content["_source"] = filepath.Base(p)
		result = append(result, content)
	}
	return result
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
