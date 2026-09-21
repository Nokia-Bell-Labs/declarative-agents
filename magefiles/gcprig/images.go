// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package gcprig

import (
	"fmt"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// donorSourceImage is the digest-pinned CLI donor the applier charts consume
// (the #2222 donor). The mirror exists because enterprise GKE nodes are
// commonly barred from Docker Hub; the overlay pins the mirror instead.
const donorSourceImage = "docker.io/alpine/k8s:1.31.4@" +
	"sha256:9c4976d47656d78cf53a92b0203fc54ac45eae18a2b45001ac221c27da4c8036"

// donorMirrorTag is the mirrored image's tag inside the project registry.
const donorMirrorTag = "cli-donor:1.31.4"

// MirrorDonor copies the pinned donor into the project registry and returns
// the mirror's digest, which the overlay pins.
//
// The copy goes through buildx imagetools rather than pull, tag, and push.
// A docker pull fetches only the host platform's manifest, so mirroring
// from an arm64 workstation would upload an arm64-only image to a registry
// serving amd64 Autopilot nodes, and the donor init container would fail on
// the one cluster the mirror exists for (GH-2437, found on a real project).
// imagetools copies the manifest list itself, from any host architecture and
// without a local pull.
//
// Because an index copy is byte-identical, the mirror's digest equals the
// upstream digest. The overlay's checked-in pin is therefore already correct
// after a mirror; only the repository path changes.
func MirrorDonor(run CommandRunner, config Config) (string, error) {
	mirror := config.RegistryPath() + "/" + donorMirrorTag
	started := time.Now()
	steps := [][]string{
		{"gcloud", "auth", "configure-docker", config.Region + "-docker.pkg.dev", "--quiet"},
		{"docker", "buildx", "imagetools", "create", "--tag", mirror, donorSourceImage},
	}
	for _, step := range steps {
		if out, err := run(step[0], step[1:]...); err != nil {
			return "", fmt.Errorf("%s: %w: %s",
				strings.Join(step[:2], " "), err, strings.TrimSpace(string(out)))
		}
	}
	out, err := run("docker", "buildx", "imagetools", "inspect", mirror)
	if err != nil {
		return "", fmt.Errorf("inspect mirror %s: %w: %s", mirror, err, strings.TrimSpace(string(out)))
	}
	digest := mirrorDigest(string(out))
	if digest == "" {
		return "", fmt.Errorf("inspect mirror %s: no digest in the manifest listing", mirror)
	}
	reference := mirror + "@" + digest
	kindrig.LogPhase(config.Cluster, "donor-mirror", "pushed", started, reference)
	return reference, nil
}

// mirrorDigest reads the top-level Digest line of an imagetools listing: the
// manifest list's own digest, not one platform's.
func mirrorDigest(listing string) string {
	for _, line := range strings.Split(listing, "\n") {
		rest, found := strings.CutPrefix(strings.TrimSpace(line), "Digest:")
		if !found {
			continue
		}
		return strings.TrimSpace(rest)
	}
	return ""
}

// PushAgentCore tags the locally built agent-core image with the short
// commit revision and pushes it to the project registry: the cloud
// counterpart of the kind image load, commit-tagged and never latest
// (eng01, eng08). localImage names the image a build target produced.
func PushAgentCore(run CommandRunner, config Config, localImage, revision string) (string, error) {
	revision = strings.TrimSpace(revision)
	if revision == "" {
		return "", fmt.Errorf("push agent-core: a commit revision is required; never latest")
	}
	if strings.EqualFold(revision, "latest") {
		return "", fmt.Errorf("push agent-core: refusing tag %q (eng01)", revision)
	}
	target := config.RegistryPath() + "/agent-core:" + revision
	started := time.Now()
	steps := [][]string{
		{"gcloud", "auth", "configure-docker", config.Region + "-docker.pkg.dev", "--quiet"},
		{"docker", "tag", localImage, target},
		{"docker", "push", target},
	}
	for _, step := range steps {
		if out, err := run(step[0], step[1:]...); err != nil {
			return "", fmt.Errorf("%s: %w: %s",
				strings.Join(step[:2], " "), err, strings.TrimSpace(string(out)))
		}
	}
	kindrig.LogPhase(config.Cluster, "agent-core-push", "pushed", started, target)
	return target, nil
}
