// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func resolveAgentProfilesRoot(rootDir string) (string, error) {
	for _, candidate := range agentProfileRootCandidates(rootDir) {
		root := normalizeAgentProfilesRoot(candidate)
		if root == "" {
			continue
		}
		if hasProfile(root, "executor") || hasProfile(root, "critic") {
			return root, nil
		}
	}
	return "", fmt.Errorf("agent profiles root not found; clone agent-profiles beside this repository or under ./agent-profiles")
}

func agentProfileRootCandidates(rootDir string) []string {
	return []string{
		filepath.Join(filepath.Dir(rootDir), "agent-profiles"),
		filepath.Join(rootDir, "agent-profiles"),
	}
}

func normalizeAgentProfilesRoot(candidate string) string {
	if candidate == "" {
		return ""
	}
	if hasProfile(candidate, "executor") || hasProfile(candidate, "critic") {
		return candidate
	}
	nested := filepath.Join(candidate, "agents")
	if hasProfile(nested, "executor") || hasProfile(nested, "critic") {
		return nested
	}
	return ""
}

func hasProfile(root, rel string) bool {
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel), "profile.yaml"))
	return err == nil && !info.IsDir()
}

func agentProfilePath(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel), "profile.yaml")
}

func agentProfileAsset(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

// conformanceAsset resolves a fixture under the agent-profiles
// testdata/conformance tree. GH-328 moved the rest/control/lifecycle fixture
// families there from the agents tree, so harnesses resolve them against the
// repository root rather than the profiles root. The profiles root is either
// <repo>/agents or the repository root itself (normalizeAgentProfilesRoot
// accepts both layouts), so probe both and fall back to the parent join so a
// miss reports the expected location (GH-821).
func conformanceAsset(profileRoot, rel string) string {
	for _, base := range []string{filepath.Dir(profileRoot), profileRoot} {
		path := filepath.Join(base, "testdata", "conformance", filepath.FromSlash(rel))
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return filepath.Join(filepath.Dir(profileRoot), "testdata", "conformance", filepath.FromSlash(rel))
}

func resolveAgentProfilesRepoRoot(rootDir string) (string, error) {
	for _, candidate := range agentProfileRootCandidates(rootDir) {
		root := normalizeAgentProfilesRepoRoot(candidate)
		if root != "" {
			return root, nil
		}
	}
	return "", fmt.Errorf("agent profiles repository root not found; clone agent-profiles beside this repository or under ./agent-profiles")
}

func normalizeAgentProfilesRepoRoot(candidate string) string {
	if candidate == "" {
		return ""
	}
	for _, root := range []string{candidate, filepath.Dir(candidate)} {
		if hasProfile(filepath.Join(root, "agents"), "executor") && hasIntegrationFixtureRoot(root) {
			return root
		}
	}
	return ""
}

func hasIntegrationFixtureRoot(root string) bool {
	info, err := os.Stat(filepath.Join(root, "testdata", "integration"))
	return err == nil && info.IsDir()
}
