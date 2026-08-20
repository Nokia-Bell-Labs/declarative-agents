// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type goPackageImports struct {
	ImportPath string
	Imports    []string
	Dir        string
	GoFiles    []string
}

type inferenceBoundaryPolicy struct {
	module          string
	adapterPackages []string
	providerImports []string
}

type declaredFieldPolicy struct {
	module                  string
	rootTypes               []string
	wholeValueFunctions     []string
	documentationOnlyFields []string
}

type parsedPackageFile struct {
	path       string
	importPath string
	imports    map[string]string
	file       *ast.File
}

type yamlStructField struct {
	goName   string
	yamlName string
	file     string
	children []string
}

type yamlStructType struct {
	key      string
	fields   []yamlStructField
	children []string
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

func TestPatternInvariantDeclaredFields(t *testing.T) {
	t.Parallel()

	root := repositoryRoot(t)
	policy, err := loadDeclaredFieldPolicy(root)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := listGoPackageImports(root, policy.module)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkDeclaredFieldReads(packages, policy); err != nil {
		t.Fatal(err)
	}
}

func TestPatternInvariantDeclaredFieldRejectsUnreadField(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFixture(t, dir, `package fixture
type Config struct {
	Read string `+"`yaml:\"read\"`"+`
	Unread string `+"`yaml:\"unread\"`"+`
}
func consume(c Config) string { return c.Read }
`)
	packages := fixtureGoPackages(dir)
	policy := declaredFieldPolicy{rootTypes: []string{"example.com/fixture.Config"}}
	err := checkDeclaredFieldReads(packages, policy)
	if err == nil || !strings.Contains(err.Error(), "fixture.go") ||
		!strings.Contains(err.Error(), "Config.unread") {
		t.Fatalf("error = %v, want fixture file and unread field", err)
	}
}

func TestPatternInvariantDeclaredFieldWholeValueConsumption(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFixture(t, dir, `package fixture
import "gopkg.in/yaml.v3"
type Config struct {
	Nested Child `+"`yaml:\"nested\"`"+`
}
type Child struct {
	Value string `+"`yaml:\"value\"`"+`
}
func render(c Config) { _, _ = yaml.Marshal(c) }
`)
	policy := declaredFieldPolicy{
		rootTypes:           []string{"example.com/fixture.Config"},
		wholeValueFunctions: []string{"gopkg.in/yaml.v3.Marshal"},
	}
	if err := checkDeclaredFieldReads(fixtureGoPackages(dir), policy); err != nil {
		t.Fatalf("whole-value marshal did not consume child fields: %v", err)
	}
}

func TestPatternInvariantDeclaredFieldParentPresenceIsNotChildRead(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeGoFixture(t, dir, `package fixture
type Config struct {
	Children []Child `+"`yaml:\"children\"`"+`
}
type Child struct {
	Detail string `+"`yaml:\"detail\"`"+`
}
func present(c Config) bool { return len(c.Children) > 0 }
`)
	policy := declaredFieldPolicy{rootTypes: []string{"example.com/fixture.Config"}}
	err := checkDeclaredFieldReads(fixtureGoPackages(dir), policy)
	if err == nil || !strings.Contains(err.Error(), "Child.detail") {
		t.Fatalf("parent presence incorrectly consumed child field: %v", err)
	}
}

func loadDeclaredFieldPolicy(root string) (declaredFieldPolicy, error) {
	language, err := loadPatternInvariants(filepath.Join(root, patternLanguagePath))
	if err != nil {
		return declaredFieldPolicy{}, err
	}
	for _, pattern := range language.Patterns {
		for _, invariant := range pattern.Invariants {
			if invariant.ID != "P2.3" || invariant.Check == nil {
				continue
			}
			check := invariant.Check
			if check.Module == "" || len(check.RootTypes) == 0 {
				return declaredFieldPolicy{}, errors.New("P2.3 requires module and root_types")
			}
			return declaredFieldPolicy{
				module:                  check.Module,
				rootTypes:               append([]string(nil), check.RootTypes...),
				wholeValueFunctions:     append([]string(nil), check.WholeValueFuncs...),
				documentationOnlyFields: append([]string(nil), check.DocumentationOnlyFields...),
			}, nil
		}
	}
	return declaredFieldPolicy{}, errors.New("pattern language has no P2.3 invariant")
}

func checkDeclaredFieldReads(packages []goPackageImports, policy declaredFieldPolicy) error {
	files, err := parsePackageGoFiles(packages)
	if err != nil {
		return err
	}
	types := collectYAMLTypes(files)
	fields := reachableYAMLFields(types, policy.rootTypes)
	reads := collectSelectorReads(files)
	wholeTypes := collectWholeValueTypes(files, types, policy.wholeValueFunctions)
	wholeReads := make(map[string]bool)
	for typeKey := range wholeTypes {
		markWholeTypeFields(typeKey, types, wholeReads, make(map[string]bool))
	}
	documentationOnly := stringSet(policy.documentationOnlyFields)
	var unread []string
	for id, field := range fields {
		if reads[field.goName] || wholeReads[id] || documentationOnly[id] {
			continue
		}
		unread = append(unread, fmt.Sprintf("%s: %s has no interpreter read", field.file, id))
	}
	sort.Strings(unread)
	if len(unread) > 0 {
		return errors.New(strings.Join(unread, "\n"))
	}
	return nil
}

func parsePackageGoFiles(packages []goPackageImports) ([]parsedPackageFile, error) {
	var files []parsedPackageFile
	fset := token.NewFileSet()
	for _, pkg := range packages {
		for _, name := range pkg.GoFiles {
			actualPath := filepath.Join(pkg.Dir, name)
			file, err := parser.ParseFile(fset, actualPath, nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", actualPath, err)
			}
			files = append(files, parsedPackageFile{
				path:       filepath.ToSlash(filepath.Join(pkg.ImportPath, name)),
				importPath: pkg.ImportPath,
				imports:    importAliases(file),
				file:       file,
			})
		}
	}
	return files, nil
}

func importAliases(file *ast.File) map[string]string {
	aliases := make(map[string]string)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := defaultImportName(path)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name != "_" && name != "." {
			aliases[name] = path
		}
	}
	return aliases
}

