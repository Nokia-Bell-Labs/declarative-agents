// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const referenceImplementationCitation = "reference-implementation"

type patternLanguageEvidence struct {
	Patterns []struct {
		ID       string `yaml:"id"`
		Examples []struct {
			System   string             `yaml:"system"`
			Cite     string             `yaml:"cite"`
			Kind     string             `yaml:"kind"`
			Note     string             `yaml:"note"`
			Evidence *referenceEvidence `yaml:"evidence"`
		} `yaml:"examples"`
	} `yaml:"patterns"`
}

type referenceEvidence struct {
	Classification string          `yaml:"classification"`
	Checks         []evidenceCheck `yaml:"checks"`
}

type evidenceCheck struct {
	Assertion       string            `yaml:"assertion"`
	Artifact        string            `yaml:"artifact"`
	Path            string            `yaml:"path"`
	Paths           []string          `yaml:"paths"`
	Test            string            `yaml:"test"`
	Symbol          string            `yaml:"symbol"`
	Function        string            `yaml:"function"`
	Type            string            `yaml:"type"`
	Fields          []string          `yaml:"fields"`
	Field           string            `yaml:"field"`
	Equals          string            `yaml:"equals"`
	Target          string            `yaml:"target"`
	TargetArtifact  string            `yaml:"target_artifact"`
	SameFields      []string          `yaml:"same_fields"`
	DifferentFields []string          `yaml:"different_fields"`
	Match           map[string]string `yaml:"match"`
}

// Audit verifies reference-implementation claims, then renders figures and builds the PDF.
func Audit() error {
	return auditDesignPatterns(
		func() error { return auditReferenceImplementationEvidence("pattern-language.yaml", "..") },
		All,
	)
}

func auditDesignPatterns(evidence, build func() error) error {
	if err := evidence(); err != nil {
		return err
	}
	return build()
}

func auditReferenceImplementationEvidence(languagePath, repositoryRoot string) error {
	data, err := os.ReadFile(languagePath)
	if err != nil {
		return fmt.Errorf("read pattern language: %w", err)
	}
	var language patternLanguageEvidence
	if err := yaml.Unmarshal(data, &language); err != nil {
		return fmt.Errorf("parse pattern language: %w", err)
	}

	var findings []error
	claims := 0
	for _, pattern := range language.Patterns {
		for _, example := range pattern.Examples {
			if example.Cite != referenceImplementationCitation || example.Kind != "internal" {
				continue
			}
			claims++
			label := fmt.Sprintf("%s / %s", pattern.ID, example.System)
			if err := validateReferenceEvidence(repositoryRoot, label, example.Note, example.Evidence); err != nil {
				findings = append(findings, err)
			}
		}
	}
	if claims == 0 {
		findings = append(findings, errors.New("pattern language has no internal reference-implementation claims"))
	}
	if len(findings) > 0 {
		return fmt.Errorf("reference-implementation evidence audit failed: %w", errors.Join(findings...))
	}
	fmt.Printf("validated %d reference-implementation claims\n", claims)
	return nil
}

func validateReferenceEvidence(repositoryRoot, label, note string, evidence *referenceEvidence) error {
	if evidence == nil {
		return fmt.Errorf("%s: evidence is required", label)
	}
	classification := strings.TrimSpace(evidence.Classification)
	switch classification {
	case "shipped":
		if len(evidence.Checks) == 0 {
			return fmt.Errorf("%s: shipped evidence requires at least one check", label)
		}
	case "conformance_fixture":
		if !strings.Contains(strings.ToLower(note), "conformance fixture") {
			return fmt.Errorf("%s: conformance_fixture note must say \"conformance fixture\"", label)
		}
		if len(evidence.Checks) == 0 {
			return fmt.Errorf("%s: conformance_fixture evidence requires at least one check", label)
		}
	case "design_intent":
		if !strings.Contains(strings.ToLower(note), "design intent") {
			return fmt.Errorf("%s: design_intent note must say \"design intent\"", label)
		}
	default:
		return fmt.Errorf("%s: unknown evidence classification %q", label, classification)
	}

	var findings []error
	for _, check := range evidence.Checks {
		if err := runEvidenceCheck(repositoryRoot, label, check); err != nil {
			findings = append(findings, err)
		}
	}
	return errors.Join(findings...)
}

