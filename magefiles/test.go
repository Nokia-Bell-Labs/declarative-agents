// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Aliases keeps the existing mage test:unit release gate available.
var Aliases = map[string]interface{}{
	"test:unit": TestUnit,
}

type unitTestRunner func(string) error
type moduleTestDetector func(string) (bool, error)

// Test runs unit tests for sub-modules that contain Go tests.
func Test() error {
	return testSubModules(subModules, moduleHasGoTests, runMageTest)
}

// TestUnit runs unit tests for sub-modules with a Go module.
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
