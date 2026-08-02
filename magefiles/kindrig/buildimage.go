// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
)

// DefaultAgentCoreImage is the repo-local tag for the agent-core runtime
// image. The ghcr.io name in chart values is the production default; local
// flows (the observability rig, kind smokes) build this tag from the checkout
// because the published image is not pullable from every environment.
const DefaultAgentCoreImage = "declarative-agents/agent-core:local"

// BuildAgentCoreImage builds the linux agent binary from the local agent-core
// checkout and bakes it into a minimal runtime image, so local flows run the
// code under test rather than a published image. The image mirrors the
// production runtime contract: agent on PATH and core tools under
// AGENT_CORE_HOME. chatbot-mesh's buildSmokeRuntimeImage is the sibling of
// this builder and must keep the same image contract.
func BuildAgentCoreImage(coreRoot, image string) error {
	ctxDir, err := os.MkdirTemp("", "agent-core-image-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(ctxDir) }()

	build := exec.Command("go", "build", "-tags", "production", "-trimpath",
		"-ldflags=-s -w", "-o", filepath.Join(ctxDir, "agent"), "./cmd/agent")
	build.Dir = coreRoot
	build.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux")
	build.Stdout, build.Stderr = os.Stderr, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("build linux agent: %w", err)
	}
	if err := copyTreeContents(filepath.Join(coreRoot, "tools"), filepath.Join(ctxDir, "tools")); err != nil {
		return err
	}
	dockerfile := "FROM alpine:3.22\n" +
		"RUN apk add --no-cache ca-certificates bash\n" +
		"COPY agent /usr/local/bin/agent\n" +
		"COPY tools /opt/agent-core/tools\n" +
		"ENV AGENT_CORE_HOME=/opt/agent-core HOME=/tmp PATH=/usr/local/bin:/usr/bin:/bin\n" +
		"ENTRYPOINT [\"agent\"]\n"
	if err := os.WriteFile(filepath.Join(ctxDir, "Dockerfile"), []byte(dockerfile), 0o644); err != nil {
		return fmt.Errorf("write agent-core image Dockerfile: %w", err)
	}
	docker := exec.Command("docker", AgentCoreImageBuildArgs(image)...)
	docker.Dir = ctxDir
	docker.Stdout, docker.Stderr = os.Stderr, os.Stderr
	if err := docker.Run(); err != nil {
		return fmt.Errorf("docker build %s: %w", image, err)
	}
	return nil
}

// AgentCoreImageBuildArgs returns the docker invocation BuildAgentCoreImage
// runs inside its temporary context directory.
func AgentCoreImageBuildArgs(image string) []string {
	return []string{"build", "-t", image, "."}
}

func copyTreeContents(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		source, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = source.Close() }()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		destination, err := os.Create(target)
		if err != nil {
			return err
		}
		defer func() { _ = destination.Close() }()
		_, err = io.Copy(destination, source)
		return err
	})
}
