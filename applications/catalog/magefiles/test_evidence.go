// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const catalogEvidenceHelperPackage = "./cmd/catalog-test-evidence"

type evidenceProfileStager func(string) (string, func() error, error)

// validateTestEvidence runs the declarative specification-critic audit profile, which owns
// inventory, claim resolution, test execution, reduction, and reporting.
func validateTestEvidence(
	run profileSmokeRunner,
	stage evidenceProfileStager,
	binary, root, coreRoot string,
) (returnErr error) {
	profile, cleanup, err := stage(root)
	if err != nil {
		return fmt.Errorf("stage catalog test evidence profile: %w", err)
	}
	defer func() {
		if err := cleanup(); err != nil {
			returnErr = errors.Join(returnErr, fmt.Errorf("clean catalog test evidence profile: %w", err))
		}
	}()

	out, err := run(binary,
		"--profile", profile,
		"--directory", root,
		"--core-root", coreRoot,
	)
	if err == nil {
		fmt.Printf("validated formal go_test evidence under %s through specification-critic audit profile\n", root)
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("formal go_test evidence audit failed:\n%s", detail)
}

func stageCatalogTestEvidenceProfile(root string) (string, func() error, error) {
	runner := catalogTestRunner{
		catalogRoot: root,
		stdout:      os.Stdout,
		stderr:      os.Stderr,
		runCommand:  executeCatalogCommand,
		mkdirTemp:   os.MkdirTemp,
		removeAll:   os.RemoveAll,
	}
	return stageCatalogTestEvidenceProfileWith(root, runner)
}

func stageCatalogTestEvidenceProfileWith(
	root string,
	runner catalogTestRunner,
) (_ string, cleanup func() error, returnErr error) {
	tempDir, err := runner.mkdirTemp("", "catalog-test-evidence-profile-*")
	if err != nil {
		return "", nil, err
	}
	cleanupPath := func() error { return runner.removeAll(tempDir) }
	cleanup = cleanupPath
	defer func() {
		if returnErr != nil {
			returnErr = errors.Join(returnErr, cleanupPath())
		}
	}()

	helper := filepath.Join(tempDir, "catalog-test-evidence")
	fmt.Fprintln(runner.stdout, "=== catalog formal evidence helper compile ===")
	if err := runner.runPhase("compile catalog formal evidence helper", catalogCommand{
		name: "go",
		args: []string{"build", "-o", helper, catalogEvidenceHelperPackage},
		dir:  root,
	}); err != nil {
		return "", nil, err
	}

	sourceDir := filepath.Join(root, "agents", "specification-critic")
	for _, name := range []string{
		"audit-profile.yaml",
		"audit-machine.yaml",
		"audit-tools.yaml",
		"go-test.yaml",
	} {
		data, err := os.ReadFile(filepath.Join(sourceDir, name))
		if err != nil {
			return "", nil, fmt.Errorf("read %s: %w", name, err)
		}
		if name == "go-test.yaml" {
			data = []byte(strings.ReplaceAll(
				string(data),
				"binary: go",
				"binary: "+strconv.Quote(helper),
			))
		}
		if err := os.WriteFile(filepath.Join(tempDir, name), data, 0o644); err != nil {
			return "", nil, fmt.Errorf("write staged %s: %w", name, err)
		}
	}
	return filepath.Join(tempDir, "audit-profile.yaml"), cleanup, nil
}
