// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// agentsSection reports per-agent state-machine and YAML metrics for every
// agent directory under agents/, plus a total across all agents.
type agentsSection struct {
	Total    agentsTotal           `json:"total"`
	PerAgent map[string]agentStats `json:"per_agent"`
}

type agentsTotal struct {
	Agents      int            `json:"agents"`
	States      int            `json:"states"`
	Transitions int            `json:"transitions"`
	Tools       int            `json:"tools"`
	YAML        agentYAMLStats `json:"yaml"`
}

type agentStats struct {
	States      int            `json:"states"`
	Transitions int            `json:"transitions"`
	Tools       int            `json:"tools"`
	YAML        agentYAMLStats `json:"yaml"`
}

type agentYAMLStats struct {
	Files int `json:"files"`
	Lines int `json:"lines"`
}

// agentMachineDoc captures the top-level sequences counted from a machine
// file. Nodes stay unparsed: only their number matters.
type agentMachineDoc struct {
	States      []yaml.Node `yaml:"states"`
	Transitions []yaml.Node `yaml:"transitions"`
	Instantiate []struct {
		Fragment string `yaml:"fragment"`
	} `yaml:"instantiate"`
}

// agentToolsDoc captures the tool selection list in tools.yaml. Declarations
// files repeat the same tools with full definitions and profile.yaml lists
// file paths under the same key, so only tools.yaml counts.
type agentToolsDoc struct {
	Tools []yaml.Node `yaml:"tools"`
}

// scanAgents walks each subdirectory of agentsDir and reports per-agent
// counts of states, transitions, tools, and YAML files/lines. Subdirectories
// without YAML files (e.g. README-only placeholders) are skipped. A missing
// agentsDir yields an empty section.
func scanAgents(agentsDir string, countLines func(string) (int, error)) (agentsSection, error) {
	section := agentsSection{PerAgent: map[string]agentStats{}}
	entries, err := os.ReadDir(agentsDir)
	if os.IsNotExist(err) {
		return section, nil
	}
	if err != nil {
		return section, err
	}

	coreRoot, _ := resolveAgentCoreRoot(filepath.Dir(filepath.Clean(agentsDir)))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stats, err := scanAgentDir(filepath.Join(agentsDir, entry.Name()), coreRoot, countLines)
		if err != nil {
			return section, err
		}
		if stats.YAML.Files == 0 {
			continue
		}
		section.PerAgent[entry.Name()] = stats
		section.Total.Agents++
		section.Total.States += stats.States
		section.Total.Transitions += stats.Transitions
		section.Total.Tools += stats.Tools
		section.Total.YAML.Files += stats.YAML.Files
		section.Total.YAML.Lines += stats.YAML.Lines
	}
	return section, nil
}

func scanAgentDir(dir, coreRoot string, countLines func(string) (int, error)) (agentStats, error) {
	var stats agentStats
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
			return nil
		}

		lines, _ := countLines(path)
		stats.YAML.Files++
		stats.YAML.Lines += lines

		base := filepath.Base(path)
		switch {
		case strings.HasSuffix(base, "machine.yaml"):
			states, transitions, err := countMachine(path, coreRoot)
			if err != nil {
				return err
			}
			stats.States += states
			stats.Transitions += transitions
		case base == "tools.yaml":
			var doc agentToolsDoc
			if err := unmarshalYAMLFile(path, &doc); err != nil {
				return err
			}
			stats.Tools += len(doc.Tools)
		}
		return nil
	})
	return stats, err
}

func unmarshalYAMLFile(path string, out any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if err := yaml.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	return nil
}

// countMachine counts the states and transitions a machine file declares. A
// file that instantiates a machine template (srd054) declares none itself, so
// its template body is counted instead: the fragment resolves against the file,
// or under the agent-core checkout for /opt/agent-core paths. Without this every
// instance counted as an empty machine (GH-2132).
func countMachine(path, coreRoot string) (states, transitions int, err error) {
	var doc agentMachineDoc
	if err := unmarshalYAMLFile(path, &doc); err != nil {
		return 0, 0, err
	}
	if len(doc.States) > 0 || len(doc.Transitions) > 0 || len(doc.Instantiate) != 1 {
		return len(doc.States), len(doc.Transitions), nil
	}
	fragment := doc.Instantiate[0].Fragment
	target := filepath.Join(filepath.Dir(path), fragment)
	if strings.HasPrefix(fragment, "/opt/agent-core/") {
		if coreRoot == "" {
			return 0, 0, nil
		}
		target = filepath.Join(coreRoot, strings.TrimPrefix(fragment, "/opt/agent-core/"))
	}
	var template struct {
		Machine agentMachineDoc `yaml:"machine"`
	}
	if err := unmarshalYAMLFile(target, &template); err != nil {
		return 0, 0, err
	}
	return len(template.Machine.States), len(template.Machine.Transitions), nil
}
