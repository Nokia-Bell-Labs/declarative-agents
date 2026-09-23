// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package imageinventory classifies every image family the repository owns or
// pins and audits active paths for prohibited identity forms (ENG01 C7, GH-2518).
package imageinventory

import (
	"fmt"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/pinsurvey"
)

// Family is one registered image family with a visible owner and retention policy.
type Family struct {
	ID        string
	Class     string
	Owner     string
	Retention string
}

// Families is the authoritative inventory of image families this repository
// manages. Every pin the survey collects and every local identity the rig
// builds must classify into one entry (srd006 R1.1, ENG01 table 7).
var Families = []Family{
	{ID: "agent-core-published", Class: "published", Owner: "README.md (one agent image)", Retention: "release tags; not removed by clean:images"},
	{ID: "agent-core-runtime-local", Class: "runtime", Owner: "docs/engineering/eng01-kind-test-demo-rig.yaml", Retention: "per-cluster lease; last owner removes managed tag"},
	{ID: "integration-test-local", Class: "test", Owner: "docs/engineering/eng01-kind-test-demo-rig.yaml", Retention: "per-cluster lease"},
	{ID: "derived-upstream-local", Class: "derived", Owner: "docs/engineering/eng01-kind-test-demo-rig.yaml", Retention: "per-cluster lease"},
	{ID: "materialization-cache-local", Class: "cache", Owner: "docs/engineering/eng01-kind-test-demo-rig.yaml", Retention: "per-cluster lease until cluster down"},
	{ID: "chart-third-party", Class: "upstream", Owner: "applications/*/helm/values.yaml", Retention: "digest-pinned; bounded by pin survey"},
	{ID: "rig-upstream-pin", Class: "upstream", Owner: "magefiles/pinsurvey/pins.yaml", Retention: "configured pin; clean:images retains current pins"},
	{ID: "kind-node", Class: "upstream", Owner: "magefiles/kindrig/platform-kind-config.yaml", Retention: "pin while clusters exist"},
	{ID: "agent-core-build", Class: "upstream", Owner: "agent-core/Dockerfile", Retention: "build-only; not a cluster runtime"},
}

// ClassifyReference maps one image reference string to a registered family.
func ClassifyReference(reference string) (Family, error) {
	ref := strings.TrimSpace(reference)
	if ref == "" {
		return Family{}, fmt.Errorf("empty reference")
	}
	lower := strings.ToLower(ref)
	if strings.HasPrefix(lower, "ghcr.io/nokia-bell-labs/declarative-agents/") ||
		strings.Contains(lower, "-docker.pkg.dev/") && strings.Contains(lower, "/declarative-agents/") {
		return familyByID("agent-core-published")
	}
	if local, err := kindrig.ParseLocal(ref); err == nil {
		switch local.Role {
		case kindrig.RuntimeRole:
			return familyByID("agent-core-runtime-local")
		case kindrig.TestRole:
			return familyByID("integration-test-local")
		case kindrig.DerivedRole:
			return familyByID("derived-upstream-local")
		case kindrig.CacheRole:
			return familyByID("materialization-cache-local")
		}
	}
	if upstream, err := kindrig.ParseUpstream(ref); err == nil {
		path := strings.ToLower(upstream.Registry + "/" + upstream.Path)
		if strings.HasPrefix(path, "kindest/") {
			return familyByID("kind-node")
		}
		for _, pin := range kindrig.ConfiguredUpstreamPins() {
			if strings.HasPrefix(strings.ToLower(pin), path) {
				return familyByID("rig-upstream-pin")
			}
		}
		if strings.HasPrefix(path, "docker.io/docker/dockerfile") ||
			(strings.HasPrefix(path, "docker.io/library/alpine") && upstream.Tag == "3.22") {
			return familyByID("agent-core-build")
		}
		return familyByID("chart-third-party")
	}
	return Family{}, fmt.Errorf("unregistered image reference %q", ref)
}

// ClassifyPin maps a survey pin to a family using its image repository.
func ClassifyPin(pin pinsurvey.Pin) (Family, error) {
	ref := pin.Image
	if pin.Tag != "" {
		ref += ":" + pin.Tag
	}
	if pin.Digest != "" {
		if !strings.HasPrefix(pin.Digest, "sha256:") {
			ref += "@sha256:" + pin.Digest
		} else {
			ref += "@" + pin.Digest
		}
	}
	return ClassifyReference(ref)
}

func familyByID(id string) (Family, error) {
	for _, family := range Families {
		if family.ID == id {
			return family, nil
		}
	}
	return Family{}, fmt.Errorf("unknown family id %q", id)
}

// GrammarFinding reports one prohibited identity in an active path.
type GrammarFinding struct {
	Path   string
	Line   int
	Image  string
	Rule   string
	Detail string
}
