// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/reusestats"
)

// Stats runs mage stats in each sub-module and participating application module,
// then outputs combined JSON to stdout. Modules that own agent implementations
// report an "agents" section; composition-only applications may instead report
// an "application" section describing canonical reuse. The combined output
// adds an "agents_total" key summing only implementation-owning sections, so
// reused agents are not counted twice (GH-754, GH-947). Values that fit on one
// line are printed on one line, so a leaf such as {"files": 4, "lines": 145}
// reads as a single fact (GH-758).
func Stats() error {
	raw, err := collectStats()
	if err != nil {
		return err
	}
	formatted, err := formatJSON(raw)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(formatted)
	return err
}

func writeReuseStats() error {
	raw, err := collectReuseStats()
	if err != nil {
		return err
	}
	formatted, err := formatJSON(raw)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(formatted)
	return err
}

func collectReuseStats() ([]byte, error) {
	results := make(map[string]reusestats.Result)
	samples := make(map[string]reusestats.Samples)
	for _, mod := range reuseParticipants() {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := os.Stat(mageDir); os.IsNotExist(err) {
			continue
		}
		result, err := runMageReuse(mod)
		if err != nil {
			return nil, fmt.Errorf("reuse stats in %s: %w", mod, err)
		}
		name := filepath.ToSlash(filepath.Clean(mod))
		results[name] = result
		// Medians do not fold, so the total re-reads each module's samples
		// from the same roots its own stats:reuse measured (GH-2111).
		moduleSamples, err := reusestats.CollectSamples(mod, reusestats.DefaultRoots...)
		if err != nil {
			return nil, fmt.Errorf("reuse samples in %s: %w", mod, err)
		}
		samples[name] = moduleSamples
	}
	results[reuseTotalKey] = sumReuseResults(results, samples)
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

const reuseTotalKey = "reuse_total"

func runMageReuse(dir string) (reusestats.Result, error) {
	cmd := exec.Command("mage", "stats:reuse")
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return reusestats.Result{}, err
	}
	var result reusestats.Result
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return result, fmt.Errorf("parse reuse stats from %s: %w", dir, err)
	}
	return result, nil
}

func sumReuseResults(
	results map[string]reusestats.Result,
	samples map[string]reusestats.Samples,
) reusestats.Result {
	var total reusestats.Result
	groups := map[string]reusestats.DuplicateBlock{}
	for module, result := range results {
		if module == reuseTotalKey {
			continue
		}
		total.TotalLines += result.TotalLines
		total.DuplicatedLines += result.DuplicatedLines
		total.CeremonyLines += result.CeremonyLines
		total.BehaviorLines += result.BehaviorLines
		total.DistinctToolDefs += result.DistinctToolDefs
		total.ToolRefs += result.ToolRefs
		total.ImportedUnits += result.ImportedUnits
		total.SharedUnits += result.SharedUnits
		total.SingleImporterUnits += result.SingleImporterUnits
		total.Expansions += result.Expansions
		addModuleBlocks(groups, module, result.TopBlocks)
	}
	total.DuplicationRatio = statsRatio(total.DuplicatedLines, total.TotalLines)
	total.CeremonyRatio = statsRatio(total.CeremonyLines, total.BehaviorLines)
	total.TopBlocks = rankedReuseBlocks(groups)
	total.Maintainability = reusestats.Summarize(reusestats.Merge(samples))
	return total
}

func addModuleBlocks(
	groups map[string]reusestats.DuplicateBlock,
	module string,
	blocks []reusestats.DuplicateBlock,
) {
	for _, block := range blocks {
		group := groups[block.Hash]
		group.Hash = block.Hash
		if block.Lines > group.Lines {
			group.Lines = block.Lines
		}
		group.Count += block.Count
		for _, file := range block.Files {
			group.Files = append(group.Files, filepath.ToSlash(filepath.Join(module, file)))
		}
		groups[block.Hash] = group
	}
}

