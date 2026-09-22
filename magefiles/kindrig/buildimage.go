// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// agentCoreComponent is the local runtime component ENG01 names for the
// profile-free agent image this checkout builds.
const agentCoreComponent = "agent-core"

// agentCoreDockerfile is the minimal runtime image contract: the linux agent on
// PATH and the core tools under AGENT_CORE_HOME. jq and ripgrep match the
// transform tools the production agent-core image carries so mounted profiles
// whose exec words are jq/rg (e.g. the documentation-curator, GH-1368) run in
// kind smokes on this local image. chatbot-mesh's buildSmokeRuntimeImage is the
// sibling of this builder and must keep the same contract.
const agentCoreDockerfile = "FROM docker.io/library/alpine:3.22@sha256:5291449c3df73caf6ed85e649dec1b9e818b39a5d8c871e97afc13e9cd5e8fa8\n" +
	"RUN apk add --no-cache ca-certificates bash jq ripgrep\n" +
	"COPY agent /usr/local/bin/agent\n" +
	"COPY tools /opt/agent-core/tools\n" +
	"ENV AGENT_CORE_HOME=/opt/agent-core HOME=/tmp PATH=/usr/local/bin:/usr/bin:/bin\n" +
	"ENTRYPOINT [\"agent\"]\n"

// imageCommandRunner runs an external command in dir with extra environment,
// injected so the build/docker invocations are observed and their failures
// exercised without a real toolchain.
type imageCommandRunner func(dir string, env []string, name string, args ...string) error
type imageOutputRunner func(dir string, env []string, name string, args ...string) ([]byte, error)

// imageBuilder holds the command and filesystem boundaries the image build
// depends on, so each boundary -- and its failure -- can be exercised in tests.
type imageBuilder struct {
	run       imageCommandRunner
	output    imageOutputRunner
	writeFile func(name string, data []byte, perm os.FileMode) error
	copyTree  func(src, dst string) error
	lockRoot  string
	sleep     func(time.Duration)
}

