// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// agentsSection reports state-machine and YAML metrics for locally implemented
// agents only. Composition wrappers are reported separately.
type agentsSection struct {
	Total    agentsTotal           `json:"total"`
	PerAgent map[string]agentStats `json:"per_agent"`
}

type compositionSection struct {
	Total      compositionTotal              `json:"total"`
	PerWrapper map[string]compositionWrapper `json:"per_wrapper"`
}

type compositionTotal struct {
	Wrappers            int            `json:"wrappers"`
	CanonicalReferences int            `json:"canonical_references"`
	YAML                agentYAMLStats `json:"yaml"`
}

type compositionWrapper struct {
	Ownership        string         `json:"ownership"`
	CanonicalSource  string         `json:"canonical_source"`
	CanonicalProgram string         `json:"canonical_program"`
	YAML             agentYAMLStats `json:"yaml"`
}

type agentOwnershipStats struct {
	Agents      agentsSection
	Composition compositionSection
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

type agentProfileDoc struct {
	Machine string `yaml:"machine"`
}

// scanAgents retains the implementation-only view used by focused callers.
// scanAgentOwnership additionally reports composition wrappers and references.
func scanAgents(agentsDir string, countLines func(string) (int, error)) (agentsSection, error) {
	ownership, err := scanAgentOwnership(agentsDir, countLines)
	return ownership.Agents, err
}

// declarationUnitsDir is the directory beside the agents that holds their
// shared declaration type units. It carries no profile and is not an agent.
const declarationUnitsDir = "units"

func scanAgentOwnership(
	agentsDir string,
	countLines func(string) (int, error),
) (agentOwnershipStats, error) {
	result := agentOwnershipStats{
		Agents: agentsSection{PerAgent: map[string]agentStats{}},
		Composition: compositionSection{
			PerWrapper: map[string]compositionWrapper{},
		},
	}
	entries, err := os.ReadDir(agentsDir)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}

	coreRoot := demoCoreRoot(filepath.Dir(filepath.Clean(agentsDir)))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		// units/ holds declaration type units, not an agent (srd051 R1.1).
		if entry.Name() == declarationUnitsDir {
			continue
		}
		stats, err := scanAgentDir(filepath.Join(agentsDir, entry.Name()), coreRoot, countLines)
		if err != nil {
			return result, err
		}
		if stats.YAML.Files == 0 {
			continue
		}
		canonical, compositionOnly, err := compositionProgram(
			agentsDir, filepath.Join(agentsDir, entry.Name()), stats)
		if err != nil {
			return result, err
		}
		if compositionOnly {
			result.Composition.PerWrapper[entry.Name()] = compositionWrapper{
				Ownership: "composition-wrapper", CanonicalSource: "applications/catalog",
				CanonicalProgram: canonical, YAML: stats.YAML,
			}
			result.Composition.Total.Wrappers++
			result.Composition.Total.CanonicalReferences++
			result.Composition.Total.YAML.Files += stats.YAML.Files
			result.Composition.Total.YAML.Lines += stats.YAML.Lines
			continue
		}
		result.Agents.PerAgent[entry.Name()] = stats
		result.Agents.Total.Agents++
		result.Agents.Total.States += stats.States
		result.Agents.Total.Transitions += stats.Transitions
		result.Agents.Total.Tools += stats.Tools
		result.Agents.Total.YAML.Files += stats.YAML.Files
		result.Agents.Total.YAML.Lines += stats.YAML.Lines
	}
	return result, nil
}

func compositionProgram(
	agentsDir, agentDir string,
	stats agentStats,
) (string, bool, error) {
	profilePath := filepath.Join(agentDir, "profile.yaml")
	var profile agentProfileDoc
	if err := unmarshalYAMLFile(profilePath, &profile); err != nil {
		if os.IsNotExist(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if strings.HasPrefix(filepath.ToSlash(profile.Machine), "agents/") {
		return filepath.ToSlash(filepath.Dir(profile.Machine)), true, nil
	}
	// A wrapper around a catalog agent names its machine through the staged
	// layout (../../catalog/<agent>/machine.yaml), which does not resolve
	// against this checkout's agents root. The canonical program is the
	// catalog agent the path ends in.
	if dir := filepath.ToSlash(filepath.Dir(profile.Machine)); strings.Contains(dir, "catalog/") {
		if agent := path.Base(dir); agent != "" && agent != "." && agent != "catalog" {
			return "agents/" + agent, true, nil
		}
	}
	if stats.States != 0 || stats.Tools != 0 {
		return "", false, nil
	}
	target := filepath.Clean(filepath.Join(agentDir, filepath.FromSlash(profile.Machine)))
	local, err := filepath.Rel(agentDir, target)
	if err != nil || (local != ".." && !strings.HasPrefix(local, ".."+string(filepath.Separator))) {
		return "", false, err
	}
	canonical, err := filepath.Rel(agentsDir, filepath.Dir(target))
	if err != nil || canonical == ".." ||
		strings.HasPrefix(canonical, ".."+string(filepath.Separator)) {
		return "", false, fmt.Errorf("canonical machine %q escapes agents root", profile.Machine)
	}
	return "agents/" + filepath.ToSlash(canonical), true, nil
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
