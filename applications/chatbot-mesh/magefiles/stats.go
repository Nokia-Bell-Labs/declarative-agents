// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type meshStatsOutput struct {
	Agents      agentsSection      `json:"agents"`
	Composition compositionSection `json:"composition"`
	Application struct {
		Ownership           string `json:"ownership"`
		AgentsContributed   int    `json:"agents_contributed"`
		CompositionWrappers int    `json:"composition_wrappers"`
	} `json:"application"`
}

// Stats outputs local implementation metrics and composition reuse separately.
// Unlike the platform sub-modules, the application reports no module-wide LOC
// breakdown: its Go and Helm code are deployment scaffolding, and only the
// locally owned agents feed root implementation totals (GH-754, GH-1000).
func Stats() error {
	ownership, err := scanAgentOwnership("agents", meshCountLines)
	if err != nil {
		return err
	}
	data, err := os.ReadFile("agents/application.yaml")
	if err != nil {
		return fmt.Errorf("read application manifest for stats: %w", err)
	}
	var manifest struct {
		Ownership string `yaml:"ownership"`
	}
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("parse application manifest for stats: %w", err)
	}
	rec := newMeshStatsOutput(ownership, manifest.Ownership)

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(rec)
}

func newMeshStatsOutput(ownership agentOwnershipStats, applicationOwnership string) meshStatsOutput {
	rec := meshStatsOutput{
		Agents:      ownership.Agents,
		Composition: ownership.Composition,
	}
	rec.Application.Ownership = applicationOwnership
	rec.Application.AgentsContributed = ownership.Agents.Total.Agents
	rec.Application.CompositionWrappers = ownership.Composition.Total.Wrappers
	return rec
}

func meshCountLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	n := 0
	s := bufio.NewScanner(f)
	for s.Scan() {
		n++
	}
	return n, s.Err()
}