func runEvidenceCheck(repositoryRoot, label string, check evidenceCheck) error {
	if strings.TrimSpace(check.Assertion) == "" {
		return fmt.Errorf("%s: evidence assertion is required", label)
	}
	if strings.TrimSpace(check.Artifact) == "" {
		return fmt.Errorf("%s: evidence artifact type is required", label)
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return fmt.Errorf("%s: resolve repository root: %w", label, err)
	}
	switch check.Assertion {
	case "go_test", "go_symbol", "go_composite_literal", "yaml_fields", "yaml_value", "yaml_reference", "yaml_transition", "yaml_sequence_match":
		path, err := resolveEvidencePath(root, check.Path)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		if err := validateArtifact(path, check.Artifact); err != nil {
			return fmt.Errorf("%s: evidence path %q: %w", label, check.Path, err)
		}
		switch check.Assertion {
		case "go_test":
			return validateGoTest(path, check.Test)
		case "go_symbol":
			return validateGoSymbol(path, check.Symbol)
		case "go_composite_literal":
			return validateGoCompositeLiteral(path, check.Function, check.Type)
		case "yaml_fields":
			return validateYAMLFields(path, check.Fields)
		case "yaml_value":
			return validateYAMLValue(path, check.Field, check.Equals)
		case "yaml_reference":
			return validateYAMLReference(root, path, check)
		case "yaml_transition":
			return validateYAMLSequenceMatch(path, "transitions", check.Match)
		case "yaml_sequence_match":
			return validateYAMLSequenceMatch(path, check.Field, check.Match)
		}
	case "yaml_relation":
		return validateYAMLRelation(root, check)
	default:
		return fmt.Errorf("%s: unknown evidence assertion %q", label, check.Assertion)
	}
	return nil
}

func resolveEvidencePath(root, relative string) (string, error) {
	if strings.TrimSpace(relative) == "" {
		return "", errors.New("evidence check path is required")
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.Clean(relative)))
	if err != nil {
		return "", fmt.Errorf("resolve evidence path %q: %w", relative, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("evidence path %q escapes repository root", relative)
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("evidence path %q: %w", relative, err)
	}
	return path, nil
}

func validateArtifact(path, artifact string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("artifact %q must be a file", artifact)
	}
	switch artifact {
	case "go_test":
		if !strings.HasSuffix(path, "_test.go") {
			return errors.New("artifact type go_test requires a *_test.go file")
		}
		_, err = parser.ParseFile(token.NewFileSet(), path, nil, 0)
		return err
	case "go_source":
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return errors.New("artifact type go_source requires a non-test .go file")
		}
		_, err = parser.ParseFile(token.NewFileSet(), path, nil, 0)
		return err
	case "machine", "profile", "rest_definition", "tool_declaration", "tool_selection":
		doc, err := readYAMLMap(path)
		if err != nil {
			return err
		}
		return validateYAMLArtifact(doc, artifact)
	default:
		return fmt.Errorf("unknown artifact type %q", artifact)
	}
}

func validateYAMLArtifact(doc map[string]any, artifact string) error {
	required := map[string][]string{
		"machine":          {"name", "states", "transitions"},
		"profile":          {"name", "machine"},
		"tool_declaration": {"tools"},
		"tool_selection":   {"tools"},
	}[artifact]
	for _, field := range required {
		if _, ok := doc[field]; !ok {
			return fmt.Errorf("artifact type %s requires field %q", artifact, field)
		}
	}
	if artifact == "machine" {
		if _, ok := doc["states"].([]any); !ok {
			return errors.New("machine states must be a sequence")
		}
		if _, ok := doc["transitions"].([]any); !ok {
			return errors.New("machine transitions must be a sequence")
		}
	}
	if artifact == "profile" {
		if _, ok := doc["machine"].(string); !ok {
			return errors.New("profile machine must be a string reference")
		}
	}
	if artifact == "rest_definition" {
		rest, ok := doc["rest"].(map[string]any)
		if !ok {
			return errors.New("rest_definition requires a rest mapping")
		}
		if _, ok := rest["servers"].(map[string]any); !ok {
			return errors.New("rest_definition requires rest.servers")
		}
	}
	if artifact == "tool_declaration" || artifact == "tool_selection" {
		tools, ok := doc["tools"].([]any)
		if !ok {
			return fmt.Errorf("%s tools must be a sequence", artifact)
		}
		for _, tool := range tools {
			_, mapping := tool.(map[string]any)
			_, scalar := tool.(string)
			if artifact == "tool_declaration" && !mapping {
				return errors.New("tool_declaration entries must be mappings")
			}
			if artifact == "tool_declaration" {
				entry := tool.(map[string]any)
				if _, ok := entry["name"].(string); !ok {
					return errors.New("tool_declaration entries require string names")
				}
			}
			if artifact == "tool_selection" && !scalar {
				return errors.New("tool_selection entries must be names")
			}
		}
	}
	return nil
}

