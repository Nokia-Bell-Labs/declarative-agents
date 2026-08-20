// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type goPackageImports struct {
	ImportPath string
	Imports    []string
}

type inferenceBoundaryPolicy struct {
	module          string
	adapterPackages []string
	providerImports []string
}

func TestPatternInvariantRepositoryConformance(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	var ran int
	err := checkPatternInvariants(root, func(_, _, _ string) error {
		ran++
		return nil
	}, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if ran == 0 {
		t.Fatal("repository pattern language registered no executable checks")
	}
}

func TestEveryRegisteredPatternInvariantCheckGatesItsViolation(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	language, err := loadPatternInvariants(filepath.Join(root, patternLanguagePath))
	if err != nil {
		t.Fatal(err)
	}
	checks, _, err := validatePatternInvariants(language)
	if err != nil {
		t.Fatal(err)
	}
	for _, planted := range checks {
		t.Run(planted.invariantID, func(t *testing.T) {
			want := errors.New("planted invariant violation")
			run := func(_, command, _ string) error {
				if command == planted.command {
					return want
				}
				return nil
			}
			err := checkPatternInvariants(root, run, &bytes.Buffer{})
			if err == nil || !strings.Contains(err.Error(), planted.invariantID) ||
				!errors.Is(err, want) {
				t.Fatalf("error = %v, want %s wrapping planted violation", err, planted.invariantID)
			}
		})
	}
}

func TestPatternInvariantRejectsMissingCheckBlock(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, `
patterns:
  - id: machine-interpreter
    invariants:
      - id: P1.1
        statement: The machine is valid.
`)
	err := checkPatternInvariants(root, passingPatternCheck, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "pattern invariant P1.1 has no check block") {
		t.Fatalf("error = %v, want invariant id and missing check block", err)
	}
}

func TestPatternInvariantRejectsExecutableWithoutNegativeTest(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, `
patterns:
  - id: machine-interpreter
    invariants:
      - id: P1.1
        statement: The machine is valid.
        check:
          kind: executable
          command: go test ./machine -run TestMachineInvalid
`)
	err := checkPatternInvariants(root, passingPatternCheck, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "pattern invariant P1.1 has no negative test") {
		t.Fatalf("error = %v, want invariant id and missing negative test", err)
	}
}

func TestPatternInvariantRejectsUnselectedNegativeTest(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, `
patterns:
  - id: machine-interpreter
    invariants:
      - id: P1.1
        statement: The machine is valid.
        check:
          kind: executable
          command: go test ./machine -run TestOther
          negative_test: TestMachineInvalid
`)
	err := checkPatternInvariants(root, passingPatternCheck, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "command does not select negative test TestMachineInvalid") {
		t.Fatalf("error = %v, want unselected negative test", err)
	}
}

func TestPatternInvariantReportsManualCount(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, `
patterns:
  - id: tool-contract
    invariants:
      - id: P3.1
        statement: Tool contracts are complete.
        check:
          kind: executable
          issue: GH-1786
          negative_test: TestFutureCheck
      - id: P3.2
        statement: Tools do not hide workflow.
        check:
          kind: manual
          reason: Atomicity requires semantic judgment.
`)
	var output bytes.Buffer
	if err := checkPatternInvariants(root, passingPatternCheck, &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "manual-invariant count: 1") {
		t.Fatalf("output = %q, want manual count", output.String())
	}
	if !strings.Contains(output.String(), "2 total, 1 executable, 1 pending") {
		t.Fatalf("output = %q, want executable and pending counts", output.String())
	}
}

func TestPatternInvariantLoadsAddedCheckWithoutCodeChange(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, executableInvariantYAML("P1.1", "TestFirst"))
	var commands []string
	run := func(_, command, _ string) error {
		commands = append(commands, command)
		return nil
	}
	if err := checkPatternInvariants(root, run, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	writePatternInvariantFixture(t, root, executableInvariantYAML("P1.1", "TestFirst")+`
  - id: agent-as-data
    invariants:
      - id: P2.1
        statement: The profile closure resolves.
        check:
          kind: executable
          command: go test ./profile -run TestSecond
          negative_test: TestSecond
`)
	commands = nil
	if err := checkPatternInvariants(root, run, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(commands) != 2 {
		t.Fatalf("ran %d commands, want 2 after YAML-only addition", len(commands))
	}
}

func TestPatternInvariantFailsRegisteredCheck(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, executableInvariantYAML("P1.1", "TestMachineInvalid"))
	want := errors.New("planted violation detected")
	err := checkPatternInvariants(root, func(_, _, _ string) error {
		return want
	}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "pattern invariant P1.1") ||
		!errors.Is(err, want) {
		t.Fatalf("error = %v, want P1.1 wrapping planted violation", err)
	}
}

func TestRunPatternInvariantCommandRequiresNegativeTestEvidence(t *testing.T) {
	t.Parallel()

	const testName = "TestPlantedViolation"
	pass := "printf '=== RUN   TestPlantedViolation\\n--- PASS: TestPlantedViolation (0.00s)\\n'"
	if err := runPatternInvariantCommand(t.TempDir(), pass, testName); err != nil {
		t.Fatalf("passing negative test evidence: %v", err)
	}
	missing := "printf 'ok\\n'"
	if err := runPatternInvariantCommand(t.TempDir(), missing, testName); err == nil ||
		!strings.Contains(err.Error(), "did not run and pass") {
		t.Fatalf("missing evidence error = %v", err)
	}
	if err := runPatternInvariantCommand(t.TempDir(), "exit 1", testName); err == nil ||
		!strings.Contains(err.Error(), "check command failed") {
		t.Fatalf("failing command error = %v", err)
	}
}

