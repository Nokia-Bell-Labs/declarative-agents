// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/applications/catalog/catalogroot"
)

// catalogOwnerRoot resolves the command's startup directory once and verifies
// that catalog Mage targets are being run from their owner root.
func catalogOwnerRoot(owner string) (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("%s: resolve catalog owner root: %w", owner, err)
	}
	resolution, err := catalogroot.Resolve(owner, cwd, cwd)
	if err != nil {
		return "", fmt.Errorf("%s; run the command from applications/catalog", err)
	}
	return resolution.Path, nil
}

// resolveAgentCoreRoot returns one absolute Agent Core checkout path. An
// explicit AGENT_CORE_ROOT is interpreted against the catalog owner root;
// otherwise the monorepo's ../../agent-core path is used.
func resolveAgentCoreRoot(catalogRoot string) (string, error) {
	candidate := strings.TrimSpace(os.Getenv(agentCoreRootEnv))
	source := agentCoreRootEnv
	if candidate == "" {
		candidate = filepath.Join(catalogRoot, "..", "..", "agent-core")
		source = "repository discovery"
	} else if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(catalogRoot, candidate)
	}
	candidate, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return "", fmt.Errorf("resolve %s path %q: %w", source, candidate, err)
	}
	return candidate, nil
}