func validateGoTest(path, name string) error {
	if !strings.HasPrefix(name, "Test") {
		return fmt.Errorf("go_test assertion requires a Test* function, got %q", name)
	}
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return err
	}
	for _, declaration := range file.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == name {
			if isGoTestFunction(fn) {
				return executeFocusedGoTest(path, name)
			}
			return fmt.Errorf("Go function %q does not have a testing.T parameter", name)
		}
	}
	return fmt.Errorf("Go test %q is not declared", name)
}

func executeFocusedGoTest(path, name string) error {
	module, err := nearestGoModule(filepath.Dir(path))
	if err != nil {
		return err
	}
	pkg, err := filepath.Rel(module, filepath.Dir(path))
	if err != nil {
		return err
	}
	pkg = "./" + filepath.ToSlash(pkg)
	command := exec.Command("go", "test", pkg,
		"-run", "^"+regexp.QuoteMeta(name)+"$", "-count=1", "-v", "-json")
	command.Dir = module
	output, runErr := command.CombinedOutput()
	result := scanFocusedGoTestJSON(output, name)
	// A genuine test failure is the most specific signal: report it with the
	// test's own output before considering the raw exit status.
	if result.failed {
		return fmt.Errorf("focused Go test %s in %s failed: %s",
			name, pkg, strings.TrimSpace(result.testOutput))
	}
	// The command errored without a recorded result for the named test
	// (build failure, panic before the test event, etc.).
	if runErr != nil {
		return fmt.Errorf("focused Go test %s in %s failed to run: %w: %s",
			name, pkg, runErr, strings.TrimSpace(string(output)))
	}
	// Exit status was zero but no top-level test with this exact name actually
	// executed. Go reports "no tests to run" and exits 0 when -run matches
	// nothing, which would otherwise pass as false-green evidence.
	if !result.ran {
		return fmt.Errorf("focused Go test %s in %s matched no executed test "+
			"(\"no tests to run\"); the selector must name a real test in the package",
			name, pkg)
	}
	if !result.passed {
		return fmt.Errorf("focused Go test %s in %s produced no pass result event",
			name, pkg)
	}
	return nil
}

// focusedGoTestResult summarizes what the go test2json stream proved about a
// single top-level test selector.
type focusedGoTestResult struct {
	ran        bool
	passed     bool
	failed     bool
	testOutput string
}

