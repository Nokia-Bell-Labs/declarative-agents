// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	// The coding roles run the shared agent-core-toolchain image (GH-1368):
	// agent-core plus the Go toolchain and golangci-lint the executor's exec words
	// need. It replaces the retired per-app coding-agent-runtime.
	codingAgentImageRepository = "ghcr.io/nokia-bell-labs/declarative-agents/agent-core-toolchain"
	codingAgentImageTag        = "0.1.0"
	codingAgentGolangciLint    = "v2.12.2"
	// A first uncached toolchain build downloads and compiles Go plus
	// golangci-lint. Keep that networked build independent of kind operations.
	codingAgentImageBuildTimeout = 10 * time.Minute
)

// Image groups production coding-agent image targets.
type Image mg.Namespace

// Build builds the profile-free production image used by every chart role: the
// shared agent-core-toolchain, layered on a locally built agent-core so the target
// is self-contained (GH-1368).
func (Image) Build() error {
	applicationRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := codingAgentCoreRoot(applicationRoot)
	if err := kindrig.BuildAgentCoreImage(coreRoot, kindrig.DefaultAgentCoreImage); err != nil {
		return err
	}
	return buildCodingAgentImage(coreRoot, kindrig.DefaultAgentCoreImage, demoImage(applicationRoot))
}

// codingAgentCoreRoot resolves the agent-core checkout that carries the shared
// toolchain.Dockerfile, two levels up from an application root.
func codingAgentCoreRoot(applicationRoot string) string {
	return filepath.Clean(filepath.Join(applicationRoot, "..", "..", "agent-core"))
}

// buildCodingAgentImage builds the shared agent-core-toolchain image from the
// agent-core base named by runtimeImage. runtimeImage is passed as RUNTIME_IMAGE;
// empty leaves the Dockerfile's published default.
func buildCodingAgentImage(coreRoot, runtimeImage, image string) error {
	contextDir, dockerfile, args := codingAgentImageBuild(coreRoot, runtimeImage, image)
	ctx, cancel := context.WithTimeout(context.Background(), codingAgentImageBuildTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "docker", args...)
	command.Dir = contextDir
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("docker build %s with %s: %w: %s",
			image, dockerfile, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func codingAgentImageBuild(coreRoot, runtimeImage, image string) (string, string, []string) {
	dockerfile := filepath.Join(coreRoot, "toolchain.Dockerfile")
	args := []string{
		"build", "--pull=false",
		"--build-arg", "GOLANGCI_LINT_VERSION=" + codingAgentGolangciLint,
	}
	if runtimeImage != "" {
		args = append(args, "--build-arg", "RUNTIME_IMAGE="+runtimeImage)
	}
	args = append(args, "-f", dockerfile, "-t", image, ".")
	return coreRoot, dockerfile, args
}
