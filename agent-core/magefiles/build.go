// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/magefile/mage/sh"
)

const binDir = "bin"

// Build compiles all cmd/ binaries into bin/. Application UI assets are owned
// and built by their external profiles, not embedded into the core runtime.
func Build() error {
	pkgs, err := discoverCmdPackages()
	if err != nil {
		return err
	}
	if len(pkgs) == 0 {
		fmt.Println("no cmd/ packages found, skipping build")
		return nil
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", binDir, err)
	}

	for _, pkg := range pkgs {
		name := filepath.Base(pkg)
		out := filepath.Join(binDir, name)
		args := []string{"build", "-o", out}
		args = append(args, pkg)
		fmt.Printf("building %s → %s\n", pkg, out)
		if err := sh.Run("go", args...); err != nil {
			return fmt.Errorf("build %s: %w", pkg, err)
		}
	}
	return nil
}

// Audit runs the jurist agent against the project documentation.
func Audit() error {
	binary, err := filepath.Abs(filepath.Join(binDir, "agent"))
	if err != nil {
		return err
	}
	fmt.Println("building agent binary...")
	if err := Build(); err != nil {
		return fmt.Errorf("build agent: %w", err)
	}

	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}
	profileRoot, err := filepath.Abs(filepath.Join("testdata", "integration", "profiles", "audit"))
	if err != nil {
		return err
	}

	for _, profileName := range []string{"profile.yaml", "audit-profile.yaml"} {
		profilePath := filepath.Join(profileRoot, profileName)
		cmd := exec.Command(binary,
			"--profile", profilePath,
			"--directory", rootDir,
			"--core-root", rootDir,
		)
		var output bytes.Buffer
		cmd.Stdout = io.MultiWriter(os.Stdout, &output)
		cmd.Stderr = io.MultiWriter(os.Stderr, &output)
		if err := cmd.Run(); !agentRunCompleted(err) {
			return err
		}
		if auditRunFailed(output.String()) {
			return fmt.Errorf("audit failed: specification-critic profile %s reported failed terminal status", profileName)
		}
	}
	return nil
}

// requiredGolangciLintMajor is the golangci-lint major version the repository's
// .golangci.yml schema (version: "2") requires. golangci-lint v1 cannot read a
// v2 config and aborts mid-run, so Lint preflights the binary and fails with
// installation guidance rather than a schema error deep in the tool output.
const requiredGolangciLintMajor = 2

const golangciLintInstallURL = "https://golangci-lint.run/welcome/install/"

// Lint runs golangci-lint on the runtime and standalone Mage modules after
// verifying a compatible binary.
func Lint() error {
	if err := checkGolangciLintVersion(requiredGolangciLintMajor); err != nil {
		return err
	}
	for _, module := range []string{".", "magefiles"} {
		cmd := exec.Command("golangci-lint", "run", "./...")
		cmd.Dir = module
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("lint %s: %w", module, err)
		}
	}
	return nil
}

// checkGolangciLintVersion verifies a golangci-lint binary whose major version
// matches the .golangci.yml schema is on PATH.
func checkGolangciLintVersion(wantMajor int) error {
	path, err := exec.LookPath("golangci-lint")
	if err != nil {
		return fmt.Errorf("golangci-lint not found on PATH: the .golangci.yml gate needs golangci-lint v%d; install it from %s", wantMajor, golangciLintInstallURL)
	}
	out, err := exec.Command(path, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("golangci-lint version: %w", err)
	}
	major, err := parseGolangciLintMajor(string(out))
	if err != nil {
		return fmt.Errorf("parse golangci-lint version from %q: %w", strings.TrimSpace(string(out)), err)
	}
	if major != wantMajor {
		return fmt.Errorf("golangci-lint v%d is required by .golangci.yml (version: %q) but v%d is installed; install golangci-lint v%d from %s", wantMajor, strconv.Itoa(wantMajor), major, wantMajor, golangciLintInstallURL)
	}
	return nil
}

var golangciLintVersionRE = regexp.MustCompile(`(\d+)\.\d+\.\d+`)

// parseGolangciLintMajor extracts the major version from `golangci-lint version`
// output, whose stable substring is "has version X.Y.Z" across v1 and v2.
func parseGolangciLintMajor(output string) (int, error) {
	m := golangciLintVersionRE.FindStringSubmatch(output)
	if m == nil {
		return 0, fmt.Errorf("no semantic version found")
	}
	return strconv.Atoi(m[1])
}

// Install runs go install for all cmd/ packages.
func Install() error {
	pkgs, err := discoverCmdPackages()
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		fmt.Printf("installing %s\n", pkg)
		if err := sh.Run("go", "install", pkg); err != nil {
			return fmt.Errorf("install %s: %w", pkg, err)
		}
	}
	return nil
}

// Clean removes the bin/ directory.
func Clean() error {
	fmt.Printf("removing %s/\n", binDir)
	return os.RemoveAll(binDir)
}

// discoverCmdPackages finds all cmd/*/main.go packages.
func discoverCmdPackages() ([]string, error) {
	entries, err := os.ReadDir("cmd")
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read cmd/: %w", err)
	}
	var pkgs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		main := filepath.Join("cmd", e.Name(), "main.go")
		if _, err := os.Stat(main); err == nil {
			pkgs = append(pkgs, "./cmd/"+e.Name())
		}
	}
	return pkgs, nil
}
