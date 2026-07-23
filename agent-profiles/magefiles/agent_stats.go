// Copyright (c) 2026 Nokia. All rights reserved.

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

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		stats, err := scanAgentDir(filepath.Join(agentsDir, entry.Name()), countLines)
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

func scanAgentDir(dir string, countLines func(string) (int, error)) (agentStats, error) {
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
			var doc agentMachineDoc
			if err := unmarshalYAMLFile(path, &doc); err != nil {
				return err
			}
			stats.States += len(doc.States)
			stats.Transitions += len(doc.Transitions)
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
