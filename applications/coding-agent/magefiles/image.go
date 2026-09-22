// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	// Every coding role runs the canonical, profile-free agent-core image.
	// Executor-only Go and lint dependencies arrive from chart-managed donors.
	codingAgentImageRepository = "ghcr.io/nokia-bell-labs/declarative-agents/agent-core"
	codingAgentImageTag        = "0.1.0"
)

// Image groups production coding-agent image targets.
type Image mg.Namespace

// Build builds the canonical profile-free agent-core image used by every chart
// role. There is no application-specific or toolchain-layered runtime image.
func (Image) Build() error {
	applicationRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := codingAgentCoreRoot(applicationRoot)
	return kindrig.BuildAgentCoreImage(coreRoot, demoImage(applicationRoot))
}

// codingAgentCoreRoot resolves the canonical agent-core checkout two levels up
// from an application root.
func codingAgentCoreRoot(applicationRoot string) string {
	return filepath.Clean(filepath.Join(applicationRoot, "..", "..", "agent-core"))
}
