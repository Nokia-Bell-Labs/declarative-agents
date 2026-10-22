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
// The schema major version is read from the module configs, because the
// schema version in those files *is* that requirement. The exact release is a
// pin: a minor release adds findings to unchanged code (errcheck in 2.15.0
// stopped a release, GH-2707), so the gate runs only the pinned release and
// `mage bump` reports when a newer one exists (GH-2710).

// golangciLintVersion is the one golangci-lint release the lint gate runs.
// magefiles/pinsurvey/pins.yaml names it and agent-core/magefiles/build.go
// repeats it for agent-core's standalone module; a test keeps them equal.
const golangciLintVersion = "v2.15.0"

const golangciLintInstallURL = "https://golangci-lint.run/welcome/install/"

// checkGolangciLint verifies the pinned golangci-lint release is on PATH, and
// returns installation guidance when it is not.
func checkGolangciLint() error {
	if _, err := requiredGolangciLintMajor(); err != nil {
		return err
	}
	installed, err := installedGolangciLintVersion()
	if err != nil {
		return err
	}
	return checkPinnedGolangciLint(installed, golangciLintVersion)
}

// checkPinnedGolangciLint compares an installed release with the pin.
func checkPinnedGolangciLint(installed, pinned string) error {
	if strings.TrimPrefix(installed, "v") == strings.TrimPrefix(pinned, "v") {
		return nil
	}
	return fmt.Errorf(
		"golangci-lint %s is pinned (magefiles/lint_preflight.go golangciLintVersion) but v%s is installed; install golangci-lint %s from %s",
		pinned, strings.TrimPrefix(installed, "v"), pinned, golangciLintInstallURL)
}

// reportGolangciLint prints the lint toolchain line for Doctor, reporting the
// mismatch as an error rather than a warning: a mismatched binary cannot run the
// gate at all.
func reportGolangciLint() error {
	if err := checkGolangciLint(); err != nil {
		return err
	}
	fmt.Printf("doctor: golangci-lint %s (pinned) installed OK\n", golangciLintVersion)
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

func installedGolangciLintVersion() (string, error) {
	path, err := exec.LookPath("golangci-lint")
	if err != nil {
		return "", fmt.Errorf(
			"golangci-lint not found on PATH: the lint gate needs golangci-lint %s; install it from %s",
			golangciLintVersion, golangciLintInstallURL)
	}
	out, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("golangci-lint version: %w", err)
	}
	return parseGolangciLintVersion(string(out))
}

var golangciLintVersionPattern = regexp.MustCompile(`v?(\d+\.\d+\.\d+)`)

// parseGolangciLintVersion extracts X.Y.Z from `golangci-lint version` output,
// whose stable substring is "has version X.Y.Z" across v1 and v2.
func parseGolangciLintVersion(output string) (string, error) {
	match := golangciLintVersionPattern.FindStringSubmatch(output)
	if match == nil {
		return "", fmt.Errorf("no semantic version found in %q", strings.TrimSpace(output))
	}
	return match[1], nil
}