func defaultImageBuilder() imageBuilder {
	return imageBuilder{
		run:       runImageCommand,
		output:    runImageOutput,
		writeFile: os.WriteFile,
		copyTree:  copyTreeContents,
		lockRoot:  filepath.Join(os.TempDir(), "declarative-agents-image-locks"),
		sleep:     time.Sleep,
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

func runImageOutput(dir string, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.Output()
}

type AgentCoreImageResult struct {
	Reference string
	Revision  string
	Recipe    string
	Platform  string
	ImageID   string
	Reused    bool
	Elapsed   time.Duration
}

type agentCoreImageIdentity struct {
	revision string
	recipe   string
	platform string
}

type dockerImageMetadata struct {
	ID           string
	OS           string
	Architecture string
	Labels       map[string]string
}

// HostPlatform is the platform a kind node runs, which is the host's: the
// node is a container on this machine. A cluster elsewhere runs its own
// architecture and says so (GH-2457).
func HostPlatform() string {
	return "linux/" + runtime.GOARCH
}

// AgentCoreRuntimeReference is the immutable local name for one agent-core
// checkout revision on one linux architecture. Published ghcr and Artifact
// Registry tags stay outside this namespace.
func AgentCoreRuntimeReference(revision, platform string) (image, shortRevision string, err error) {
	revision = strings.TrimSpace(revision)
	if !gitRevision.MatchString(revision) {
		return "", "", fmt.Errorf("git revision %q must be 12-64 hexadecimal characters", revision)
	}
	image, err = FormatGitLocal(RuntimeRole, agentCoreComponent, revision, platform)
	if err != nil {
		return "", "", err
	}
	return image, strings.ToLower(revision[:12]), nil
}

// AgentCoreRuntimeImage resolves the checkout HEAD and host platform into the
// canonical local runtime reference.
func AgentCoreRuntimeImage(coreRoot string) (string, error) {
	return AgentCoreRuntimeImageForPlatform(coreRoot, HostPlatform())
}

// AgentCoreRuntimeImageForPlatform is AgentCoreRuntimeImage for a named
// cluster platform.
func AgentCoreRuntimeImageForPlatform(coreRoot, platform string) (string, error) {
	identity, err := defaultImageBuilder().identity(coreRoot, platform)
	if err != nil {
		return "", err
	}
	image, _, err := AgentCoreRuntimeReference(identity.revision, identity.platform)
	return image, err
}

// BuildAgentCoreRuntimeImage builds the canonical local runtime image for the
// checkout on the host platform.
func BuildAgentCoreRuntimeImage(coreRoot string) error {
	image, err := AgentCoreRuntimeImage(coreRoot)
	if err != nil {
		return err
	}
	return BuildAgentCoreImage(coreRoot, image)
}

// BuildAgentCoreImage builds the linux agent binary from the local agent-core
// checkout and bakes it into a minimal runtime image, so local flows run the
// code under test rather than a published image.
func BuildAgentCoreImage(coreRoot, image string) error {
	return BuildAgentCoreImageForPlatform(coreRoot, image, HostPlatform())
}

// BuildAgentCoreImageForPlatform builds for a named platform, for a cluster
// whose nodes are not this machine.
func BuildAgentCoreImageForPlatform(coreRoot, image, platform string) error {
	return defaultImageBuilder().build(coreRoot, image, platform)
}

// EnsureAgentCoreImage reuses image only when Docker proves that the
// canonical typed reference names the exact checkout revision, build recipe,
// and host architecture. Separate Mage processes coordinate through an atomic
// lock.
func EnsureAgentCoreImage(coreRoot, image string) (AgentCoreImageResult, error) {
	return EnsureAgentCoreImageForPlatform(coreRoot, image, HostPlatform())
}

// EnsureAgentCoreImageForPlatform is EnsureAgentCoreImage for a named
// platform. The platform is part of the reuse identity, so a host image and
// a cluster image of the same revision do not stand in for each other: an
// arm64 workstation that pushed its own build to a registry serving amd64
// nodes left every pod reporting "no match for platform in manifest"
// (GH-2457).
func EnsureAgentCoreImageForPlatform(
	coreRoot, image, platform string,
) (AgentCoreImageResult, error) {
	return defaultImageBuilder().ensure(coreRoot, image, platform)
}

func (b imageBuilder) build(coreRoot, image, platform string) error {
	identity, err := b.identity(coreRoot, platform)
	if err != nil {
		return err
	}
	return b.buildIdentity(coreRoot, image, identity)
}

func (b imageBuilder) buildIdentity(
	coreRoot, image string,
	identity agentCoreImageIdentity,
) error {
	if err := requireCanonicalAgentCoreImage(image, identity); err != nil {
		return err
	}
	ctxDir, err := os.MkdirTemp("", "agent-core-image-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(ctxDir) }()

	arch := strings.TrimPrefix(identity.platform, "linux/")
	if err := b.run(coreRoot, []string{"CGO_ENABLED=0", "GOOS=linux", "GOARCH=" + arch},
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
	if err := b.run(ctxDir, nil, "docker", agentCoreImageBuildArgs(image, identity)...); err != nil {
		return fmt.Errorf("docker build %s: %w", image, err)
	}
	return nil
}

func (b imageBuilder) identity(coreRoot, platform string) (agentCoreImageIdentity, error) {
	output, err := b.output(coreRoot, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return agentCoreImageIdentity{}, fmt.Errorf("resolve agent-core revision: %w", err)
	}
	revision := strings.ToLower(strings.TrimSpace(string(output)))
	if len(revision) < 12 {
		return agentCoreImageIdentity{}, fmt.Errorf("agent-core revision %q is not a commit hash", revision)
	}
	platform = strings.TrimSpace(platform)
	if platform == "" {
		return agentCoreImageIdentity{}, fmt.Errorf("agent-core image platform is required")
	}
	osName, arch, err := SplitPlatform(platform)
	if err != nil {
		return agentCoreImageIdentity{}, err
	}
	platform = osName + "/" + arch
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"agent-core-image/v1", agentCoreDockerfile, platform,
		"CGO_ENABLED=0", "GOOS=linux", "GOARCH=" + arch,
		"go build -tags production -trimpath -ldflags=-s -w ./cmd/agent",
	}, "\x00")))
	return agentCoreImageIdentity{
		revision: revision,
		recipe:   fmt.Sprintf("sha256:%x", sum),
		platform: platform,
	}, nil
}

