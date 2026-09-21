// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/gcprig"
	"github.com/magefile/mage/mg"
)

// GCP groups the cloud rig verbs (eng08-gcp-demo-rig). Uppercase like CLEAN
// and STATS so the namespace reads gcp: on the command line.
type GCP mg.Namespace

func gcpConfig() (gcprig.Config, error) {
	root, err := os.Getwd()
	if err != nil {
		return gcprig.Config{}, err
	}
	return gcprig.Load(root)
}

// Up provisions or reuses the Autopilot cluster, the objectstore bucket,
// the workload-identity binding, and the Artifact Registry from gcp.yaml,
// observing readiness and leaving everything running (eng08 Table 2).
func (GCP) Up() error {
	config, err := gcpConfig()
	if err != nil {
		return err
	}
	return gcprig.Up(gcprig.DefaultRun, config)
}

// Down deletes only the configured names, cluster last. It requires the
// typed confirmation argument eng08 documents, because a shared project may
// hold resources the rig did not create.
func (GCP) Down(confirmation string) error {
	config, err := gcpConfig()
	if err != nil {
		return err
	}
	return gcprig.Down(gcprig.DefaultRun, config, confirmation)
}

// PushAgentCore tags a locally built agent-core image with the given commit
// revision and pushes it to the configured registry; latest is refused
// (eng01).
func (GCP) PushAgentCore(localImage, revision string) error {
	config, err := gcpConfig()
	if err != nil {
		return err
	}
	if err := gcprig.Preflight(gcprig.DefaultRun, config); err != nil {
		return err
	}
	target, err := gcprig.PushAgentCore(gcprig.DefaultRun, config, localImage, revision)
	if err != nil {
		return err
	}
	fmt.Printf("pushed %s\n", target)
	return nil
}

// MirrorDonor copies the digest-pinned CLI donor into the configured
// registry and prints the mirror reference to pin in gcp-values under
// applier.cliDonor.image.
func (GCP) MirrorDonor() error {
	config, err := gcpConfig()
	if err != nil {
		return err
	}
	if err := gcprig.Preflight(gcprig.DefaultRun, config); err != nil {
		return err
	}
	reference, err := gcprig.MirrorDonor(gcprig.DefaultRun, config)
	if err != nil {
		return err
	}
	fmt.Printf("mirrored donor: %s\n", reference)
	return nil
}
