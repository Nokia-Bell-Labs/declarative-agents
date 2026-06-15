// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

const repoRoot = "/Users/djukic/WORKSPACE/proof-of-concepts/agentic-loop/agent-core"

// START OMIT
func main() {
	profile := "agents/knowledge-manager/documentation-curator/profile.yaml"
	StartAgent(profile)
}

// END OMIT

func StartAgent(profile string) {
	cmd := exec.Command("go", "run", "./cmd/agent", "--profile", filepath.Join(repoRoot, profile), "--directory", repoRoot)
	cmd.Dir = repoRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic(err)
	}
}