func (b imageBuilder) ensure(coreRoot, image, platform string) (AgentCoreImageResult, error) {
	started := time.Now()
	identity, err := b.identity(coreRoot, platform)
	if err != nil {
		return AgentCoreImageResult{}, err
	}
	if err := requireCanonicalAgentCoreImage(image, identity); err != nil {
		return AgentCoreImageResult{}, err
	}
	if result, matches := b.inspect(image, identity); matches {
		result.Reused, result.Elapsed = true, time.Since(started)
		logImagePhase(result, "reused")
		return result, nil
	}
	unlock, err := b.acquireLock(coreRoot, identity)
	if err != nil {
		return AgentCoreImageResult{}, err
	}
	defer unlock()
	if result, matches := b.inspect(image, identity); matches {
		result.Reused, result.Elapsed = true, time.Since(started)
		logImagePhase(result, "reused-after-wait")
		return result, nil
	}
	if err := b.buildIdentity(coreRoot, image, identity); err != nil {
		return AgentCoreImageResult{}, err
	}
	result, matches := b.inspect(image, identity)
	if !matches {
		return AgentCoreImageResult{}, fmt.Errorf(
			"built image %s does not carry the required revision, recipe, and platform identity", image)
	}
	result.Elapsed = time.Since(started)
	logImagePhase(result, "built")
	return result, nil
}

func (b imageBuilder) inspect(
	image string,
	identity agentCoreImageIdentity,
) (AgentCoreImageResult, bool) {
	item, ok := b.inspectDockerImage(image)
	if !ok {
		return AgentCoreImageResult{}, false
	}
	labels := item.Labels
	matches := strings.HasPrefix(item.ID, "sha256:") &&
		item.OS+"/"+item.Architecture == identity.platform &&
		labels["org.opencontainers.image.revision"] == identity.revision &&
		labels["io.declarative-agents.agent-core.recipe"] == identity.recipe &&
		labels["io.declarative-agents.agent-core.platform"] == identity.platform
	return AgentCoreImageResult{
		Reference: image, Revision: identity.revision, Recipe: identity.recipe,
		Platform: identity.platform, ImageID: item.ID,
	}, matches
}

func (b imageBuilder) inspectDockerImage(image string) (dockerImageMetadata, bool) {
	output, err := b.output("", nil, "docker", "image", "inspect", image)
	if err != nil {
		return dockerImageMetadata{}, false
	}
	var inspected []struct {
		ID           string
		Os           string
		Architecture string
		Config       struct {
			Labels map[string]string
		}
	}
	if json.Unmarshal(output, &inspected) != nil || len(inspected) != 1 {
		return dockerImageMetadata{}, false
	}
	item := inspected[0]
	return dockerImageMetadata{
		ID:           item.ID,
		OS:           item.Os,
		Architecture: item.Architecture,
		Labels:       item.Config.Labels,
	}, true
}

func (b imageBuilder) acquireLock(
	coreRoot string,
	identity agentCoreImageIdentity,
) (func(), error) {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		filepath.Clean(coreRoot), identity.revision, identity.recipe, identity.platform,
	}, "\x00")))
	if err := os.MkdirAll(b.lockRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create image lock root: %w", err)
	}
	path := filepath.Join(b.lockRoot, fmt.Sprintf("%x.lock", sum[:16]))
	deadline := time.Now().Add(10 * time.Minute)
	for {
		if err := os.Mkdir(path, 0o700); err == nil {
			return func() { _ = os.RemoveAll(path) }, nil
		} else if !os.IsExist(err) {
			return nil, fmt.Errorf("acquire image build lock: %w", err)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for image build lock %s", path)
		}
		b.sleep(100 * time.Millisecond)
	}
}

func logImagePhase(result AgentCoreImageResult, outcome string) {
	fmt.Printf("phase target=agent-core-image name=build elapsed=%s outcome=%s image=%s digest=%s\n",
		result.Elapsed.Round(time.Millisecond), outcome, result.Reference, result.ImageID)
}

func agentCoreImageBuildArgs(image string, identity agentCoreImageIdentity) []string {
	return []string{
		"build", "--platform", identity.platform,
		"--label", "org.opencontainers.image.revision=" + identity.revision,
		"--label", "io.declarative-agents.agent-core.recipe=" + identity.recipe,
		"--label", "io.declarative-agents.agent-core.platform=" + identity.platform,
		"-t", image, ".",
	}
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
