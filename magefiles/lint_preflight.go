// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// A golangci-lint binary whose major version does not match the config schema
// aborts mid-run with a schema error, naming neither the fix nor the required
// version. agent-core's module-local Lint has preflighted this for a while, but
// the root target -- the one that covers all nine modules -- did not, so the
// repository-wide `mage lint` was the one place that surfaced the raw tool error
// (GH-1479).
//
// The required version is read from the module configs rather than held as a
// constant here, because the schema version in those files *is* the requirement.
// A constant would be a second place to update and a second place to drift.

const golangciLintInstallURL = "https://golangci-lint.run/welcome/install/"

// checkGolangciLint verifies a golangci-lint binary matching the config schema is
// on PATH, and returns installation guidance when it is not.
func checkGolangciLint() error {
	wantMajor, err := requiredGolangciLintMajor()
	if err != nil {
		return err
	}
	major, err := installedGolangciLintMajor()
	if err != nil {
		return err
	}
	if major != wantMajor {
		return fmt.Errorf(
			"golangci-lint v%d is required by .golangci.yml (version: %q) but v%d is installed; install golangci-lint v%d from %s",
			wantMajor, strconv.Itoa(wantMajor), major, wantMajor, golangciLintInstallURL)
	}
	return nil
}

// reportGolangciLint prints the lint toolchain line for Doctor, reporting the
// mismatch as an error rather than a warning: a mismatched binary cannot run the
// gate at all.
func reportGolangciLint() error {
	wantMajor, err := requiredGolangciLintMajor()
	if err != nil {
		return err
	}
	if err := checkGolangciLint(); err != nil {
		return err
	}
	fmt.Printf("doctor: golangci-lint v%d config schema satisfied by the installed binary OK\n", wantMajor)
	return nil
}

// requiredGolangciLintMajor returns the schema version every module config
// declares, and reports configs that disagree instead of trusting the first.
func requiredGolangciLintMajor() (int, error) {
	required := 0
	source := ""
	for _, module := range lintModuleDirs {
		version, err := golangciConfigSchema(module)
		if err != nil {
			return 0, err
		}
		if required == 0 {
			required, source = version, module
			continue
		}
		if version != required {
			return 0, fmt.Errorf(
				"module lint configs disagree on the schema version: %s declares v%d, %s declares v%d",
				module, version, source, required)
		}
	}
	if required == 0 {
		return 0, fmt.Errorf("no module declares a .golangci.yml schema version")
	}
	return required, nil
}

// findRepositoryRoot locates the root by walking up from the working directory, so
// the same helpers work under mage, which runs at the root, and under go test,
// which runs in the package directory.
func findRepositoryRoot() (string, error) {
	dir := "."
	for range 6 {
		if _, err := os.Stat(filepath.Join(dir, "magefiles", ".golangci.yml")); err == nil {
			return dir, nil
		}
		dir = filepath.Join(dir, "..")
	}
	return "", fmt.Errorf("repository root not found above the working directory")
}

func golangciConfigSchema(module string) (int, error) {
	root, err := findRepositoryRoot()
	if err != nil {
		return 0, err
	}
	path := filepath.Join(root, filepath.FromSlash(module), ".golangci.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read lint config: %w", err)
	}
	var config struct {
		Version string `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return 0, fmt.Errorf("parse %s: %w", path, err)
	}
	major, err := strconv.Atoi(strings.TrimSpace(config.Version))
	if err != nil {
		return 0, fmt.Errorf("%s declares schema version %q, want an integer", path, config.Version)
	}
	return major, nil
}

func installedGolangciLintMajor() (int, error) {
	path, err := exec.LookPath("golangci-lint")
	if err != nil {
		wantMajor, versionErr := requiredGolangciLintMajor()
		if versionErr != nil {
			return 0, versionErr
		}
		return 0, fmt.Errorf(
			"golangci-lint not found on PATH: the .golangci.yml gate needs golangci-lint v%d; install it from %s",
			wantMajor, golangciLintInstallURL)
	}
	out, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("golangci-lint version: %w", err)
	}
	return parseGolangciLintMajor(string(out))
}

var golangciLintVersionPattern = regexp.MustCompile(`(\d+)\.\d+\.\d+`)

// parseGolangciLintMajor extracts the major version from `golangci-lint version`
// output, whose stable substring is "has version X.Y.Z" across v1 and v2.
func parseGolangciLintMajor(output string) (int, error) {
	match := golangciLintVersionPattern.FindStringSubmatch(output)
	if match == nil {
		return 0, fmt.Errorf("no semantic version found in %q", strings.TrimSpace(output))
	}
	return strconv.Atoi(match[1])
}
