// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const agentProfilesRootEnv = "AGENT_PROFILES_ROOT"

func resolveAgentProfilesRoot(rootDir string) (string, error) {
	for _, candidate := range agentProfileRootCandidates(rootDir) {
		root := normalizeAgentProfilesRoot(candidate)
		if root == "" {
			continue
		}
		if hasProfile(root, "generator") || hasProfile(root, "knowledge-manager/documentation-curator") {
			return root, nil
		}
	}
	return "", fmt.Errorf("agent profiles root not found; set %s", agentProfilesRootEnv)
}

func agentProfileRootCandidates(rootDir string) []string {
	candidates := []string{}
	if configured := os.Getenv(agentProfilesRootEnv); configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates,
		filepath.Join(filepath.Dir(rootDir), "agent-profiles"),
		filepath.Join(rootDir, "agent-profiles"),
	)
	return candidates
}

func normalizeAgentProfilesRoot(candidate string) string {
	if candidate == "" {
		return ""
	}
	if hasProfile(candidate, "generator") || hasProfile(candidate, "knowledge-manager/documentation-curator") {
		return candidate
	}
	nested := filepath.Join(candidate, "agents")
	if hasProfile(nested, "generator") || hasProfile(nested, "knowledge-manager/documentation-curator") {
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

func resolveAgentProfilesRepoRoot(rootDir string) (string, error) {
	for _, candidate := range agentProfileRootCandidates(rootDir) {
		root := normalizeAgentProfilesRepoRoot(candidate)
		if root != "" {
			return root, nil
		}
	}
	return "", fmt.Errorf("agent profiles repository root not found; set %s", agentProfilesRootEnv)
}

func normalizeAgentProfilesRepoRoot(candidate string) string {
	if candidate == "" {
		return ""
	}
	for _, root := range []string{candidate, filepath.Dir(candidate)} {
		if hasProfile(filepath.Join(root, "agents"), "generator") && hasIntegrationFixtureRoot(root) {
			return root
		}
	}
	return ""
}

func hasIntegrationFixtureRoot(root string) bool {
	info, err := os.Stat(filepath.Join(root, "testdata", "integration"))
	return err == nil && info.IsDir()
}
