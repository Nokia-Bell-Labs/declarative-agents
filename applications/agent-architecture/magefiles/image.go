// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	// Both chart workloads run the shared agent-core image (GH-1368); the
	// application owns no runtime image. This is the published default the chart
	// values reference for the curator, the curator UI init container, and the
	// collector.
	agentArchitectureImageRepository = "ghcr.io/nokia-bell-labs/declarative-agents/agent-core"
	agentArchitectureImageTag        = "0.1.0"
)

// Image groups production agent-architecture image targets.
type Image mg.Namespace

// Build builds the shared agent-core image locally from the pinned agent-core
// checkout so demos and smokes run the code under test rather than a published
// image the environment may not be able to pull. The application bakes no image
// of its own: profiles are mounted and the curator UI is delivered as ConfigMaps
// (GH-1368).
func (Image) Build() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	return kindrig.BuildAgentCoreImage(resolved.Core, kindrig.DefaultAgentCoreImage)
}
