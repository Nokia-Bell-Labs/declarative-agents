// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// agentCoreModule is the platform module that owns cmd/agent. Every module
// audit that rebuilds the agent binary compiles this same package, so the root
// audit warms its build cache once before fanning out (see warmAgentBuild).
const agentCoreModule = "agent-core"

var subModules = []string{
	agentCoreModule,
	"applications/catalog",
	"design-patterns",
}

// applicationModules are runnable application modules that participate in the root
// audit, Go-test, and stats gates but expose no build or default target. Each
// owns a mage audit target, Go tests, and a stats target under magefiles/.
// Agent-owning applications report implementations; composition-only applications
// report application reuse without contributing duplicate agents. They are
// absent from Build and All because they define no such targets: an application is
// a deployable artifact governed by its own spec corpus, not a platform module.
var applicationModules = []string{
	"applications/chatbot-mesh",
	"applications/coding-agent",
	"applications/agent-architecture",
}

// auditOnlyApplicationModules contains applications that participate in the
// documentation gate before they
// grow Go tests, stats, or build targets.
var auditOnlyApplicationModules = []string{}

// auditParticipants lists every module the root audit gate dispatches to: the
// platform submodules plus every application that owns a mage audit target.
func auditParticipants() []string {
	participants := append(append([]string{}, subModules...), applicationModules...)
	return append(participants, auditOnlyApplicationModules...)
}

// statsParticipants lists every module the root stats target dispatches to:
// the platform submodules plus the application modules. The repo-wide agents total
// sums only "agents" sections; composition-only modules may report a separate
// application section without double-counting canonical agents (GH-754,
// GH-947).
func statsParticipants() []string {
	return append(append([]string{}, subModules...), applicationModules...)
}

type buildRunner func(string) error

// All runs the default mage target in each sub-module (default target).
func All() error {
	for _, mod := range subModules {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := os.Stat(mageDir); os.IsNotExist(err) {
			fmt.Printf("skipping %s (no magefiles/)\n", mod)
			continue
		}
		fmt.Printf("=== %s ===\n", mod)
		cmd := exec.Command("mage")
		cmd.Dir = mod
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("mage in %s: %w", mod, err)
		}
	}
	return nil
}

// Build runs mage build in each sub-module.
func Build() error {
	return buildSubModules(subModules, os.Stat, runMageBuild)
}

func buildSubModules(modules []string, stat statFunc, run buildRunner) error {
	for _, mod := range modules {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := stat(mageDir); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("skipping %s (no magefiles/)\n", mod)
				continue
			}
			return fmt.Errorf("stat %s: %w", mageDir, err)
		}
		fmt.Printf("=== %s build ===\n", mod)
		if err := run(mod); err != nil {
			return fmt.Errorf("build in %s: %w", mod, err)
		}
	}
	return nil
}

func runMageBuild(dir string) error {
	cmd := exec.Command("mage", "build")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
