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

// agentCoreDockerfile is the minimal runtime image contract: the linux agent on
// PATH and the core tools under AGENT_CORE_HOME. jq and ripgrep match the
// transform tools the production agent-core image carries so mounted profiles
// whose exec words are jq/rg (e.g. the documentation-curator, GH-1368) run in
// kind smokes on this local image. chatbot-mesh's buildSmokeRuntimeImage is the
// sibling of this builder and must keep the same contract.
const agentCoreDockerfile = "FROM alpine:3.22\n" +
	"RUN apk add --no-cache ca-certificates bash jq ripgrep\n" +
	"COPY agent /usr/local/bin/agent\n" +
	"COPY tools /opt/agent-core/tools\n" +
	"ENV AGENT_CORE_HOME=/opt/agent-core HOME=/tmp PATH=/usr/local/bin:/usr/bin:/bin\n" +
	"ENTRYPOINT [\"agent\"]\n"

// imageCommandRunner runs an external command in dir with extra environment,
// injected so the build/docker invocations are observed and their failures
// exercised without a real toolchain.
type imageCommandRunner func(dir string, env []string, name string, args ...string) error

// imageBuilder holds the command and filesystem boundaries the image build
// depends on, so each boundary -- and its failure -- can be exercised in tests.
type imageBuilder struct {
	run       imageCommandRunner
	writeFile func(name string, data []byte, perm os.FileMode) error
	copyTree  func(src, dst string) error
}

func defaultImageBuilder() imageBuilder {
	return imageBuilder{
		run:       runImageCommand,
		writeFile: os.WriteFile,
		copyTree:  copyTreeContents,
	}
}

func runImageCommand(dir string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	return cmd.Run()
}

// BuildAgentCoreImage builds the linux agent binary from the local agent-core
// checkout and bakes it into a minimal runtime image, so local flows run the
// code under test rather than a published image.
func BuildAgentCoreImage(coreRoot, image string) error {
	return defaultImageBuilder().build(coreRoot, image)
}

func (b imageBuilder) build(coreRoot, image string) error {
	ctxDir, err := os.MkdirTemp("", "agent-core-image-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(ctxDir) }()

	if err := b.run(coreRoot, []string{"CGO_ENABLED=0", "GOOS=linux"},
		"go", "build", "-tags", "production", "-trimpath", "-ldflags=-s -w",
		"-o", filepath.Join(ctxDir, "agent"), "./cmd/agent"); err != nil {
		return fmt.Errorf("build linux agent: %w", err)
	}
	if err := b.copyTree(filepath.Join(coreRoot, "tools"), filepath.Join(ctxDir, "tools")); err != nil {
		return err
	}
	if err := b.writeFile(filepath.Join(ctxDir, "Dockerfile"), []byte(agentCoreDockerfile), 0o644); err != nil {
		return fmt.Errorf("write agent-core image Dockerfile: %w", err)
	}
	if err := b.run(ctxDir, nil, "docker", AgentCoreImageBuildArgs(image)...); err != nil {
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
		return copyRegularFile(path, target)
	})
}

// copyRegularFile copies one regular file, closing both descriptors
// immediately rather than deferring to the end of a traversal (so a many-file
// tree cannot exhaust descriptors) and reporting read, write, and close
// failures.
func copyRegularFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		_ = source.Close()
		return err
	}
	destination, err := os.Create(dst)
	if err != nil {
		_ = source.Close()
		return err
	}
	_, copyErr := io.Copy(destination, source)
	closeDstErr := destination.Close()
	closeSrcErr := source.Close()
	switch {
	case copyErr != nil:
		return fmt.Errorf("copy %s to %s: %w", src, dst, copyErr)
	case closeDstErr != nil:
		return fmt.Errorf("close %s: %w", dst, closeDstErr)
	case closeSrcErr != nil:
		return fmt.Errorf("close %s: %w", src, closeSrcErr)
	}
	return nil
}
