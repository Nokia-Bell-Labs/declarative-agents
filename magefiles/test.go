// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Aliases keeps the existing mage test:unit release gate available.
var Aliases = map[string]interface{}{
	"test:unit": TestUnit,
}

type unitTestRunner func(string) error
type moduleTestDetector func(string) (bool, error)

// testTarget pairs a module directory with the command that runs its tests.
type testTarget struct {
	module string
	run    unitTestRunner
}

// nestedTestModules are standalone Go modules that live inside another module's
// directory tree, so neither `go test ./...` in the parent nor the parent's
// mage test target reaches them; each carries its own go.mod. Without explicit
// dispatch their tests never gate a release (GH-1345): the root magefiles
// (shared kindrig packages), agent-core's magefiles, and the design-patterns
// magefiles (which is the design-patterns module itself, its go.mod nested
// under magefiles).
var nestedTestModules = []string{
	"magefiles",
	"agent-core/magefiles",
	"design-patterns/magefiles",
}

// testTargets is the single registry of module directories the root Test gate
// runs, each paired with its command. Platform sub-modules and the catalog run
// through their Mage test target; other applications and standalone nested
// modules run through `go test`. Every maintained non-fixture Go module must
// appear here exactly once (TestEveryMaintainedGoModuleIsDispatchedExactlyOnce).
func testTargets() []testTarget {
	targets := make([]testTarget, 0,
		len(subModules)+len(applicationModules)+len(nestedTestModules))
	for _, m := range subModules {
		targets = append(targets, testTarget{module: m, run: runMageTest})
	}
	for _, m := range applicationModules {
		run := runGoUnitTests
		if m == "applications/catalog" {
			// Catalog conformance uses its Mage runner so the test binary can be
			// compiled to a stable path before execution (GH-1437).
			run = runMageTest
		}
		targets = append(targets, testTarget{module: m, run: run})
	}
	for _, m := range nestedTestModules {
		targets = append(targets, testTarget{module: m, run: runGoUnitTests})
	}
	return targets
}

// Test runs unit tests for every participating module from the single
// testTargets registry: platform sub-modules and the catalog through their Mage
// test target, other applications and standalone nested modules through go test.
func Test() error {
	for _, target := range testTargets() {
		if err := testSubModules([]string{target.module}, moduleHasGoTests, target.run); err != nil {
			return err
		}
	}
	// Shipped UI reproducibility gate: fail if a tracked dist no longer matches a
	// clean source build (GH-518). Skips cleanly where node/npm is absent.
	return UIDist()
}

// discoverMaintainedGoModules returns every non-fixture Go module directory
// under root (relative, slash-separated), skipping fixture and vendored trees.
// It is the source of truth the orchestration test compares the dispatch
// registry against, so a newly added maintained module cannot silently escape
// the test gate.
func discoverMaintainedGoModules(root string) ([]string, error) {
	var modules []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "testdata", "generated-files":
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != "go.mod" {
			return nil
		}
		rel, relErr := filepath.Rel(root, filepath.Dir(path))
		if relErr != nil {
			return relErr
		}
		modules = append(modules, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(modules)
	return modules, nil
}

// TestUnit is a compatibility target for mage test:unit; use Test for release gates.
func TestUnit() error {
	return testUnitSubModules(subModules, os.Stat, runGoUnitTests)
}

func testSubModules(modules []string, hasTests moduleTestDetector, run unitTestRunner) error {
	for _, mod := range modules {
		ok, err := hasTests(mod)
		if err != nil {
			return fmt.Errorf("discover Go tests in %s: %w", mod, err)
		}
		if !ok {
			fmt.Printf("skipping %s (no Go tests)\n", mod)
			continue
		}
		fmt.Printf("=== %s tests ===\n", mod)
		if err := run(mod); err != nil {
			return fmt.Errorf("tests in %s: %w", mod, err)
		}
	}
	return nil
}

func testUnitSubModules(modules []string, stat statFunc, run unitTestRunner) error {
	for _, mod := range modules {
		goMod := filepath.Join(mod, "go.mod")
		if _, err := stat(goMod); err != nil {
			if os.IsNotExist(err) {
				fmt.Printf("skipping %s (no go.mod)\n", mod)
				continue
			}
			return fmt.Errorf("stat %s: %w", goMod, err)
		}
		fmt.Printf("=== %s unit tests ===\n", mod)
		if err := run(mod); err != nil {
			return fmt.Errorf("unit tests in %s: %w", mod, err)
		}
	}
	return nil
}

func moduleHasGoTests(dir string) (bool, error) {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat %s: %w", filepath.Join(dir, "go.mod"), err)
	}
	found := false
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "generated-files":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	if err == filepath.SkipAll {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return found, err
}

func runMageTest(dir string) error {
	cmd := exec.Command("mage", "test")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func runGoUnitTests(dir string) error {
	cmd := exec.Command("go", "test", "-short", "./...")
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
