// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// runAcceptanceEvidence executes every acceptance entry of every statement and
// reports the first failures joined. moduleRoot is the application-dsl-spec
// directory; all evidence paths resolve relative to it and may not escape it.
func runAcceptanceEvidence(moduleRoot string, language languageFile) error {
	var findings []error
	entries := 0
	for _, statement := range language.Statements {
		for _, entry := range statement.Acceptance {
			entries++
			if err := runAcceptanceEntry(moduleRoot, entry); err != nil {
				findings = append(findings, fmt.Errorf("%s: %w", statement.ID, err))
			}
		}
	}
	if len(findings) > 0 {
		return fmt.Errorf("acceptance evidence failed: %w", errors.Join(findings...))
	}
	fmt.Printf("validated %d acceptance entries across %d statements\n",
		entries, len(language.Statements))
	return nil
}

func runAcceptanceEntry(moduleRoot string, entry acceptanceEntry) error {
	// A rig entry references the wider repository, so it resolves against the
	// module's parent rather than the module.
	if entry.Assertion == "rig" {
		return runRigEvidence(filepath.Join(moduleRoot, ".."), entry)
	}
	path, err := resolveModulePath(moduleRoot, entry.Path)
	if err != nil {
		return err
	}
	switch entry.Assertion {
	case "fixture":
		return runFixtureCheck(path, entry)
	case "go_test":
		return runGoTestEvidence(path, entry.Test)
	default:
		return fmt.Errorf("unknown acceptance assertion %q", entry.Assertion)
	}
}

// runRigEvidence verifies a rig reference: the repository test file exists and
// declares the named test. Rig entries anchor runtime- and population-target
// statements to integration evidence that the repository's test and
// integration gates execute; the audit proves the reference cannot rot, not
// the run, because those suites need clusters and rigs the audit does not own.
func runRigEvidence(repositoryRoot string, entry acceptanceEntry) error {
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return fmt.Errorf("resolve repository root: %w", err)
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.Clean(filepath.FromSlash(entry.Path))))
	if err != nil {
		return fmt.Errorf("resolve rig path %q: %w", entry.Path, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("rig path %q escapes the repository", entry.Path)
	}
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("rig path %q: %w", entry.Path, err)
	}
	return checkGoTestDeclared(path, entry.Test)
}

// resolveModulePath resolves relative against moduleRoot and rejects paths
// that escape it, so evidence cannot silently point outside the module.
func resolveModulePath(moduleRoot, relative string) (string, error) {
	root, err := filepath.Abs(moduleRoot)
	if err != nil {
		return "", fmt.Errorf("resolve module root: %w", err)
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.Clean(filepath.FromSlash(relative))))
	if err != nil {
		return "", fmt.Errorf("resolve evidence path %q: %w", relative, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("evidence path %q escapes the module", relative)
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("evidence path %q: %w", relative, err)
	}
	return path, nil
}

// runFixtureCheck applies the entry's check to the fixture and compares the
// outcome with the declared verdict. A valid fixture must pass the check and
// an invalid fixture must fail it; either mismatch is failing evidence. The
// check kind resolves before the verdict comparison, so an unknown kind can
// never read as an invalid fixture correctly failing.
func runFixtureCheck(path string, entry acceptanceEntry) error {
	check, ok := fixtureChecks[entry.Check]
	if !ok {
		return fmt.Errorf("unknown fixture check %q", entry.Check)
	}
	checkErr := check(path)
	switch entry.Verdict {
	case "valid":
		if checkErr != nil {
			return fmt.Errorf("fixture %s declared valid but failed check %s: %w",
				entry.Path, entry.Check, checkErr)
		}
	case "invalid":
		if checkErr == nil {
			return fmt.Errorf("fixture %s declared invalid but passed check %s",
				entry.Path, entry.Check)
		}
	default:
		return fmt.Errorf("fixture verdict must be valid or invalid, got %q", entry.Verdict)
	}
	return nil
}

// fixtureChecks registers every document check a fixture entry may name.
// Grammar chapters add checks as their statements land.
var fixtureChecks = map[string]func(string) error{
	"yaml_mapping":             checkYAMLMapping,
	"machine_expansion_shape":  checkMachineExpansionShape,
	"expansion_names_fragment": checkExpansionNamesFragment,
}

