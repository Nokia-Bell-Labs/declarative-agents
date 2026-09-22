// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

// CLEAN groups repository-wide extensions under the existing clean target.
type CLEAN mg.Namespace

// commitImageFamilies are host repositories clean:images may delete by
// untyped 12-hex tag. Typed localhost/declarative-agents references are
// owned by image leases, not this sweeper. :local tags, third-party images,
// and every other repository are never touched. The applier families retired
// with the CLI donor (GH-2222); their remaining local copies are removed by
// hand with docker image rm.
var commitImageFamilies = []string{
	"ghcr.io/nokia-bell-labs/declarative-agents/agent-core",
}

const (
	rigIdentityLabelPrefix = "io.declarative-agents."
	imageSourceLabel       = "org.opencontainers.image.source"
	rigImageSource         = "declarative-agents"
)

var commitImageTag = regexp.MustCompile(`^[0-9a-f]{12}$`)
var commitImageLeaseStatus = kindrig.ImageLeaseStatus

// Images removes commit-tagged local images older than the newest keep
// revisions of each rig image family (GH-2215). Revisions are ordered by image
// creation time. The identity labels the rig's image builds write confirm an
// image is rig-built; an unlabeled image falls back to the commit-tag pattern;
// a labeled image naming another source is ambiguous and kept. Images a
// container still uses fail to remove and are reported. No release gate runs
// this; it is an operator target.
func (CLEAN) Images(keep int) error {
	return cleanCommitImages(dockerImageRunner, keep, false)
}

// ImagesDryRun lists what clean:images would remove, removing nothing.
func (CLEAN) ImagesDryRun(keep int) error {
	return cleanCommitImages(dockerImageRunner, keep, true)
}

// ImageLeaseRecover explicitly recovers a force-killed integration's lease.
// It retains the image-ID and container-use guards; unlike normal cleanup it
// intentionally disregards recorded owners, so operators must name the exact
// canonical reference after confirming those owners are dead.
func (CLEAN) ImageLeaseRecover(reference string) error {
	if !strings.HasPrefix(reference,
		"ghcr.io/nokia-bell-labs/declarative-agents/agent-core:") {
		return fmt.Errorf("clean:imageLeaseRecover accepts only canonical agent-core references, got %q",
			reference)
	}
	return kindrig.RecoverAgentCoreImageLease(reference)
}

type imageCommandRunner func(args ...string) ([]byte, error)

func dockerImageRunner(args ...string) ([]byte, error) {
	return exec.Command("docker", args...).CombinedOutput()
}

type commitImage struct {
	ref     string
	created time.Time
	labels  map[string]string
}

func cleanCommitImages(run imageCommandRunner, keep int, dryRun bool) error {
	if keep < 1 {
		return fmt.Errorf("clean:images keeps at least one revision per family, got %d", keep)
	}
	var removals []string
	for _, family := range commitImageFamilies {
		images, err := listCommitImages(run, family)
		if err != nil {
			return err
		}
		removals = append(removals, selectImageRemovals(images, keep)...)
	}
	verb := "removing"
	if dryRun {
		verb = "would remove"
	}
	fmt.Printf("clean:images: %s %d commit-tagged image(s), keeping the newest %d revision(s) per family\n",
		verb, len(removals), keep)
	var failures []error
	for _, ref := range removals {
		fmt.Printf("  %s %s\n", verb, ref)
		if dryRun {
			continue
		}
		if output, err := run("image", "rm", ref); err != nil {
			failures = append(failures, fmt.Errorf("remove %s: %w: %s",
				ref, err, strings.TrimSpace(string(output))))
		}
	}
	return errors.Join(failures...)
}

// listCommitImages returns the family's tags shaped like a commit revision,
// with the creation time and revision label of the image each names.
func listCommitImages(run imageCommandRunner, family string) ([]commitImage, error) {
	output, err := run("image", "ls", "--format", "{{.Tag}}", family)
	if err != nil {
		return nil, fmt.Errorf("list %s images: %w: %s", family, err, strings.TrimSpace(string(output)))
	}
	var images []commitImage
	for _, tag := range strings.Fields(string(output)) {
		if !commitImageTag.MatchString(tag) {
			continue
		}
		ref := family + ":" + tag
		detail, err := run("image", "inspect", "--format",
			"{{.Created}}|{{json .Config.Labels}}", ref)
		if err != nil {
			return nil, fmt.Errorf("inspect %s: %w: %s", ref, err, strings.TrimSpace(string(detail)))
		}
		created, rawLabels, _ := strings.Cut(strings.TrimSpace(string(detail)), "|")
		when, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("inspect %s: creation time %q: %w", ref, created, err)
		}
		var labels map[string]string
		if err := json.Unmarshal([]byte(rawLabels), &labels); err != nil {
			return nil, fmt.Errorf("inspect %s: labels %q: %w", ref, rawLabels, err)
		}
		images = append(images, commitImage{ref: ref, created: when, labels: labels})
	}
	return images, nil
}

// selectImageRemovals keeps the newest keep revisions and returns the rest,
// oldest last. An image whose provenance is ambiguous is kept and reported.
func selectImageRemovals(images []commitImage, keep int) []string {
	var candidates []commitImage
	for _, image := range images {
		if !rigBuiltImage(image.labels) {
			fmt.Printf("clean:images: keeping %s: labels name source %q, not a rig build\n",
				image.ref, image.labels[imageSourceLabel])
			continue
		}
		if active, diagnostic := commitImageLeaseStatus(image.ref); active {
			fmt.Printf("clean:images: keeping %s: %s\n", image.ref, diagnostic)
			continue
		}
		candidates = append(candidates, image)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].created.After(candidates[j].created)
	})
	if len(candidates) <= keep {
		return nil
	}
	removals := make([]string, 0, len(candidates)-keep)
	for _, image := range candidates[keep:] {
		removals = append(removals, image.ref)
	}
	return removals
}

// rigBuiltImage reports whether an image's labels place it in the rig. The
// identity labels written by the agent-core, applier, and toolchain builds are
// conclusive; an OCI source naming this repository also counts; an unlabeled
// image falls back to its commit-shaped tag. Any other labeled image is ambiguous.
func rigBuiltImage(labels map[string]string) bool {
	if len(labels) == 0 {
		return true
	}
	for key := range labels {
		if strings.HasPrefix(key, rigIdentityLabelPrefix) {
			return true
		}
	}
	return strings.Contains(labels[imageSourceLabel], rigImageSource)
}