func TestPatternInvariantInferenceBoundary(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	policy, err := loadInferenceBoundaryPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := listGoPackageImports(root, policy.module)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkInferenceBoundary(packages, policy); err != nil {
		t.Fatal(err)
	}
}

func TestPatternInvariantInferenceBoundaryRejectsImportOutsideAdapter(t *testing.T) {
	t.Parallel()

	const provider = "example.com/provider/sdk"
	policy := inferenceBoundaryPolicy{
		adapterPackages: []string{"example.com/project/internal/model"},
		providerImports: []string{provider},
	}
	packages := []goPackageImports{
		{ImportPath: "example.com/project/internal/model/openai", Imports: []string{provider}},
		{ImportPath: "example.com/project/cmd/agent", Imports: []string{provider}},
		{ImportPath: "example.com/project/internal/tools/search", Imports: []string{provider + "/chat"}},
	}
	err := checkInferenceBoundary(packages, policy)
	if err == nil {
		t.Fatal("inference boundary accepted planted provider imports")
	}
	for _, violatingPackage := range []string{
		"example.com/project/cmd/agent",
		"example.com/project/internal/tools/search",
	} {
		if !strings.Contains(err.Error(), violatingPackage) {
			t.Errorf("error missing violating package %s: %v", violatingPackage, err)
		}
	}
	if strings.Contains(err.Error(), "internal/model/openai") {
		t.Fatalf("error reported package inside adapter boundary: %v", err)
	}
}

func TestPatternInvariantInferenceBoundaryProviderListIsDeclarative(t *testing.T) {
	t.Parallel()

	root := patternInvariantFixture(t, inferenceBoundaryPolicyYAML(
		"example.com/provider/existing-sdk"))
	packages := []goPackageImports{{
		ImportPath: "example.com/project/cmd/agent",
		Imports:    []string{"example.com/provider/new-sdk"},
	}}
	policy, err := loadInferenceBoundaryPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkInferenceBoundary(packages, policy); err != nil {
		t.Fatalf("undeclared provider changed verdict: %v", err)
	}
	writePatternInvariantFixture(t, root, inferenceBoundaryPolicyYAML(
		"example.com/provider/existing-sdk",
		"example.com/provider/new-sdk"))
	policy, err = loadInferenceBoundaryPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkInferenceBoundary(packages, policy); err == nil {
		t.Fatal("adding provider to pattern language did not change verdict")
	}
}

func loadInferenceBoundaryPolicy(root string) (inferenceBoundaryPolicy, error) {
	language, err := loadPatternInvariants(filepath.Join(root, patternLanguagePath))
	if err != nil {
		return inferenceBoundaryPolicy{}, err
	}
	for _, pattern := range language.Patterns {
		for _, invariant := range pattern.Invariants {
			if invariant.ID != "P5.1" || invariant.Check == nil {
				continue
			}
			check := invariant.Check
			if check.Module == "" || len(check.AdapterPackages) == 0 ||
				len(check.ProviderImports) == 0 {
				return inferenceBoundaryPolicy{}, errors.New(
					"P5.1 requires module, adapter_packages, and provider_imports")
			}
			return inferenceBoundaryPolicy{
				module:          check.Module,
				adapterPackages: append([]string(nil), check.AdapterPackages...),
				providerImports: append([]string(nil), check.ProviderImports...),
			}, nil
		}
	}
	return inferenceBoundaryPolicy{}, errors.New("pattern language has no P5.1 invariant")
}

func listGoPackageImports(root, module string) ([]goPackageImports, error) {
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = filepath.Join(root, filepath.FromSlash(module))
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list Go packages in %s: %w", module, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(output))
	var packages []goPackageImports
	for {
		var pkg goPackageImports
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		packages = append(packages, pkg)
	}
	return packages, nil
}

func checkInferenceBoundary(packages []goPackageImports, policy inferenceBoundaryPolicy) error {
	violations := make(map[string]bool)
	for _, pkg := range packages {
		if packageMatchesAnyPrefix(pkg.ImportPath, policy.adapterPackages) {
			continue
		}
		for _, imported := range pkg.Imports {
			if packageMatchesAnyPrefix(imported, policy.providerImports) {
				violations[pkg.ImportPath] = true
			}
		}
	}
	names := make([]string, 0, len(violations))
	for name := range violations {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 0 {
		return fmt.Errorf("packages outside inference adapter import provider code: %s",
			strings.Join(names, ", "))
	}
	return nil
}

func packageMatchesAnyPrefix(importPath string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
}

func inferenceBoundaryPolicyYAML(providerImports ...string) string {
	var yaml strings.Builder
	yaml.WriteString(`
patterns:
  - id: inference-boundary
    invariants:
      - id: P5.1
        statement: Provider imports stay inside the adapter.
        check:
          kind: executable
          module: agent-core
          adapter_packages:
            - example.com/project/internal/model
          provider_imports:
`)
	for _, providerImport := range providerImports {
		fmt.Fprintf(&yaml, "            - %s\n", providerImport)
	}
	return yaml.String()
}

func executableInvariantYAML(id, negativeTest string) string {
	return `
patterns:
  - id: machine-interpreter
    invariants:
      - id: ` + id + `
        statement: The machine is valid.
        check:
          kind: executable
          command: go test ./machine -run ` + negativeTest + `
          negative_test: ` + negativeTest + `
`
}

func patternInvariantFixture(t *testing.T, content string) string {
	t.Helper()

	root := t.TempDir()
	writePatternInvariantFixture(t, root, content)
	return root
}

func writePatternInvariantFixture(t *testing.T, root, content string) {
	t.Helper()

	path := filepath.Join(root, patternLanguagePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func passingPatternCheck(_, _, _ string) error {
	return nil
}
