// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	conformanceTempPattern = "catalog-conformance-*"
	conformanceTestTimeout = "10m"
	defaultLiveTimeout     = "5m"
)

type catalogCommand struct {
	name           string
	args           []string
	dir            string
	stdout, stderr io.Writer
}

type catalogCommandRunner func(catalogCommand) error

type catalogTestRunner struct {
	catalogRoot string
	stdout      io.Writer
	stderr      io.Writer
	runCommand  catalogCommandRunner
	mkdirTemp   func(string, string) (string, error)
	removeAll   func(string) error
}

type conformanceOptions struct {
	live        bool
	liveTimeout string
}

type catalogTestMode struct {
	nonConformance bool
	conformance    conformanceOptions
}

// Test runs Go unit tests for the catalog.
func Test() error {
	return runCatalogTestMode(catalogTestMode{
		nonConformance: true,
		conformance:    conformanceOptions{live: false},
	})
}

// Conformance runs the deterministic per-family profile conformance gate. It
// includes static, protocol, and profile tests and disables live inference even
// when the caller's shell has retained the live-conformance opt-in.
func Conformance() error {
	return runCatalogTestMode(catalogTestMode{
		conformance: conformanceOptions{live: false},
	})
}

// LiveConformance explicitly enables conformance paths that perform inference
// against the exact Ollama models declared by each test. Dependency checks still
// skip unavailable models; direct go test callers can override the five-minute
// per-run timeout with -args -live=true -live-timeout=<duration>.
func LiveConformance() error {
	return runCatalogTestMode(catalogTestMode{
		conformance: conformanceOptions{
			live:        true,
			liveTimeout: defaultLiveTimeout,
		},
	})
}

func runCatalogTestMode(mode catalogTestMode) error {
	catalogRoot, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve catalog working directory: %w", err)
	}
	runner := catalogTestRunner{
		catalogRoot: catalogRoot,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		runCommand:  executeCatalogCommand,
		mkdirTemp:   os.MkdirTemp,
		removeAll:   os.RemoveAll,
	}
	if mode.nonConformance {
		if err := runner.runNonConformance(); err != nil {
			return err
		}
	}
	return runner.runConformance(mode.conformance)
}

func executeCatalogCommand(invocation catalogCommand) error {
	cmd := exec.Command(invocation.name, invocation.args...)
	cmd.Dir = invocation.dir
	cmd.Stdout = invocation.stdout
	cmd.Stderr = invocation.stderr
	return cmd.Run()
}

func (runner catalogTestRunner) runNonConformance() error {
	packages, err := runner.nonConformancePackages()
	if err != nil {
		return err
	}
	if len(packages) == 0 {
		fmt.Fprintln(runner.stdout, "no non-conformance catalog packages to test")
		return nil
	}
	fmt.Fprintln(runner.stdout, "=== catalog non-conformance tests ===")
	return runner.runPhase("run non-conformance catalog tests", catalogCommand{
		name: "go",
		args: append([]string{"test"}, packages...),
		dir:  runner.catalogRoot,
	})
}

func (runner catalogTestRunner) nonConformancePackages() ([]string, error) {
	var stdout, stderr bytes.Buffer
	err := runner.runCommand(catalogCommand{
		name:   "go",
		args:   []string{"list", "./..."},
		dir:    runner.catalogRoot,
		stdout: &stdout,
		stderr: io.MultiWriter(runner.stderr, &stderr),
	})
	if err != nil {
		return nil, commandFailure("list catalog packages", err, stdout.String(), stderr.String())
	}

	var packages []string
	for _, packagePath := range strings.Fields(stdout.String()) {
		if packagePath == "conformance" || strings.HasSuffix(packagePath, "/conformance") {
			continue
		}
		packages = append(packages, packagePath)
	}
	return packages, nil
}

func (runner catalogTestRunner) runConformance(options conformanceOptions) (returnErr error) {
	tempDir, err := runner.mkdirTemp("", conformanceTempPattern)
	if err != nil {
		return fmt.Errorf("create stable conformance test directory: %w", err)
	}
	defer func() {
		if err := runner.removeAll(tempDir); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("clean conformance test directory %s: %w", tempDir, err))
		}
	}()

	binary := filepath.Join(tempDir, "conformance.test")
	fmt.Fprintln(runner.stdout, "=== catalog conformance compile ===")
	if err := runner.runPhase("compile catalog conformance test binary", catalogCommand{
		name: "go",
		args: []string{"test", "-c", "-o", binary, "./conformance"},
		dir:  runner.catalogRoot,
	}); err != nil {
		return err
	}

	args := []string{
		"-test.timeout=" + conformanceTestTimeout,
		"-test.count=1",
		fmt.Sprintf("-live=%t", options.live),
	}
	if options.live {
		args = append(args, "-live-timeout="+options.liveTimeout)
	}
	fmt.Fprintln(runner.stdout, "=== catalog conformance execution ===")
	return runner.runPhase("execute catalog conformance test binary", catalogCommand{
		name: binary,
		args: args,
		dir:  filepath.Join(runner.catalogRoot, "conformance"),
	})
}

func (runner catalogTestRunner) runPhase(label string, invocation catalogCommand) error {
	var stdout, stderr bytes.Buffer
	invocation.stdout = io.MultiWriter(runner.stdout, &stdout)
	invocation.stderr = io.MultiWriter(runner.stderr, &stderr)
	if err := runner.runCommand(invocation); err != nil {
		return commandFailure(label, err, stdout.String(), stderr.String())
	}
	return nil
}

func commandFailure(label string, err error, stdout, stderr string) error {
	var output strings.Builder
	if stdout != "" {
		fmt.Fprintf(&output, "\nstdout:\n%s", stdout)
	}
	if stderr != "" {
		fmt.Fprintf(&output, "\nstderr:\n%s", stderr)
	}
	return fmt.Errorf("%s: %w%s", label, err, output.String())
}