// checkFixture dispatches a named document check against a fixture file.
func checkFixture(kind, path string) error {
	check, ok := fixtureChecks[kind]
	if !ok {
		return fmt.Errorf("unknown fixture check %q", kind)
	}
	return check(path)
}

// checkYAMLMapping enforces R-INTRO-001: a single YAML document whose root
// node is a mapping.
func checkYAMLMapping(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("parse fixture: %w", err)
	}
	if len(document.Content) == 0 {
		return errors.New("fixture holds no YAML document")
	}
	if document.Content[0].Kind != yaml.MappingNode {
		return errors.New("root node is not a mapping")
	}
	return nil
}

// checkMachineExpansionShape enforces R-MODEL-001: a machine-profile document
// either declares its own states and transitions (and may splice
// stage-fragments beside them) or carries no body and expands exactly one
// machine-template.
func checkMachineExpansionShape(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	_, hasStates := doc["states"]
	_, hasTransitions := doc["transitions"]
	entries, hasExpansion := doc["expand"]
	switch {
	case hasStates && hasTransitions:
		return nil
	case hasStates || hasTransitions:
		return errors.New("a declared body requires both states and transitions")
	case !hasExpansion:
		return errors.New("no body and no expansion")
	}
	list, ok := entries.([]any)
	if !ok {
		return errors.New("expand must be a sequence")
	}
	if len(list) != 1 {
		return fmt.Errorf("a bodiless machine-profile document must expand exactly one machine-template, got %d entries", len(list))
	}
	return nil
}

// checkExpansionNamesFragment enforces R-MODEL-002: every expansion entry
// names its source unit under fragment. A document with no expansion
// satisfies the statement vacuously.
func checkExpansionNamesFragment(path string) error {
	doc, err := readYAMLMapping(path)
	if err != nil {
		return err
	}
	entries, ok := doc["expand"]
	if !ok {
		return nil
	}
	list, ok := entries.([]any)
	if !ok {
		return errors.New("expand must be a sequence")
	}
	for index, raw := range list {
		entry, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("expand[%d] must be a mapping", index)
		}
		fragment, _ := entry["fragment"].(string)
		if strings.TrimSpace(fragment) == "" {
			return fmt.Errorf("expand[%d] does not name a fragment", index)
		}
	}
	return nil
}

func readYAMLMapping(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parse fixture: %w", err)
	}
	if doc == nil {
		return nil, errors.New("fixture holds no YAML mapping")
	}
	return doc, nil
}

// runGoTestEvidence proves the named test exists in the file at path and
// passes. The declaration check prevents the false green where a -run
// selector matches nothing: `go test` reports "no tests to run" and exits 0.
func runGoTestEvidence(path, name string) error {
	if !strings.HasPrefix(name, "Test") {
		return fmt.Errorf("go_test evidence requires a Test* function, got %q", name)
	}
	if err := checkGoTestDeclared(path, name); err != nil {
		return err
	}
	command := exec.Command("go", "test", "-count=1", "-v", "-run", "^"+regexp.QuoteMeta(name)+"$", ".")
	command.Dir = filepath.Dir(path)
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("go test %s: %w: %s", name, err, strings.TrimSpace(string(output)))
	}
	if !strings.Contains(string(output), "--- PASS: "+name) {
		return fmt.Errorf("go test %s did not run and pass: %s", name, strings.TrimSpace(string(output)))
	}
	return nil
}

func checkGoTestDeclared(path, name string) error {
	if !strings.HasSuffix(path, "_test.go") {
		return fmt.Errorf("go_test evidence requires a *_test.go file, got %s", path)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return err
	}
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Name.Name != name {
			continue
		}
		if isTestingFunc(fn) {
			return nil
		}
		return fmt.Errorf("Go function %q does not take *testing.T", name)
	}
	return fmt.Errorf("Go test %q is not declared in %s", name, path)
}

func isTestingFunc(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	pointer, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := pointer.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "T" {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "testing"
}
