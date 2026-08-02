// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/magefile/mage/mg"
)

const (
	codingAgentImageRepository = "ghcr.io/nokia-bell-labs/declarative-agents/coding-agent-runtime"
	codingAgentImageTag        = "0.1.0"
	codingAgentGolangciLint    = "v2.12.2"
)

// Image groups production coding-agent image targets.
type Image mg.Namespace

// Build builds the profile-free production image used by every chart role.
func (Image) Build() error {
	applicationRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	return buildCodingAgentImage(applicationRoot, demoImage(applicationRoot))
}

func buildCodingAgentImage(applicationRoot, image string) error {
	contextDir, dockerfile, args := codingAgentImageBuild(applicationRoot, image)
	ctx, cancel := context.WithTimeout(context.Background(), codingHelmClusterTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", args...)
	command.Dir = contextDir
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("docker build %s with %s: %w: %s",
			image, dockerfile, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func codingAgentImageBuild(applicationRoot, image string) (string, string, []string) {
	repositoryRoot := filepath.Clean(filepath.Join(applicationRoot, "..", ".."))
	dockerfile := filepath.Join(applicationRoot, "Dockerfile")
	args := []string{
		"build", "--pull=false",
		"--build-arg", "GOLANGCI_LINT_VERSION=" + codingAgentGolangciLint,
		"-f", dockerfile,
		"-t", image,
		".",
	}
	return repositoryRoot, dockerfile, args
}
