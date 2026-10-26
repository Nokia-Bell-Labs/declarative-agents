// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause
package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// parentRepository is the parent corpus whose specification-critic agent
// audits this repository (docs/guides/corpus-audit.md).
const parentRepository = "github.com/Nokia-Bell-Labs/declarative-agents"

const (
	checkoutsFile = "docs/external-checkouts.yaml"
	auditProfile  = "applications/catalog/agents/specification-critic/corpus-audit-profile.yaml"
)

type commandRunner func(dir string, name string, args ...string) error

// runtimeGates are agent-core's gates that hold files this application
// changes to the monorepo's rules: declaration and Go size, package shape,
// and the Markdown corpus's canonical form (GH-112).
var runtimeGates = []string{"./internal/declstyle/", "./internal/gostyle/", "./internal/boundaries/", "./pkg/spec/markdown/"}

// Audit runs the repository gate tests, agent-core's gates over the
// monorepo, the parent's specification-critic corpus-audit agent over the
// repository, and the runtime's --validate-config over every shipped
// profile (GH-135).
func Audit() error {
	root, err := repositoryRoot()
	if err != nil {
		return err
	}
	if err := runStreaming(root, "go", "test", "./magefiles/...", "-count=1"); err != nil {
		return fmt.Errorf("repository gate tests: %w", err)
	}
	if err := runRuntimeGates(root, runStreaming); err != nil {
		return err
	}
	if err := runCorpusAudit(root, runStreaming); err != nil {
		return err
	}
	return validateProfiles(root, runStreaming)
}

// runRuntimeGates runs agent-core's gates when the application sits in the
// monorepo beside it, so a change here that breaks them fails the audit
// rather than the next change to agent-core (GH-112). Outside the monorepo
// there is no agent-core to run.
func runRuntimeGates(root string, run commandRunner) error {
	core := filepath.Join(root, "..", "..", "agent-core")
	if _, err := os.Stat(filepath.Join(core, "go.mod")); err != nil {
		return nil
	}
	if err := run(core, "go", append([]string{"test", "-count=1"}, runtimeGates...)...); err != nil {
		return fmt.Errorf("agent-core gates: %w", err)
	}
	return nil
}

// validateProfiles holds every shipped declarative-agents profile under
// plugin/agents to the pinned runtime: the same boot smoke a rollout runs,
// so a profile the runtime would refuse fails the gate (GH-135, srd036 R6.1).
func validateProfiles(root string, run commandRunner) error {
	profiles, err := filepath.Glob(filepath.Join(root, "plugin", "agents", "*", "profile.yaml"))
	if err != nil {
		return err
	}
	if len(profiles) == 0 {
		return nil
	}
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	for _, p := range profiles {
		if err := run(root, rel.Agent, "--profile", p, "--validate-config", "--core-root", rel.CoreRoot()); err != nil {
			return fmt.Errorf("profile %s fails the pinned runtime's validation: %w", p, err)
		}
	}
	return nil
}

// runCorpusAudit runs this repository's agent over root, with derived_from
// entries resolving against the repository's own source (GH-190).
func runCorpusAudit(root string, run commandRunner) error {
	rel, err := repoRuntime()
	if err != nil {
		return err
	}
	restore, err := withCheckouts(root, rel.Src, parentRepository)
	if err != nil {
		return err
	}
	defer restore()
	out, err := os.MkdirTemp("", "scrum-master-audit-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(out)
	err = run(root, rel.Agent,
		"--profile", filepath.Join(rel.Src, auditProfile),
		"--directory", root,
		"--core-root", filepath.Join(rel.Src, "agent-core"),
		"--output", out,
	)
	if err != nil {
		return fmt.Errorf("specification corpus audit failed: %w", err)
	}
	return nil
}

// repositoryRoot is the application's root: the directory holding its
// go.mod, which inside the monorepo is applications/scrum-master, not the git
// toplevel (GH-190).
func repositoryRoot() (string, error) {
	out, err := exec.Command("go", "env", "GOMOD").Output()
	gomod := strings.TrimSpace(string(out))
	if err != nil || gomod == "" || gomod == os.DevNull {
		return "", errors.New("not inside the application's module")
	}
	return filepath.Dir(gomod), nil
}

func runStreaming(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
