// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/appmanifest"
	"gopkg.in/yaml.v3"
)

type codingApplicationStats struct {
	Application struct {
		Ownership           string   `json:"ownership"`
		AgentsContributed   int      `json:"agents_contributed"`
		CanonicalReferences int      `json:"canonical_references"`
		CanonicalRoles      []string `json:"canonical_roles"`
		CompositionWrappers int      `json:"composition_wrappers"`
		ServingProfiles     int      `json:"serving_profiles"`
		ServingRoles        []string `json:"serving_roles"`
		ProfileFreeRuntime  bool     `json:"profile_free_runtime"`
		UIAssets            int      `json:"ui_assets"`
		PackageAssets       int      `json:"package_assets"`
	} `json:"application"`
}

// Stats reports application composition without adding an "agents" section.
// Canonical implementations are counted once by applications/catalog; this
// target reports every manifest-declared deployment without maintaining a
// second serving-role inventory.
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
	var manifest appmanifest.Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return result, fmt.Errorf("parse application manifest: %w", err)
	}

	result.Application.Ownership = manifest.Ownership
	for _, root := range manifest.Roots {
		if root.Ownership == "catalog" {
			result.Application.CanonicalReferences++
			result.Application.CanonicalRoles = append(result.Application.CanonicalRoles, root.ID)
		} else if root.Ownership == "local" {
			result.Application.CompositionWrappers++
		}
	}
	for _, entry := range manifest.Deployment.Entries {
		result.Application.ServingProfiles++
		result.Application.ServingRoles = append(result.Application.ServingRoles, entry.ID)
	}
	result.Application.ProfileFreeRuntime = !manifest.Runtime.ImageContainsProfiles
	result.Application.UIAssets = len(manifest.UI.Assets)
	result.Application.PackageAssets = len(manifest.Package.Assets)
	return result, nil
}