func defaultImportName(path string) string {
	name := filepath.Base(path)
	if version := strings.LastIndex(name, ".v"); version > 0 {
		if _, err := strconv.Atoi(name[version+2:]); err == nil {
			return name[:version]
		}
	}
	return name
}

func collectYAMLTypes(files []parsedPackageFile) map[string]*yamlStructType {
	types := make(map[string]*yamlStructType)
	for _, parsed := range files {
		for _, declaration := range parsed.file.Decls {
			gen, ok := declaration.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec := spec.(*ast.TypeSpec)
				key := parsed.importPath + "." + typeSpec.Name.Name
				types[key] = &yamlStructType{key: key}
			}
		}
	}
	for _, parsed := range files {
		populateYAMLTypes(parsed, types)
	}
	return types
}

func populateYAMLTypes(parsed parsedPackageFile, types map[string]*yamlStructType) {
	for _, declaration := range parsed.file.Decls {
		gen, ok := declaration.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec := spec.(*ast.TypeSpec)
			key := parsed.importPath + "." + typeSpec.Name.Name
			info := types[key]
			structType, isStruct := typeSpec.Type.(*ast.StructType)
			if !isStruct {
				info.children = typeRefs(typeSpec.Type, parsed, types)
				continue
			}
			for _, field := range structType.Fields.List {
				info.fields = append(info.fields, yamlFields(field, parsed, types)...)
			}
		}
	}
}

func yamlFields(field *ast.Field, parsed parsedPackageFile, types map[string]*yamlStructType) []yamlStructField {
	if field.Tag == nil || len(field.Names) == 0 {
		return nil
	}
	tag, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return nil
	}
	yamlName := strings.Split(reflect.StructTag(tag).Get("yaml"), ",")[0]
	if yamlName == "" || yamlName == "-" {
		return nil
	}
	children := typeRefs(field.Type, parsed, types)
	fields := make([]yamlStructField, 0, len(field.Names))
	for _, name := range field.Names {
		fields = append(fields, yamlStructField{
			goName: name.Name, yamlName: yamlName, file: parsed.path,
			children: append([]string(nil), children...),
		})
	}
	return fields
}

func typeRefs(expr ast.Expr, parsed parsedPackageFile, types map[string]*yamlStructType) []string {
	var candidates []string
	switch value := expr.(type) {
	case *ast.Ident:
		candidates = append(candidates, parsed.importPath+"."+value.Name)
	case *ast.SelectorExpr:
		if alias, ok := value.X.(*ast.Ident); ok {
			candidates = append(candidates, parsed.imports[alias.Name]+"."+value.Sel.Name)
		}
	case *ast.ArrayType:
		candidates = append(candidates, typeRefs(value.Elt, parsed, types)...)
	case *ast.StarExpr:
		candidates = append(candidates, typeRefs(value.X, parsed, types)...)
	case *ast.MapType:
		candidates = append(candidates, typeRefs(value.Value, parsed, types)...)
	}
	var refs []string
	for _, candidate := range candidates {
		if types[candidate] != nil {
			refs = append(refs, candidate)
		}
	}
	return refs
}

func reachableYAMLFields(types map[string]*yamlStructType, roots []string) map[string]yamlStructField {
	fields := make(map[string]yamlStructField)
	queue := append([]string(nil), roots...)
	visited := make(map[string]bool)
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if visited[key] || types[key] == nil {
			continue
		}
		visited[key] = true
		info := types[key]
		queue = append(queue, info.children...)
		for _, field := range info.fields {
			fields[key+"."+field.yamlName] = field
			queue = append(queue, field.children...)
		}
	}
	return fields
}

