// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/magefile/mage/mg"
)

const knowledgeManagerProfile = "agents/knowledge-manager/documentation-curator/profile.yaml"

// Demo contains runnable profile demonstrations.
type Demo mg.Namespace

// KnowledgeManager builds agent-core and runs the shipped documentation-curator profile.
func (Demo) KnowledgeManager() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve agent-profiles root: %w", err)
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(profilesRoot), "agent-core"))
	binary, err := buildIntegrationAgent(coreRoot)
	if err != nil {
		return err
	}
	cmd := knowledgeManagerCommand(binary, profilesRoot, coreRoot)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func knowledgeManagerCommand(binary, profilesRoot, coreRoot string) *exec.Cmd {
	cmd := exec.Command(
		binary,
		"--profile", filepath.Join(profilesRoot, knowledgeManagerProfile),
		"--directory", profilesRoot,
		"--core-root", coreRoot,
	)
	cmd.Dir = profilesRoot
	return cmd
}
