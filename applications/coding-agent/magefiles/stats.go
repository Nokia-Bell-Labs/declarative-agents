// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type applicationManifestStats struct {
	Roots []struct {
		ID        string `yaml:"id"`
		Ownership string `yaml:"ownership"`
	} `yaml:"roots"`
	Deployment struct {
		Entries []struct {
			ID string `yaml:"id"`
		} `yaml:"entries"`
	} `yaml:"deployment"`
	Runtime struct {
		ImageContainsProfiles bool `yaml:"image_contains_profiles"`
	} `yaml:"runtime"`
}

type codingApplicationStats struct {
	Application struct {
		Ownership           string   `json:"ownership"`
		AgentsContributed   int      `json:"agents_contributed"`
		CanonicalReferences int      `json:"canonical_references"`
		CanonicalRoles      []string `json:"canonical_roles"`
		ServingProfiles     int      `json:"serving_profiles"`
		ServingRoles        []string `json:"serving_roles"`
		ProfileFreeRuntime  bool     `json:"profile_free_runtime"`
	} `json:"application"`
}

// Stats reports application composition without adding an "agents" section.
// Canonical planner, executor, and critic implementations are counted once by
// applications/catalog; this target makes their reuse and the owned serving profiles
// visible without adding them to the repository-wide agents_total.
func Stats() error {
	stats, err := collectCodingApplicationStats("agents/application.yaml")
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(stats)
	if err != nil {
		return err
	}
	fmt.Println(string(encoded))
	return nil
}

func collectCodingApplicationStats(path string) (codingApplicationStats, error) {
	var result codingApplicationStats
	data, err := os.ReadFile(path)
	if err != nil {
		return result, fmt.Errorf("read application manifest: %w", err)
	}
	var manifest applicationManifestStats
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return result, fmt.Errorf("parse application manifest: %w", err)
	}

	result.Application.Ownership = "composition"
	for _, root := range manifest.Roots {
		if root.Ownership != "catalog" {
			continue
		}
		result.Application.CanonicalReferences++
		result.Application.CanonicalRoles = append(result.Application.CanonicalRoles, root.ID)
	}
	for _, entry := range manifest.Deployment.Entries {
		if entry.ID == "applier" {
			continue
		}
		result.Application.ServingProfiles++
		result.Application.ServingRoles = append(result.Application.ServingRoles, entry.ID)
	}
	result.Application.ProfileFreeRuntime = !manifest.Runtime.ImageContainsProfiles
	return result, nil
}