func collectSelectorReads(files []parsedPackageFile) map[string]bool {
	reads := make(map[string]bool)
	for _, parsed := range files {
		writes := make(map[*ast.SelectorExpr]bool)
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			assign, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				if selector, ok := lhs.(*ast.SelectorExpr); ok {
					writes[selector] = true
				}
			}
			return true
		})
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && !writes[selector] {
				reads[selector.Sel.Name] = true
			}
			return true
		})
	}
	return reads
}

func collectWholeValueTypes(
	files []parsedPackageFile,
	types map[string]*yamlStructType,
	wholeValueFunctions []string,
) map[string]bool {
	wholeFunctions := stringSet(wholeValueFunctions)
	consumed := make(map[string]bool)
	for _, parsed := range files {
		for _, declaration := range parsed.file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Body == nil {
				continue
			}
			env := functionTypeEnvironment(function, parsed, types)
			ast.Inspect(function.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || !wholeFunctions[calledFunction(call.Fun, parsed.imports)] {
					return true
				}
				for _, argument := range call.Args {
					if key := expressionType(argument, env, parsed, types); key != "" {
						consumed[key] = true
					}
				}
				return true
			})
		}
	}
	return consumed
}

func functionTypeEnvironment(
	function *ast.FuncDecl,
	parsed parsedPackageFile,
	types map[string]*yamlStructType,
) map[string]string {
	env := make(map[string]string)
	bindFieldList(env, function.Recv, parsed, types)
	bindFieldList(env, function.Type.Params, parsed, types)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.DeclStmt:
			if declaration, ok := value.Decl.(*ast.GenDecl); ok {
				bindValueSpec(env, declaration, parsed, types)
			}
		case *ast.AssignStmt:
			bindAssignment(env, value, parsed, types)
		case *ast.RangeStmt:
			if ident, ok := value.Value.(*ast.Ident); ok {
				env[ident.Name] = expressionType(value.X, env, parsed, types)
			}
		}
		return true
	})
	return env
}

func bindFieldList(
	env map[string]string,
	list *ast.FieldList,
	parsed parsedPackageFile,
	types map[string]*yamlStructType,
) {
	if list == nil {
		return
	}
	for _, field := range list.List {
		refs := typeRefs(field.Type, parsed, types)
		if len(refs) == 0 {
			continue
		}
		for _, name := range field.Names {
			env[name.Name] = refs[0]
		}
	}
}

func bindValueSpec(
	env map[string]string,
	declaration *ast.GenDecl,
	parsed parsedPackageFile,
	types map[string]*yamlStructType,
) {
	for _, spec := range declaration.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		refs := typeRefs(value.Type, parsed, types)
		if len(refs) == 0 {
			continue
		}
		for _, name := range value.Names {
			env[name.Name] = refs[0]
		}
	}
}

func bindAssignment(
	env map[string]string,
	assign *ast.AssignStmt,
	parsed parsedPackageFile,
	types map[string]*yamlStructType,
) {
	if assign.Tok != token.DEFINE {
		return
	}
	for index, lhs := range assign.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || index >= len(assign.Rhs) {
			continue
		}
		if key := expressionType(assign.Rhs[index], env, parsed, types); key != "" {
			env[ident.Name] = key
		}
	}
}

func expressionType(
	expr ast.Expr,
	env map[string]string,
	parsed parsedPackageFile,
	types map[string]*yamlStructType,
) string {
	switch value := expr.(type) {
	case *ast.Ident:
		return env[value.Name]
	case *ast.CompositeLit:
		refs := typeRefs(value.Type, parsed, types)
		if len(refs) > 0 {
			return refs[0]
		}
	case *ast.UnaryExpr:
		return expressionType(value.X, env, parsed, types)
	case *ast.ParenExpr:
		return expressionType(value.X, env, parsed, types)
	case *ast.IndexExpr:
		return expressionType(value.X, env, parsed, types)
	case *ast.SelectorExpr:
		parent := expressionType(value.X, env, parsed, types)
		info := types[parent]
		if info == nil {
			return ""
		}
		for _, field := range info.fields {
			if field.goName == value.Sel.Name && len(field.children) > 0 {
				return field.children[0]
			}
		}
	}
	return ""
}

func calledFunction(expr ast.Expr, imports map[string]string) string {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	alias, ok := selector.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return imports[alias.Name] + "." + selector.Sel.Name
}

func markWholeTypeFields(
	key string,
	types map[string]*yamlStructType,
	reads map[string]bool,
	visited map[string]bool,
) {
	if visited[key] || types[key] == nil {
		return
	}
	visited[key] = true
	info := types[key]
	for _, child := range info.children {
		markWholeTypeFields(child, types, reads, visited)
	}
	for _, field := range info.fields {
		reads[key+"."+field.yamlName] = true
		for _, child := range field.children {
			markWholeTypeFields(child, types, reads, visited)
		}
	}
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func writeGoFixture(t *testing.T, dir, source string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixtureGoPackages(dir string) []goPackageImports {
	return []goPackageImports{{
		ImportPath: "example.com/fixture",
		Dir:        dir,
		GoFiles:    []string{"fixture.go"},
	}}
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
