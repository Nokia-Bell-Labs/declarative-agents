// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"path/filepath"
	"testing"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profileaudit"
)

func TestCodingAgentProfileTimeoutEnvelopes(t *testing.T) {
	roots := codingPolicyRoots(t)
	manifest, err := readApplicationProfileManifestWithCatalog(
		filepath.Join(roots.Application, filepath.FromSlash(profileManifestPath)),
		roots.Profiles,
	)
	if err != nil {
		t.Fatal(err)
	}
	profiles := stageCodingTimeoutProfiles(t, roots, manifest)
	for _, profile := range profiles {
		if err := profileaudit.ValidateWithOptions(
			profile, profileaudit.Options{CoreRoot: roots.Core},
		); err != nil {
			t.Errorf("%s: %v", profile, err)
		}
	}

	requestProfile, requestMachine := false, false
	for _, profile := range profiles[len(manifest.Catalog.References):] {
		report, err := profileaudit.InspectWithOptions(
			profile, profileaudit.Options{CoreRoot: roots.Core},
		)
		if err != nil {
			t.Fatal(err)
		}
		for _, operation := range report.Operations {
			if filepath.Base(operation.Profile) == "request-profile.yaml" {
				requestProfile = true
			}
			if filepath.Base(operation.Machine) == "request-machine.yaml" {
				requestMachine = true
			}
		}
	}
	if !requestProfile || !requestMachine {
		t.Errorf(
			"serving closures omitted request variants: profile=%t machine=%t",
			requestProfile, requestMachine,
		)
	}
	t.Logf(
		"validated %d coding profiles (%d canonical, %d serving)",
		len(profiles), len(manifest.Catalog.References), len(manifest.Deployment.Entries),
	)
}

func codingPolicyRoots(t *testing.T) integrationRoots {
	t.Helper()
	application, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	profiles, err := resolveCatalogRoot("coding-agent timeout policy", application)
	if err != nil {
		t.Fatal(err)
	}
	repository := filepath.Clean(filepath.Join(application, "..", ".."))
	core, err := absoluteOwnerPath(
		loadDemoConfigOrEmpty(application).CoreRoot,
		application,
		filepath.Join(repository, "agent-core"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return integrationRoots{Application: application, Core: core, Profiles: profiles}
}

func stageCodingTimeoutProfiles(
	t *testing.T,
	roots integrationRoots,
	manifest applicationProfileManifest,
) []string {
	t.Helper()
	source, err := inspectPackageSource(roots.Profiles, manifest.Catalog.CompatibleRelease)
	if err != nil {
		t.Fatal(err)
	}
	stage := t.TempDir()
	canonicalRoot := filepath.Join(stage, "profiles")
	if _, err := assembleProfileClosure(
		manifest, roots.Profiles, canonicalRoot, source,
	); err != nil {
		t.Fatal(err)
	}
	deploymentRoot := filepath.Join(stage, "deployment")
	shards, err := packageServingDeployment(
		roots.Application, roots.Profiles, deploymentRoot, manifest, source,
	)
	if err != nil {
		t.Fatal(err)
	}
	profiles := make([]string, 0, len(manifest.Catalog.References)+len(shards))
	for _, ref := range manifest.Catalog.References {
		profiles = append(profiles, filepath.Join(canonicalRoot, filepath.FromSlash(ref.RuntimePath)))
	}
	for _, shard := range shards {
		profiles = append(profiles,
			filepath.Join(deploymentRoot, shard.Path, filepath.FromSlash(shard.Profile)))
	}
	return profiles
}
