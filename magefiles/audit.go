// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
)

type statFunc func(string) (os.FileInfo, error)
type auditRunner func(string) error

// Audit first validates the repository-wide actor-role realization inventory,
// warms the agent build cache once, then runs mage audit in each sub-module and
// participating example module concurrently.
func Audit() error {
	if err := runAgentRoleRealizationAudit(); err != nil {
		return err
	}
	if err := warmAgentBuild(); err != nil {
		return err
	}
	return auditSubModules(auditParticipants(), os.Stat, runMageAudit)
}

func runAgentRoleRealizationAudit() error {
	cmd := exec.Command("go", "test", ".", "-run", "^TestAgentRoleRealization")
	cmd.Dir = "magefiles"
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("agent role-realization audit: %w", err)
	}
	return nil
}

// warmAgentBuild compiles the agent-core production binary once so that every
// module audit which rebuilds it (buildAgent) links from a warm Go build cache
// rather than racing to compile the same packages cold under parallel dispatch.
// The compiled artifact is discarded; only the cache it primes matters. A
// failure here is a genuine agent-core breakage every agent-building module
// would hit, so it fails fast with one clear error instead of many concurrent
// identical ones.
func warmAgentBuild() error {
	tmp, err := os.MkdirTemp("", "audit-warm-agent-*")
	if err != nil {
		return fmt.Errorf("warm agent build: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	out := filepath.Join(tmp, "agent")
	fmt.Println("warming agent build cache...")
	cmd := exec.Command("go", "build", "-tags", "production", "-o", out, "./cmd/agent")
	cmd.Dir = agentCoreModule
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("warm agent build: %w", err)
	}
	return nil
}

// auditSubModules dispatches mage audit to every runnable module concurrently,
// bounded by auditConcurrency. Module audits are independent processes, so the
// only shared state is the first-error latch. The skip decision runs serially
// first so its messages and stat errors stay deterministic and ordered.
func auditSubModules(modules []string, stat statFunc, run auditRunner) error {
	return auditSubModulesLimited(modules, auditConcurrency(), stat, run)
}

// maxConcurrentModuleAudits is intentionally lower than NumCPU. Each module
// audit runs multi-core Go builds, tests, profile boot smokes, and evidence
// agents; fanning every module out at once starves process-spawning tests whose
// own deadlines are meaningful (#1587).
const maxConcurrentModuleAudits = 2

// auditConcurrency bounds how many module audits run at once.
func auditConcurrency() int {
	n := runtime.NumCPU()
	if n < 1 {
		return 1
	}
	if n < maxConcurrentModuleAudits {
		return n
	}
	return maxConcurrentModuleAudits
}

func auditSubModulesLimited(modules []string, limit int, stat statFunc, run auditRunner) error {
	if limit < 1 {
		limit = 1
	}
	runnable, err := runnableAuditModules(modules, stat)
	if err != nil {
		return err
	}

	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	for _, mod := range runnable {
		wg.Add(1)
		sem <- struct{}{}
		go func(mod string) {
			defer wg.Done()
			defer func() { <-sem }()
			fmt.Printf("=== %s audit ===\n", mod)
			if err := run(mod); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("audit in %s: %w", mod, err)
				}
				mu.Unlock()
			}
		}(mod)
	}
	wg.Wait()
	return firstErr
}

// runnableAuditModules filters modules to those exposing a mage entrypoint,
// preserving the original serial skip semantics: a missing magefiles/ falls
// back to magefile.go, and only IsNotExist for both is a skip. Any other stat
// error aborts before dispatch.
func runnableAuditModules(modules []string, stat statFunc) ([]string, error) {
	var runnable []string
	for _, mod := range modules {
		mageDir := filepath.Join(mod, "magefiles")
		if _, err := stat(mageDir); err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("stat %s: %w", mageDir, err)
			}
			mageFile := filepath.Join(mod, "magefile.go")
			if _, fileErr := stat(mageFile); fileErr != nil {
				if os.IsNotExist(fileErr) {
					fmt.Printf("skipping %s (no magefiles/ or magefile.go)\n", mod)
					continue
				}
				return nil, fmt.Errorf("stat %s: %w", mageFile, fileErr)
			}
		}
		runnable = append(runnable, mod)
	}
	return runnable, nil
}

func runMageAudit(dir string) error {
	cmd := exec.Command("mage", "audit")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