// scanFocusedGoTestJSON reads a `go test -json` event stream and reports
// whether the exact top-level test named ran, and its terminal outcome. Only
// events for the exact test name are considered so that a non-matching or
// absent selector cannot be mistaken for an executed pass. Non-JSON lines
// (interleaved build errors) are ignored; the caller inspects the raw output
// and exit status for those cases.
func scanFocusedGoTestJSON(output []byte, name string) focusedGoTestResult {
	var result focusedGoTestResult
	var collected strings.Builder
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var event struct {
			Action string `json:"Action"`
			Test   string `json:"Test"`
			Output string `json:"Output"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if event.Test != name {
			continue
		}
		switch event.Action {
		case "run":
			result.ran = true
		case "output":
			collected.WriteString(event.Output)
		case "pass":
			result.ran = true
			result.passed = true
		case "fail":
			result.ran = true
			result.failed = true
		}
	}
	result.testOutput = collected.String()
	return result
}

func nearestGoModule(start string) (string, error) {
	dir := filepath.Clean(start)
	for {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && !info.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod found above %s", start)
		}
		dir = parent
	}
}

func isGoTestFunction(fn *ast.FuncDecl) bool {
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
	// Prove the parameter is *testing.T, not an unrelated type whose selector
	// happens to be named T.
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "testing"
}

func validateGoSymbol(path, name string) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return err
	}
	for _, declaration := range file.Decls {
		switch decl := declaration.(type) {
		case *ast.FuncDecl:
			if decl.Name.Name == name {
				return nil
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch value := spec.(type) {
				case *ast.TypeSpec:
					if value.Name.Name == name {
						return nil
					}
				case *ast.ValueSpec:
					for _, candidate := range value.Names {
						if candidate.Name == name {
							return nil
						}
					}
				}
			}
		}
	}
	return fmt.Errorf("Go symbol %q is not declared", name)
}

func validateGoCompositeLiteral(path, function, typeName string) error {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return err
	}
	for _, declaration := range file.Decls {
		fn, ok := declaration.(*ast.FuncDecl)
		if !ok || fn.Name.Name != function {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			literal, ok := node.(*ast.CompositeLit)
			if ok && goExpressionName(literal.Type) == typeName {
				found = true
			}
			return true
		})
		if found {
			return nil
		}
	}
	return fmt.Errorf("Go function %q has no %s composite literal", function, typeName)
}

func goExpressionName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		prefix := goExpressionName(value.X)
		if prefix == "" {
			return value.Sel.Name
		}
		return prefix + "." + value.Sel.Name
	}
	return ""
}

func readYAMLMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func validateYAMLFields(path string, fields []string) error {
	if len(fields) == 0 {
		return errors.New("yaml_fields assertion requires fields")
	}
	doc, err := readYAMLMap(path)
	if err != nil {
		return err
	}
	for _, field := range fields {
		if _, ok := yamlField(doc, field); !ok {
			return fmt.Errorf("YAML field %q is absent", field)
		}
	}
	return nil
}

func validateYAMLValue(path, field, expected string) error {
	doc, err := readYAMLMap(path)
	if err != nil {
		return err
	}
	value, ok := yamlField(doc, field)
	if !ok {
		return fmt.Errorf("YAML field %q is absent", field)
	}
	if fmt.Sprint(value) != expected {
		return fmt.Errorf("YAML field %q = %q, want %q",
			field, fmt.Sprint(value), expected)
	}
	return nil
}

func yamlField(doc map[string]any, field string) (any, bool) {
	var current any = doc
	for _, part := range strings.Split(field, ".") {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = mapping[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

func validateYAMLReference(root, path string, check evidenceCheck) error {
	doc, err := readYAMLMap(path)
	if err != nil {
		return err
	}
	value, ok := yamlField(doc, check.Field)
	if !ok {
		return fmt.Errorf("YAML reference field %q is absent", check.Field)
	}
	references := yamlStrings(value)
	if len(references) == 0 {
		return fmt.Errorf("YAML reference field %q has no string references", check.Field)
	}
	target, err := resolveEvidencePath(root, check.Target)
	if err != nil {
		return err
	}
	if err := validateArtifact(target, check.TargetArtifact); err != nil {
		return fmt.Errorf("reference target: %w", err)
	}
	for _, reference := range references {
		resolved := filepath.Clean(filepath.Join(filepath.Dir(path), filepath.FromSlash(reference)))
		if resolved == target {
			return nil
		}
	}
	return fmt.Errorf("field %q does not reference %q", check.Field, check.Target)
}

func yamlStrings(value any) []string {
	if text, ok := value.(string); ok {
		return []string{text}
	}
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	var stringsOnly []string
	for _, value := range values {
		if text, ok := value.(string); ok {
			stringsOnly = append(stringsOnly, text)
		}
	}
	return stringsOnly
}

func validateYAMLSequenceMatch(path, field string, match map[string]string) error {
	if len(match) == 0 {
		return errors.New("YAML sequence assertion requires match fields")
	}
	doc, err := readYAMLMap(path)
	if err != nil {
		return err
	}
	items, ok := doc[field].([]any)
	if !ok {
		return fmt.Errorf("YAML field %q is not a sequence", field)
	}
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		matches := true
		for field, expected := range match {
			if fmt.Sprint(item[field]) != expected {
				matches = false
				break
			}
		}
		if matches {
			return nil
		}
	}
	return fmt.Errorf("no entry in YAML field %q matches %v", field, match)
}

func validateYAMLRelation(root string, check evidenceCheck) error {
	if len(check.Paths) < 2 {
		return errors.New("yaml_relation assertion requires at least two paths")
	}
	var docs []map[string]any
	for _, relative := range check.Paths {
		path, err := resolveEvidencePath(root, relative)
		if err != nil {
			return err
		}
		if err := validateArtifact(path, check.Artifact); err != nil {
			return fmt.Errorf("evidence path %q: %w", relative, err)
		}
		doc, err := readYAMLMap(path)
		if err != nil {
			return err
		}
		docs = append(docs, doc)
	}
	for _, field := range check.SameFields {
		first, ok := yamlField(docs[0], field)
		if !ok {
			return fmt.Errorf("same field %q is absent", field)
		}
		for _, doc := range docs[1:] {
			value, ok := yamlField(doc, field)
			if !ok || !reflect.DeepEqual(value, first) {
				return fmt.Errorf("field %q is not equal across related artifacts", field)
			}
		}
	}
	for _, field := range check.DifferentFields {
		first, ok := yamlField(docs[0], field)
		if !ok {
			return fmt.Errorf("different field %q is absent", field)
		}
		allEqual := true
		for _, doc := range docs[1:] {
			value, ok := yamlField(doc, field)
			if !ok {
				return fmt.Errorf("different field %q is absent", field)
			}
			if !reflect.DeepEqual(value, first) {
				allEqual = false
			}
		}
		if allEqual {
			return fmt.Errorf("field %q is equal across artifacts but must differ", field)
		}
	}
	if len(check.SameFields) == 0 && len(check.DifferentFields) == 0 {
		return errors.New("yaml_relation requires same_fields or different_fields")
	}
	return nil
}
