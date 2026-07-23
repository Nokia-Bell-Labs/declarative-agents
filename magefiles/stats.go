// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Stats runs mage stats in each sub-module and participating example module,
// then outputs combined JSON to stdout. Modules that own agents report an
// "agents" section; the combined output adds an "agents_total" key summing
// those sections across the repository (GH-754). Values that fit on one line
// are printed on one line, so a leaf such as {"files": 4, "lines": 145} reads
// as a single fact (GH-758).
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
		results[mod] = raw
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
				Total agentsTotalJSON `json:"total"`
			} `json:"agents"`
		}
		if err := json.Unmarshal(raw, &doc); err != nil {
			return total, fmt.Errorf("parse stats from %s: %w", mod, err)
		}
		if doc.Agents == nil {
			continue
		}
		t := doc.Agents.Total
		total.Agents += t.Agents
		total.States += t.States
		total.Transitions += t.Transitions
		total.Tools += t.Tools
		total.YAML.Files += t.YAML.Files
		total.YAML.Lines += t.YAML.Lines
	}
	return total, nil
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
