// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/magefile/mage/sh"
)

const binDir = "bin"

// Build compiles all cmd/ binaries into bin/.
// If any embedded UI directories are found (internal/evaluation/bench/ui/, etc.),
// their frontends are built first and Go is compiled with -tags
// production to embed the assets.
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

	needsProduction := false
	for _, uiDir := range embeddedUIDirs {
		if hasUI(uiDir) {
			fmt.Printf("installing frontend deps for %s\n", uiDir)
			if err := runInDir(uiDir, "npm", "install"); err != nil {
				return fmt.Errorf("%s npm install: %w", uiDir, err)
			}
			fmt.Printf("building frontend for %s\n", uiDir)
			if err := runInDir(uiDir, "npm", "run", "build"); err != nil {
				return fmt.Errorf("%s frontend build: %w", uiDir, err)
			}
			needsProduction = true
		}
	}

	for _, pkg := range pkgs {
		name := filepath.Base(pkg)
		out := filepath.Join(binDir, name)
		args := []string{"build", "-o", out}
		if needsProduction {
			args = append(args, "-tags", "production")
		}
		args = append(args, pkg)
		fmt.Printf("building %s → %s\n", pkg, out)
		if err := sh.Run("go", args...); err != nil {
			return fmt.Errorf("build %s: %w", pkg, err)
		}
	}
	return nil
}

var embeddedUIDirs = []string{
	"internal/evaluation/bench/ui",
	"internal/knowledge/documentation/ui",
}

func hasUI(uiDir string) bool {
	_, err := os.Stat(filepath.Join(uiDir, "package.json"))
	return err == nil
}

func runInDir(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
	profileRoot, err := resolveAgentProfilesRoot(rootDir)
	if err != nil {
		return err
	}

	cmd := exec.Command(binary,
		"--profile", agentProfilePath(profileRoot, "jurist"),
		"--directory", rootDir,
		"--core-root", rootDir,
	)
	var output bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)
	if err := cmd.Run(); err != nil {
		return err
	}
	if auditRunFailed(output.String()) {
		return fmt.Errorf("audit failed: jurist reported failed terminal status")
	}
	return nil
}

// JuristCharterSmoke runs the profile-owned demo charter against its fixture.
func JuristCharterSmoke() error {
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
	profilesRepoRoot, err := resolveAgentProfilesRepoRoot(rootDir)
	if err != nil {
		return err
	}
	profilePath, cleanup, err := writeJuristCharterSmokeProfile(rootDir, profilesRepoRoot)
	if err != nil {
		return err
	}
	defer cleanup()

	fixtureDir := filepath.Join(profilesRepoRoot, "testdata", "integration", "jurist-charter-demo")
	cmd := exec.Command(binary,
		"--profile", profilePath,
		"--directory", fixtureDir,
		"--core-root", rootDir,
	)
	var output bytes.Buffer
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run jurist charter smoke: %w", err)
	}
	if err := assertJuristCharterSmoke(output.String()); err != nil {
		return err
	}
	return nil
}

func writeJuristCharterSmokeProfile(coreRoot, profilesRepoRoot string) (string, func(), error) {
	tmpDir, err := os.MkdirTemp("", "agent-core-jurist-charter-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	profileRoot := filepath.Join(profilesRepoRoot, "agents")
	toolDeclPath := filepath.Join(tmpDir, "load-corpus-demo.yaml")
	profilePath := filepath.Join(tmpDir, "profile.yaml")
	profile := fmt.Sprintf(`name: jurist-charter-smoke
machine: %q
tools:
  - %q
tool_config_dirs:
  - %q
tool_declarations:
  - %q
`, agentProfileAsset(profileRoot, "jurist/machine.yaml"),
		agentProfileAsset(profileRoot, "jurist/tools.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "spec-validation"),
		toolDeclPath)
	if err := os.WriteFile(profilePath, []byte(profile), 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write jurist charter smoke profile: %w", err)
	}
	toolDecl := fmt.Sprintf(`includes:
  - %q
tools:
  - name: load_corpus
    type: builtin
    init: load_corpus
    visibility: internal
    config:
      suite_paths:
        - %q
    emits:
      - ToolDone
      - CommandError
`, filepath.Join(coreRoot, "tools", "builtin", "load-corpus.yaml"),
		agentProfileAsset(profileRoot, "jurist/suites/demo-charter.yaml"))
	if err := os.WriteFile(toolDeclPath, []byte(toolDecl), 0o644); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("write jurist charter smoke declaration: %w", err)
	}
	return profilePath, cleanup, nil
}

func assertJuristCharterSmoke(output string) error {
	required := []string{
		"jurist-demo-charter/no-internal-vocabulary (grep_check)",
		"jurist-demo-charter/citations-resolve (ref_check)",
		"jurist-demo-charter/artifacts-exist (consistency_check)",
		"terminal state: failed",
	}
	for _, want := range required {
		if !bytes.Contains([]byte(output), []byte(want)) {
			return fmt.Errorf("jurist charter smoke output missing %q", want)
		}
	}
	return nil
}

// Lint runs golangci-lint on the project.
func Lint() error {
	return sh.Run("golangci-lint", "run", "./...")
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
