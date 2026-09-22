// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package apprig

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// dnsLabel is the RFC 1123 label the application name, namespace, release, and
// host segment must all satisfy, so a resolved coordinate is always a legal
// Kubernetes and DNS name.
var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Manifest is the lifecycle-relevant projection of an application's
// agents/application.yaml. It carries only what the runner resolves
// coordinates from; the full manifest owns much more, which apprig does not
// read (#2479 R3).
type Manifest struct {
	SchemaVersion int    `yaml:"schema_version"`
	Application   string `yaml:"application"`
	Ownership     string `yaml:"ownership"`
	Runtime       struct {
		MountPath string `yaml:"mount_path"`
	} `yaml:"runtime"`
	Deployment struct {
		Entries []DeploymentEntry `yaml:"entries"`
	} `yaml:"deployment"`
}

// DeploymentEntry is one workload the application deploys, naming the profile
// root mounted into it.
type DeploymentEntry struct {
	ID          string `yaml:"id"`
	Root        string `yaml:"root"`
	Workload    string `yaml:"workload"`
	ProfilePath string `yaml:"profile_path"`
	MountPath   string `yaml:"mount_path"`
}

// LoadManifest reads and validates one application manifest.
func LoadManifest(path string) (Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, fmt.Errorf("read application manifest %s: %w", path, err)
	}
	var manifest Manifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode application manifest %s: %w", path, err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, fmt.Errorf("application manifest %s: %w", path, err)
	}
	return manifest, nil
}

// Validate reports every structural fault at once so a caller fixes the
// manifest in one pass. It checks the identity the runner keys every
// coordinate on and the deployment entries it derives profile roots from.
func (m Manifest) Validate() error {
	var faults []string
	if m.SchemaVersion == 0 {
		faults = append(faults, "schema_version is required")
	}
	if !dnsLabel.MatchString(m.Application) {
		faults = append(faults, fmt.Sprintf("application %q is not a DNS-1123 label", m.Application))
	}
	if len(m.Deployment.Entries) == 0 {
		faults = append(faults, "deployment.entries is empty; the runner has no workload to place")
	}
	seenWorkload := map[string]bool{}
	for index, entry := range m.Deployment.Entries {
		if entry.Workload == "" {
			faults = append(faults, fmt.Sprintf("deployment.entries[%d] names no workload", index))
		} else if seenWorkload[entry.Workload] {
			faults = append(faults, fmt.Sprintf("deployment.entries workload %q is declared twice", entry.Workload))
		}
		seenWorkload[entry.Workload] = true
		if entry.ProfilePath == "" {
			faults = append(faults, fmt.Sprintf("deployment.entries[%d] (%s) names no profile_path", index, entry.Workload))
		}
	}
	if len(faults) > 0 {
		return fmt.Errorf("invalid: %v", faults)
	}
	return nil
}

// ProfileRoots returns the deployment entries' profile paths in declaration
// order, the roots a package-time closure and the deploy mounts derive from.
func (m Manifest) ProfileRoots() []string {
	roots := make([]string, 0, len(m.Deployment.Entries))
	for _, entry := range m.Deployment.Entries {
		roots = append(roots, entry.ProfilePath)
	}
	return roots
}