func rankedReuseBlocks(groups map[string]reusestats.DuplicateBlock) []reusestats.DuplicateBlock {
	blocks := make([]reusestats.DuplicateBlock, 0, len(groups))
	for _, block := range groups {
		sort.Strings(block.Files)
		blocks = append(blocks, block)
	}
	sort.Slice(blocks, func(i, j int) bool {
		left := blocks[i].Lines * (blocks[i].Count - 1)
		right := blocks[j].Lines * (blocks[j].Count - 1)
		if left != right {
			return left > right
		}
		return blocks[i].Hash < blocks[j].Hash
	})
	if len(blocks) > 10 {
		blocks = blocks[:10]
	}
	return blocks
}

func statsRatio(numerator, denominator int) float64 {
	if denominator == 0 {
		return 0
	}
	return float64(numerator) / float64(denominator)
}

// collectStats dispatches mage stats to every participating module and returns
// the combined document, including the repository-wide agents_total.
func collectStats() ([]byte, error) {
	results := make(map[string]json.RawMessage)

	for _, mod := range statsParticipants() {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := os.Stat(mageDir); os.IsNotExist(err) {
			continue
		}

		raw, err := runMageStats(mod)
		if err != nil {
			return nil, fmt.Errorf("stats in %s: %w", mod, err)
		}
		// Source-relative slash paths are the stable public keys, independent of
		// the host path separator used to dispatch the child Mage process.
		results[filepath.ToSlash(filepath.Clean(mod))] = raw
	}

	total, err := sumAgentsTotals(results)
	if err != nil {
		return nil, err
	}
	rawTotal, err := json.Marshal(total)
	if err != nil {
		return nil, err
	}
	results[agentsTotalKey] = rawTotal

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(results); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// agentsTotalKey is the combined document's repository-wide agents total.
const agentsTotalKey = "agents_total"

// agentsTotalJSON mirrors the "agents.total" object each module's stats
// target emits for its agents/ directory.
type agentsTotalJSON struct {
	Agents      int `json:"agents"`
	States      int `json:"states"`
	Transitions int `json:"transitions"`
	Tools       int `json:"tools"`
	YAML        struct {
		Files int `json:"files"`
		Lines int `json:"lines"`
	} `json:"yaml"`
}

// sumAgentsTotals folds the per-module "agents.total" sections into one
// repository-wide total. Modules without an "agents" section (agent-core,
// design-patterns) contribute nothing.
func sumAgentsTotals(results map[string]json.RawMessage) (agentsTotalJSON, error) {
	var total agentsTotalJSON
	for mod, raw := range results {
		var doc struct {
			Agents *struct {
				Total    agentsTotalJSON            `json:"total"`
				PerAgent map[string]agentsTotalJSON `json:"per_agent"`
			} `json:"agents"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return total, fmt.Errorf("parse stats from %s: %w", mod, err)
		}
		if doc.Agents == nil {
			continue
		}
		t := doc.Agents.Total
		if doc.Agents.PerAgent != nil {
			var perAgent agentsTotalJSON
			for _, agent := range doc.Agents.PerAgent {
				addAgentsTotal(&perAgent, agent)
				perAgent.Agents++
			}
			if perAgent != t {
				return total, fmt.Errorf(
					"stats from %s has agents.total %+v, but per_agent sums to %+v",
					mod, t, perAgent)
			}
		}
		addAgentsTotal(&total, t)
	}
	return total, nil
}

func addAgentsTotal(total *agentsTotalJSON, value agentsTotalJSON) {
	total.Agents += value.Agents
	total.States += value.States
	total.Transitions += value.Transitions
	total.Tools += value.Tools
	total.YAML.Files += value.YAML.Files
	total.YAML.Lines += value.YAML.Lines
}

func runMageStats(dir string) (json.RawMessage, error) {
	cmd := exec.Command("mage", "stats")
	cmd.Dir = dir
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	raw := json.RawMessage(bytes.TrimSpace(stdout.Bytes()))
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid JSON from %s: %s", dir, stdout.String())
	}
	return raw, nil
}
