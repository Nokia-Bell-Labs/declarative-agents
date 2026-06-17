// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

const knowledgeManagerProfile = "agents/knowledge-manager/documentation-curator/profile.yaml"

// START OMIT
func main() {
	StartAgent(knowledgeManagerProfile)
}

// END OMIT

func StartAgent(profile string) {
	profilesRoot := envOrDefault("AGENT_PROFILES_ROOT", "/profiles")
	workspace := envOrDefault("AGENT_WORKSPACE", "/work")
	cmd := exec.Command("agent",
		"--profile", filepath.Join(profilesRoot, profile),
		"--directory", workspace,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
