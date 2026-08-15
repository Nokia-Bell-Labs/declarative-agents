// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package agentbuild holds the one agent-CLI build recipe shared by the
// catalog magefile and the conformance harness. Both previously duplicated the
// production build tag, the ./cmd/agent target, and the go build invocation;
// magefiles is package main, so the recipe lives here as an importable
// non-main package alongside catalogroot (GH-1390).
package agentbuild

import (
	"fmt"
	"io"
	"os/exec"
)

const (
	// BuildTag selects the production tool catalog for the agent CLI.
	BuildTag = "production"
	// target is the agent CLI main package relative to the agent-core root.
	target = "./cmd/agent"
)

// Build compiles the agent CLI from coreRoot to outputPath with the shared
// production recipe. Combined build output is written to logOut when it is
// non-nil (callers stream it live or buffer it to report only on failure).
// outputPath is caller-chosen so the magefile and the harness can keep
// distinct binaries. On success it returns outputPath.
func Build(coreRoot, outputPath string, logOut io.Writer) (string, error) {
	cmd := exec.Command("go", "build", "-tags", BuildTag, "-o", outputPath, target)
	cmd.Dir = coreRoot
	if logOut != nil {
		cmd.Stdout = logOut
		cmd.Stderr = logOut
	}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build agent from %s: %w", coreRoot, err)
	}
	return outputPath, nil
}
