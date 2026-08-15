// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	stableTestPattern = "catalog-evidence-conformance-*"
	testTimeout       = "10m"
)

type evidenceMode int

const (
	inventoryMode evidenceMode = iota
	runMode
)

type evidenceCommand struct {
	name           string
	args           []string
	dir            string
	stdout, stderr io.Writer
}

type evidenceRunner struct {
	root       string
	stdout     io.Writer
	stderr     io.Writer
	runCommand func(evidenceCommand) error
	mkdirTemp  func(string, string) (string, error)
	removeAll  func(string) error
}

func newEvidenceRunner() evidenceRunner {
	return evidenceRunner{
		root:       ".",
		stdout:     os.Stdout,
		stderr:     os.Stderr,
		runCommand: executeEvidenceCommand,
		mkdirTemp:  os.MkdirTemp,
		removeAll:  os.RemoveAll,
	}
}

func executeEvidenceCommand(invocation evidenceCommand) error {
	cmd := exec.Command(invocation.name, invocation.args...)
	cmd.Dir = invocation.dir
	cmd.Stdout = invocation.stdout
	cmd.Stderr = invocation.stderr
	return cmd.Run()
}

func (runner evidenceRunner) run(args []string) error {
	if reflect.DeepEqual(args, []string{"list", "-m"}) ||
		reflect.DeepEqual(args, []string{"list", "./..."}) {
		return runner.runForwarded("delegate catalog Go inventory", evidenceCommand{
			name: "go",
			args: args,
			dir:  runner.root,
		})
	}
	mode, err := parseEvidenceMode(args)
	if err != nil {
		return err
	}
	module, packages, err := runner.catalogPackages()
	if err != nil {
		return err
	}

	var failures []error
	if len(packages) > 0 {
		goArgs := []string{"test", "-json"}
		if mode == inventoryMode {
			goArgs = append(goArgs, "-list", "^Test")
		} else {
			goArgs = append(goArgs, "-count=1")
		}
		goArgs = append(goArgs, packages...)
		if err := runner.runForwarded("non-conformance Go evidence", evidenceCommand{
			name: "go",
			args: goArgs,
			dir:  runner.root,
		}); err != nil {
			failures = append(failures, err)
		}
	}
	if err := runner.runStableConformance(module+"/conformance", mode); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func parseEvidenceMode(args []string) (evidenceMode, error) {
	switch {
	case reflect.DeepEqual(args, []string{"test", "-json", "-list", "^Test", "./..."}):
		return inventoryMode, nil
	case reflect.DeepEqual(args, []string{"test", "-json", "-count=1", "./..."}):
		return runMode, nil
	default:
		return 0, fmt.Errorf("unsupported catalog evidence command: go %s", strings.Join(args, " "))
	}
}

func (runner evidenceRunner) catalogPackages() (string, []string, error) {
	module, err := runner.captureGo("list", "-m")
	if err != nil {
		return "", nil, fmt.Errorf("resolve catalog module: %w", err)
	}
	rawPackages, err := runner.captureGo("list", "./...")
	if err != nil {
		return "", nil, fmt.Errorf("inventory catalog packages: %w", err)
	}
	module = strings.TrimSpace(module)
	if module == "" {
		return "", nil, errors.New("resolve catalog module: go list -m returned no module")
	}

	var packages []string
	for _, packagePath := range strings.Fields(rawPackages) {
		if packagePath == module+"/conformance" {
			continue
		}
		packages = append(packages, packagePath)
	}
	return module, packages, nil
}

func (runner evidenceRunner) captureGo(args ...string) (string, error) {
	var stdout, stderr bytes.Buffer
	err := runner.runCommand(evidenceCommand{
		name:   "go",
		args:   args,
		dir:    runner.root,
		stdout: &stdout,
		stderr: io.MultiWriter(runner.stderr, &stderr),
	})
	if err != nil {
		return "", fmt.Errorf("go %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (runner evidenceRunner) runStableConformance(packagePath string, mode evidenceMode) (returnErr error) {
	tempDir, err := runner.mkdirTemp("", stableTestPattern)
	if err != nil {
		return fmt.Errorf("create stable conformance directory: %w", err)
	}
	defer func() {
		if err := runner.removeAll(tempDir); err != nil {
			returnErr = errors.Join(returnErr,
				fmt.Errorf("clean stable conformance directory %s: %w", tempDir, err))
		}
	}()

	binary := filepath.Join(tempDir, "conformance.test")
	if err := runner.runForwarded("compile conformance evidence binary", evidenceCommand{
		name: "go",
		args: []string{"test", "-c", "-o", binary, "./conformance"},
		dir:  runner.root,
	}); err != nil {
		return err
	}

	testArgs := []string{"-test.v=true", "-test.timeout=" + testTimeout}
	if mode == inventoryMode {
		testArgs = append(testArgs, "-test.list=^Test")
	} else {
		testArgs = append(testArgs, "-test.count=1", "-live=false")
	}
	return runner.runForwarded("execute conformance evidence binary", evidenceCommand{
		name: "go",
		args: append([]string{
			"tool", "test2json", "-t", "-p", packagePath, binary,
		}, testArgs...),
		dir: filepath.Join(runner.root, "conformance"),
	})
}

func (runner evidenceRunner) runForwarded(label string, invocation evidenceCommand) error {
	invocation.stdout = runner.stdout
	invocation.stderr = runner.stderr
	if err := runner.runCommand(invocation); err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	return nil
}
