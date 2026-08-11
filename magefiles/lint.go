// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var lintModuleDirs = []string{
	"magefiles",
	"agent-core",
	"agent-core/magefiles",
	"applications/catalog",
	"applications/chatbot-mesh",
	"applications/coding-agent",
	"applications/agent-architecture",
	"applications/prose-editor",
	"design-patterns/magefiles",
}

type lintRunner func(string) error

// Lint runs the pinned golangci-lint v2 policy in every non-fixture Go module,
// including the standalone Mage modules. It preflights the binary so a version
// that cannot read the config schema fails with installation guidance rather than
// a schema error from inside the first module's run (GH-1479).
//
// Lint is a release gate, wired into the recipe in Tag. It could not be one
// before: the policy had never actually run, and its first run reported twelve
// forbidigo findings, which GH-1481 resolved by refactoring or annotating each
// site. The go-style constitution lists every annotated site (GH-1479).
func Lint() error {
	if err := checkGolangciLint(); err != nil {
		return err
	}
	return lintSubModules(lintModuleDirs, runGolangciLint)
}

func lintSubModules(modules []string, run lintRunner) error {
	for _, module := range modules {
		fmt.Printf("=== %s lint ===\n", module)
		if err := run(module); err != nil {
			return fmt.Errorf("lint in %s: %w", module, err)
		}
	}
	return nil
}

func runGolangciLint(dir string) error {
	cmd, err := golangciLintCommand(dir)
	if err != nil {
		return err
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func golangciLintCommand(dir string) (*exec.Cmd, error) {
	root, err := findRepositoryRoot()
	if err != nil {
		return nil, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve repository root for lint cache: %w", err)
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("resolve user cache for golangci-lint: %w", err)
	}
	cacheDir := golangciLintCacheDir(cacheRoot, root)
	cmd := exec.Command("golangci-lint", "run", "./...")
	cmd.Dir = filepath.Join(root, filepath.FromSlash(dir))
	cmd.Env = lintCommandEnvironment(os.Environ(), cacheDir)
	return cmd, nil
}

func golangciLintCacheDir(cacheRoot, repositoryRoot string) string {
	canonical := filepath.Clean(repositoryRoot)
	digest := sha256.Sum256([]byte(canonical))
	namespace := fmt.Sprintf("%x", digest[:12])
	return filepath.Join(
		cacheRoot, "declarative-agents", "golangci-lint", namespace)
}

func lintCommandEnvironment(inherited []string, cacheDir string) []string {
	const key = "GOLANGCI_LINT_CACHE"
	environment := make([]string, 0, len(inherited)+1)
	for _, entry := range inherited {
		if name, _, _ := strings.Cut(entry, "="); name == key {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment, key+"="+cacheDir)
}
