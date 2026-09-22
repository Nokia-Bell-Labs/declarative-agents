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

const (
	rigIdentityLabelPrefix = "io.declarative-agents."
	imageSourceLabel       = "org.opencontainers.image.source"
	rigImageSource         = "declarative-agents"
	retiredAgentCorePrefix = "ghcr.io/nokia-bell-labs/declarative-agents/agent-core:"
)

var (
	commitImageTag          = regexp.MustCompile(`^[0-9a-f]{12}$`)
	commitImageLeaseStatus  = kindrig.ImageLeaseStatus
	cleanImageReconcile     = kindrig.ReconcileImageLeases
	isConfiguredUpstreamPin = kindrig.IsConfiguredUpstreamPin
)

// Images recovers host copies the lifecycle classification no longer retains
// (GH-2510). It reconciles leases against live kind clusters, removes unleased
// typed local images while keeping the newest keep revisions of each
// role/component, retains configured upstream pins, and diagnoses retired
// kindrig or untyped leftovers for explicit recovery. It is not docker image
// prune. No release gate runs it.
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
// typed local reference after confirming those owners are dead. Upstream pins
// are not recovered this way.
func (CLEAN) ImageLeaseRecover(reference string) error {
	class, normalized, err := kindrig.ClassifyLeaseImage(reference)
	if err != nil {
		return fmt.Errorf("clean:imageLeaseRecover: %w", err)
	}
	if class == kindrig.UpstreamImageClass {
		return fmt.Errorf("clean:imageLeaseRecover does not remove upstream pins, got %s", normalized)
	}
	return kindrig.RecoverAgentCoreImageLease(normalized)
}

type imageCommandRunner func(args ...string) ([]byte, error)

func dockerImageRunner(args ...string) ([]byte, error) {
	return exec.Command("docker", args...).CombinedOutput()
}

type commitImage struct {
	ref     string
	family  string
	created time.Time
	labels  map[string]string
}

func cleanCommitImages(run imageCommandRunner, keep int, dryRun bool) error {
	if keep < 1 {
		return fmt.Errorf("clean:images keeps at least one revision per family, got %d", keep)
	}
	if err := cleanImageReconcile(); err != nil {
		return fmt.Errorf("clean:images: reconcile image leases: %w", err)
	}
	listed, err := listHostImageReferences(run)
	if err != nil {
		return err
	}
	var (
		locals   []commitImage
		failures []error
	)
	for _, ref := range listed {
		if diagnosis := retiredImageDiagnosis(ref); diagnosis != "" {
			fmt.Printf("clean:images: diagnose %s: %s\n", ref, diagnosis)
			continue
		}
		if isConfiguredUpstreamPin(ref) {
			fmt.Printf("clean:images: keeping %s: configured upstream pin\n", ref)
			continue
		}
		local, err := kindrig.ParseLocal(ref)
		if err != nil {
			continue
		}
		detail, err := inspectHostImage(run, ref)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		detail.family = string(local.Role) + "/" + local.Component
		locals = append(locals, detail)
	}
	removals := selectImageRemovals(locals, keep)
	verb := "removing"
	if dryRun {
		verb = "would remove"
	}
	fmt.Printf("clean:images: %s %d typed local image(s), keeping the newest %d revision(s) per role/component\n",
		verb, len(removals), keep)
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

func listHostImageReferences(run imageCommandRunner) ([]string, error) {
	output, err := run("image", "ls", "--format", "{{.Repository}}:{{.Tag}}")
	if err != nil {
		return nil, fmt.Errorf("list host images: %w: %s", err, strings.TrimSpace(string(output)))
	}
	var refs []string
	seen := map[string]bool{}
	for _, ref := range strings.Fields(string(output)) {
		if ref == "" || strings.Contains(ref, "<none>") || seen[ref] {
			continue
		}
		seen[ref] = true
		refs = append(refs, ref)
	}
	return refs, nil
}

func inspectHostImage(run imageCommandRunner, ref string) (commitImage, error) {
	detail, err := run("image", "inspect", "--format",
		"{{.Created}}|{{json .Config.Labels}}", ref)
	if err != nil {
		return commitImage{}, fmt.Errorf("inspect %s: %w: %s", ref, err, strings.TrimSpace(string(detail)))
	}
	created, rawLabels, _ := strings.Cut(strings.TrimSpace(string(detail)), "|")
	when, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return commitImage{}, fmt.Errorf("inspect %s: creation time %q: %w", ref, created, err)
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(rawLabels), &labels); err != nil {
		return commitImage{}, fmt.Errorf("inspect %s: labels %q: %w", ref, rawLabels, err)
	}
	return commitImage{ref: ref, created: when, labels: labels}, nil
}

func retiredImageDiagnosis(ref string) string {
	if diagnosis := kindrig.KindrigAliasDiagnosis(ref); diagnosis != "" {
		return diagnosis
	}
	if strings.HasPrefix(ref, "localhost/declarative-agents/") {
		if _, err := kindrig.ParseLocal(ref); err != nil {
			return "retired untyped local reference; recover with docker image rm after confirming no cluster uses it"
		}
	}
	if strings.HasPrefix(ref, retiredAgentCorePrefix) {
		tag, _, _ := strings.Cut(strings.TrimPrefix(ref, retiredAgentCorePrefix), "@")
		if commitImageTag.MatchString(tag) {
			return "retired untyped commit tag; recover with docker image rm after confirming no cluster uses it"
		}
	}
	return ""
}

// selectImageRemovals keeps the newest keep revisions of each role/component
// and returns the rest, oldest last. An image whose provenance is ambiguous,
// that an active lease still names, or that is a configured pin is kept.
func selectImageRemovals(images []commitImage, keep int) []string {
	byFamily := map[string][]commitImage{}
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
		byFamily[image.family] = append(byFamily[image.family], image)
	}
	var removals []string
	for _, family := range sortedFamilyKeys(byFamily) {
		candidates := byFamily[family]
		sort.SliceStable(candidates, func(i, j int) bool {
			return candidates[i].created.After(candidates[j].created)
		})
		if len(candidates) <= keep {
			continue
		}
		for _, image := range candidates[keep:] {
			removals = append(removals, image.ref)
		}
	}
	return removals
}

func sortedFamilyKeys(byFamily map[string][]commitImage) []string {
	keys := make([]string, 0, len(byFamily))
	for key := range byFamily {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// rigBuiltImage reports whether an image's labels place it in the rig. The
// identity labels written by the agent-core, applier, and toolchain builds are
// conclusive; an OCI source naming this repository also counts; an unlabeled
// image falls back to its typed local tag. Any other labeled image is ambiguous.
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
