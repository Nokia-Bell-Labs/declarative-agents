// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	applierRecipeLabel   = "io.declarative-agents.applier.recipe"
	applierPlatformLabel = "io.declarative-agents.applier.platform"
	applierRuntimeLabel  = "io.declarative-agents.applier.runtime"
)

// ApplierImageResult records the verified identity of the shared applier image.
type ApplierImageResult struct {
	Reference string
	Revision  string
	Recipe    string
	Platform  string
	RuntimeID string
	ImageID   string
	Reused    bool
	Elapsed   time.Duration
}

type applierImageIdentity struct {
	revision  string
	recipe    string
	platform  string
	runtimeID string
}

// EnsureApplierImage builds or reuses the shared agent-core image with Helm and
// kubectl. Reuse requires Docker to prove the exact checkout revision, recipe,
// platform, and verified runtime image ID; the mutable image tag is never enough.
func EnsureApplierImage(
	coreRoot, runtimeImage, image string,
) (ApplierImageResult, error) {
	return defaultImageBuilder().ensureApplier(coreRoot, runtimeImage, image)
}

func (b imageBuilder) ensureApplier(
	coreRoot, runtimeImage, image string,
) (ApplierImageResult, error) {
	started := time.Now()
	identity, err := b.applierIdentity(coreRoot, runtimeImage)
	if err != nil {
		return ApplierImageResult{}, err
	}
	if result, matches := b.inspectApplier(image, identity); matches {
		result.Reused, result.Elapsed = true, time.Since(started)
		logApplierImagePhase(result, "reused")
		return result, nil
	}

	// Lock the mutable tag, not the recipe. Chatbot Mesh FROMs
	// declarative-agents/agent-core:<rev> while Coding Agent and Agent
	// Architecture FROM :local; those recipes differ, so a recipe-keyed lock
	// lets sibling lanes retag the same declarative-agents/applier:<rev>
	// out from under inspect (GH-1764).
	unlock, err := b.acquireApplierTagLock(image)
	if err != nil {
		return ApplierImageResult{}, err
	}
	defer unlock()
	if result, matches := b.inspectApplier(image, identity); matches {
		result.Reused, result.Elapsed = true, time.Since(started)
		logApplierImagePhase(result, "reused-after-wait")
		return result, nil
	}
	if err := b.buildApplier(coreRoot, runtimeImage, image, identity); err != nil {
		return ApplierImageResult{}, err
	}
	result, matches := b.inspectApplier(image, identity)
	if !matches {
		return ApplierImageResult{}, fmt.Errorf(
			"built applier image %s does not carry the required revision, recipe, platform, and runtime identity",
			image)
	}
	result.Elapsed = time.Since(started)
	logApplierImagePhase(result, "built")
	return result, nil
}

func (b imageBuilder) applierIdentity(
	coreRoot, runtimeImage string,
) (applierImageIdentity, error) {
	revisionOutput, err := b.output(coreRoot, nil, "git", "rev-parse", "HEAD")
	if err != nil {
		return applierImageIdentity{}, fmt.Errorf("resolve applier source revision: %w", err)
	}
	revision := strings.ToLower(strings.TrimSpace(string(revisionOutput)))
	if len(revision) < 12 {
		return applierImageIdentity{}, fmt.Errorf("applier source revision %q is not a commit hash", revision)
	}
	platform := "linux/" + runtime.GOARCH
	runtimeMetadata, ok := b.inspectDockerImage(runtimeImage)
	if !ok ||
		runtimeMetadata.ID == "" ||
		runtimeMetadata.OS+"/"+runtimeMetadata.Architecture != platform ||
		runtimeMetadata.Labels["org.opencontainers.image.revision"] != revision ||
		runtimeMetadata.Labels["io.declarative-agents.agent-core.platform"] != platform ||
		runtimeMetadata.Labels["io.declarative-agents.agent-core.recipe"] == "" {
		return applierImageIdentity{}, fmt.Errorf(
			"runtime image %s does not carry the verified revision and platform identity required by the applier",
			runtimeImage)
	}
	dockerfile, err := os.ReadFile(filepath.Join(coreRoot, "applier.Dockerfile"))
	if err != nil {
		return applierImageIdentity{}, fmt.Errorf("read applier Dockerfile: %w", err)
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{
		"applier-image/v1",
		string(dockerfile),
		platform,
		"RUNTIME_IMAGE=" + runtimeImage,
		"RUNTIME_IMAGE_ID=" + runtimeMetadata.ID,
		"RUNTIME_RECIPE=" + runtimeMetadata.Labels["io.declarative-agents.agent-core.recipe"],
		"TARGETARCH=" + runtime.GOARCH,
	}, "\x00")))
	return applierImageIdentity{
		revision:  revision,
		recipe:    fmt.Sprintf("sha256:%x", sum),
		platform:  platform,
		runtimeID: runtimeMetadata.ID,
	}, nil
}

func (b imageBuilder) buildApplier(
	coreRoot, runtimeImage, image string,
	identity applierImageIdentity,
) error {
	args := []string{
		"build", "--platform", identity.platform,
		"-f", filepath.Join(coreRoot, "applier.Dockerfile"),
		"--build-arg", "RUNTIME_IMAGE=" + runtimeImage,
		"--build-arg", "TARGETARCH=" + strings.TrimPrefix(identity.platform, "linux/"),
		"--label", "org.opencontainers.image.revision=" + identity.revision,
		"--label", applierRecipeLabel + "=" + identity.recipe,
		"--label", applierPlatformLabel + "=" + identity.platform,
		"--label", applierRuntimeLabel + "=" + identity.runtimeID,
		"-t", image, ".",
	}
	if err := b.run(coreRoot, nil, "docker", args...); err != nil {
		return fmt.Errorf("docker build applier image %s: %w", image, err)
	}
	return nil
}

func (b imageBuilder) inspectApplier(
	image string,
	identity applierImageIdentity,
) (ApplierImageResult, bool) {
	metadata, ok := b.inspectDockerImage(image)
	if !ok {
		return ApplierImageResult{}, false
	}
	matches := strings.HasPrefix(metadata.ID, "sha256:") &&
		metadata.OS+"/"+metadata.Architecture == identity.platform &&
		metadata.Labels["org.opencontainers.image.revision"] == identity.revision &&
		metadata.Labels[applierRecipeLabel] == identity.recipe &&
		metadata.Labels[applierPlatformLabel] == identity.platform &&
		metadata.Labels[applierRuntimeLabel] == identity.runtimeID
	return ApplierImageResult{
		Reference: image,
		Revision:  identity.revision,
		Recipe:    identity.recipe,
		Platform:  identity.platform,
		RuntimeID: identity.runtimeID,
		ImageID:   metadata.ID,
	}, matches
}

func logApplierImagePhase(result ApplierImageResult, outcome string) {
	fmt.Printf("phase target=applier-image name=build elapsed=%s outcome=%s image=%s digest=%s runtime=%s\n",
		result.Elapsed.Round(time.Millisecond), outcome, result.Reference, result.ImageID, result.RuntimeID)
}

// acquireApplierTagLock serializes every builder of the same mutable image
// reference. The lock is machine-local (lockRoot) so concurrent Mage processes
// on one Docker daemon cannot clobber each other's inspect.
func (b imageBuilder) acquireApplierTagLock(image string) (func(), error) {
	sum := sha256.Sum256([]byte("applier-image-tag\x00" + image))
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
