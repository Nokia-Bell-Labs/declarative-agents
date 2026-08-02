// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/magefile/mage/mg"
)

const (
	agentArchitectureImageRepository = "ghcr.io/nokia-bell-labs/declarative-agents/agent-architecture-runtime"
	agentArchitectureImageTag        = "0.1.0"
	imageBuildTimeout                = 5 * time.Minute
)

// Image groups production agent-architecture image targets.
type Image mg.Namespace

// Build builds the profile-free runtime image both chart workloads run. The
// build context is the repository root so the multi-stage build reaches
// agent-core; the runtime stage carries the agent binary and /opt/agent-core
// tools and never the profiles (srd001 R1.2, R4).
func (Image) Build() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	return buildAgentArchitectureImage(resolved.Application, resolved.Image)
}

func buildAgentArchitectureImage(applicationRoot, image string) error {
	contextDir, dockerfile, args := agentArchitectureImageBuild(applicationRoot, image)
	ctx, cancel := context.WithTimeout(context.Background(), imageBuildTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", args...)
	command.Dir = contextDir
	command.Stdout, command.Stderr = os.Stderr, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("docker build %s with %s: %w", image, dockerfile, err)
	}
	return nil
}

func agentArchitectureImageBuild(applicationRoot, image string) (string, string, []string) {
	repositoryRoot := filepath.Clean(filepath.Join(applicationRoot, "..", ".."))
	dockerfile := filepath.Join(applicationRoot, "Dockerfile")
	args := []string{
		"build", "--pull=false",
		"-f", dockerfile,
		"-t", image,
		".",
	}
	return repositoryRoot, dockerfile, args
}
