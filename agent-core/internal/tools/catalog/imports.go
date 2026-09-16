// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
)

var toolUnitName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// ToolDefsFile is the top-level YAML structure for declaration files.
type ToolDefsFile struct {
	Unit           string                    `yaml:"unit,omitempty"`
	Imports        []string                  `yaml:"imports,omitempty"`
	Params         []fragments.Param         `yaml:"params,omitempty"`
	Instantiate    []fragments.Instantiation `yaml:"instantiate,omitempty"`
	Tools          []ToolDef                 `yaml:"tools,omitempty"`
	Types          []typesys.TypeDecl        `yaml:"types,omitempty"`
	hasTools       bool
	hasTypes       bool
	hasImports     bool
	hasParams      bool
	hasInstantiate bool
}

// IsTypeUnit reports a declaration file that carries types instead of tools
// (srd051 R1.1). A type unit contributes no tools; it contributes named types
// and may import further units of either kind.
func (f ToolDefsFile) IsTypeUnit() bool { return f.hasTypes && !f.hasTools }

// IsFragment reports a declaration file with declared parameters, which is
// only ever instantiated and never imported plainly (srd052 R1.1, R2.6).
func (f ToolDefsFile) IsFragment() bool { return f.hasParams }

// ToolSource identifies the declaration unit and file that owns one tool.
type ToolSource struct {
	Unit string
	Path string
}

// ToolImport is one authored dependency between declaration units. Args is
// set when the edge is an instantiation rather than an import, carrying the
// arguments so an unused-instantiation diagnostic can name them (srd052 R3.1).
type ToolImport struct {
	Importer ToolSource
	Imported ToolSource
	Args     map[string]string
}

// LoadOptions selects the documented runtime or audit-side declaration policy.
type LoadOptions struct {
	TolerateNonToolFiles bool
	ExpandEnv            bool
}

// DeclarationSource returns the tool's loader-assigned provenance.
func (td ToolDef) DeclarationSource() ToolSource {
	return ToolSource{Unit: td.sourceUnit, Path: td.sourcePath}
}

// OverrideTarget returns the imported source replaced by this local tool.
func (td ToolDef) OverrideTarget() ToolSource {
	return td.overrideTarget
}

type toolImportResolver struct {
	visit          FileVisitor
	files          map[string]ToolDefsFile
	raw            map[string][]byte
	resolved       map[string][]ToolDef
	units          map[string]ToolSource
	visiting       map[string]int
	stack          []string
	imports        []ToolImport
	typeUnits      []typesys.TypeUnitFile
	hasImportEdges bool
	options        LoadOptions
}

func newToolImportResolver(visit FileVisitor) *toolImportResolver {
	return newToolImportResolverWithOptions(visit, runtimeLoadOptions())
}

func newToolImportResolverWithOptions(
	visit FileVisitor,
	options LoadOptions,
) *toolImportResolver {
	return &toolImportResolver{
		visit: visit,
		files: map[string]ToolDefsFile{}, raw: map[string][]byte{}, resolved: map[string][]ToolDef{},
		units: map[string]ToolSource{}, visiting: map[string]int{},
		options: options,
	}
}

func runtimeLoadOptions() LoadOptions {
	return LoadOptions{ExpandEnv: true}
}

func (r *toolImportResolver) loadRoots(paths []string) ([]ToolDef, error) {
	var all []ToolDef
	for _, path := range paths {
		defs, err := r.resolve(path, false)
		if err != nil {
			return nil, err
		}
		all = MergeToolDefs(all, defs)
	}
	return all, nil
}

func (r *toolImportResolver) resolve(
	path string, imported bool,
) ([]ToolDef, error) {
	path, err := canonicalToolDeclarationPath(path)
	if err != nil {
		return nil, err
	}
	if index, ok := r.visiting[path]; ok {
		return nil, r.cycleError(index, path)
	}
	file, err := r.readFile(path)
	if err != nil {
		return nil, err
	}
	if !file.hasTools && r.options.TolerateNonToolFiles {
		r.resolved[path] = nil
		return nil, nil
	}
	if err := r.validateFile(file, path, imported); err != nil {
		return nil, err
	}
	if defs, ok := r.resolved[path]; ok {
		return defs, nil
	}
	r.registerEdges(file, path)
	r.visiting[path] = len(r.stack)
	r.stack = append(r.stack, path)
	defs, err := r.resolveFile(file, path)
	r.stack = r.stack[:len(r.stack)-1]
	delete(r.visiting, path)
	if err != nil {
		return nil, err
	}
	r.resolved[path] = defs
	return defs, nil
}

func (r *toolImportResolver) resolveFile(file ToolDefsFile, path string) ([]ToolDef, error) {
	if file.IsTypeUnit() {
		r.typeUnits = append(r.typeUnits, typesys.TypeUnitFile{
			Path: path, Unit: file.Unit, Imports: file.Imports, Types: file.Types,
		})
		if len(file.Imports) == 0 {
			return nil, nil
		}
		return r.resolveImports(file, path)
	}
	source := ToolSource{Unit: file.Unit, Path: path}
	local := annotateToolSources(file.Tools, source)
	if err := validateAndDefaultToolDefs(local); err != nil {
		return nil, fmt.Errorf("tool unit %q at %s: %w", file.Unit, path, err)
	}
	if len(file.Imports) == 0 && len(file.Instantiate) == 0 {
		if hasToolOverride(local) {
			return nil, fmt.Errorf("tool unit %q at %s declares override without an imported target", file.Unit, path)
		}
		return local, nil
	}
	dependencies, err := r.resolveDependencies(file, path)
	if err != nil {
		return nil, err
	}
	return applyLocalTools(dependencies, local, source)
}

// resolveDependencies gathers what a unit imports and what it instantiates.
// Both arrive as imported tools: an instantiation is an import whose unit was
// filled in on the way (srd052 R2.4).
func (r *toolImportResolver) resolveDependencies(file ToolDefsFile, path string) ([]ToolDef, error) {
	imported, err := r.resolveImports(file, path)
	if err != nil {
		return nil, err
	}
	instantiated, err := r.resolveInstantiations(file, path)
	if err != nil {
		return nil, err
	}
	merged, err := mergeImportedTools(imported, instantiated)
	if err != nil {
		return nil, fmt.Errorf("tool unit %q at %s: %w", file.Unit, path, err)
	}
	return merged, nil
}

func (r *toolImportResolver) resolveImports(file ToolDefsFile, path string) ([]ToolDef, error) {
	var merged []ToolDef
	for _, importPath := range file.Imports {
		target, err := canonicalToolDeclarationPath(filepath.Join(filepath.Dir(path), importPath))
		if err != nil {
			return nil, err
		}
		defs, err := r.resolve(target, true)
		if err != nil {
			return nil, fmt.Errorf("tool unit %q at %s imports %q: %w", file.Unit, path, importPath, err)
		}
		importedFile := r.files[target]
		r.imports = append(r.imports, ToolImport{
			Importer: ToolSource{Unit: file.Unit, Path: path},
			Imported: ToolSource{Unit: importedFile.Unit, Path: target},
		})
		merged, err = mergeImportedTools(merged, defs)
		if err != nil {
			return nil, fmt.Errorf("tool unit %q at %s: %w", file.Unit, path, err)
		}
	}
	return merged, nil
}

func (r *toolImportResolver) readFile(path string) (ToolDefsFile, error) {
	if file, ok := r.files[path]; ok {
		return file, nil
	}
	file, raw, err := readToolDefsFile(path, r.options, r.visit)
	if err != nil {
		return ToolDefsFile{}, err
	}
	r.files[path] = file
	r.raw[path] = raw
	return file, nil
}

func (r *toolImportResolver) validateFile(
	file ToolDefsFile, path string, imported bool,
) error {
	if !file.hasTools && !file.hasTypes {
		return fmt.Errorf("tool declaration %s: top-level tools field is required", path)
	}
	if file.hasTypes && file.hasTools {
		return fmt.Errorf("declaration %s cannot carry both tools and types", path)
	}
	if file.IsTypeUnit() && file.Unit == "" {
		return fmt.Errorf("type declaration %s must declare unit", path)
	}
	if err := validateFragmentFile(file, path, imported); err != nil {
		return err
	}
	if file.Unit != "" && !toolUnitName.MatchString(file.Unit) {
		return fmt.Errorf("tool declaration %s has invalid unit %q", path, file.Unit)
	}
	if (imported || file.hasImports) && file.Unit == "" {
		return fmt.Errorf("tool declaration %s must declare unit", path)
	}
	if err := validateToolImportPaths(file, path); err != nil {
		return err
	}
	if file.Unit != "" || len(file.Imports) > 0 {
		if err := rejectLocalToolDuplicates(file.Tools, ToolSource{Unit: file.Unit, Path: path}); err != nil {
			return err
		}
	}
	return r.registerUnit(file.Unit, path)
}

func validateToolImportPaths(file ToolDefsFile, path string) error {
	for _, importPath := range file.Imports {
		if strings.TrimSpace(importPath) == "" {
			return fmt.Errorf("tool unit %q at %s has an empty import path", file.Unit, path)
		}
		if filepath.IsAbs(importPath) {
			return fmt.Errorf("tool unit %q at %s imports absolute path %q", file.Unit, path, importPath)
		}
	}
	return nil
}

func (r *toolImportResolver) registerUnit(unit, path string) error {
	if unit == "" {
		return nil
	}
	if previous, exists := r.units[unit]; exists && previous.Path != path {
		return fmt.Errorf("duplicate tool unit %q: %s and %s", unit, previous.Path, path)
	}
	r.units[unit] = ToolSource{Unit: unit, Path: path}
	return nil
}

func (r *toolImportResolver) registerEdges(file ToolDefsFile, path string) {
	if len(file.Imports) > 0 || len(file.Instantiate) > 0 {
		r.hasImportEdges = true
	}
}

func (r *toolImportResolver) cycleError(index int, path string) error {
	chain := append(append([]string(nil), r.stack[index:]...), path)
	return fmt.Errorf("tool import cycle: %s", strings.Join(chain, " -> "))
}

func canonicalToolDeclarationPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve path %s: %w", path, err)
	}
	return filepath.Clean(absolute), nil
}

func annotateToolSources(tools []ToolDef, source ToolSource) []ToolDef {
	annotated := make([]ToolDef, len(tools))
	for index, tool := range tools {
		tool.sourceUnit = source.Unit
		tool.sourcePath = source.Path
		annotated[index] = tool
	}
	return annotated
}

func rejectLocalToolDuplicates(tools []ToolDef, source ToolSource) error {
	seen := map[string]bool{}
	for _, tool := range tools {
		if seen[tool.Name] {
			return fmt.Errorf(
				"duplicate local tool %q in unit %q at %s", tool.Name, source.Unit, source.Path,
			)
		}
		seen[tool.Name] = true
	}
	return nil
}

func mergeImportedTools(base, incoming []ToolDef) ([]ToolDef, error) {
	result := append([]ToolDef(nil), base...)
	index := toolIndexes(result)
	for _, tool := range incoming {
		if position, exists := index[tool.Name]; exists {
			previous := result[position]
			if previous.sourcePath == tool.sourcePath {
				continue
			}
			return nil, fmt.Errorf(
				"duplicate imported tool %q: %s and %s",
				tool.Name, formatToolSource(previous.DeclarationSource()), formatToolSource(tool.DeclarationSource()),
			)
		}
		index[tool.Name] = len(result)
		result = append(result, tool)
	}
	return result, nil
}

func applyLocalTools(imported, local []ToolDef, source ToolSource) ([]ToolDef, error) {
	result := append([]ToolDef(nil), imported...)
	index := toolIndexes(result)
	for _, tool := range local {
		position, exists := index[tool.Name]
		switch {
		case exists && !tool.Override:
			return nil, fmt.Errorf(
				"local tool %q in %s collides with imported %s without override",
				tool.Name, formatToolSource(source), formatToolSource(result[position].DeclarationSource()),
			)
		case !exists && tool.Override:
			return nil, fmt.Errorf("tool %q in %s overrides no imported target", tool.Name, formatToolSource(source))
		case exists:
			tool.overrideTarget = result[position].DeclarationSource()
			result[position] = tool
		default:
			index[tool.Name] = len(result)
			result = append(result, tool)
		}
	}
	return result, nil
}

func toolIndexes(tools []ToolDef) map[string]int {
	index := make(map[string]int, len(tools))
	for position, tool := range tools {
		index[tool.Name] = position
	}
	return index
}

func hasToolOverride(tools []ToolDef) bool {
	for _, tool := range tools {
		if tool.Override {
			return true
		}
	}
	return false
}

func formatToolSource(source ToolSource) string {
	return fmt.Sprintf("unit %q at %s", source.Unit, source.Path)
}

func productionToolImportResolver(visit FileVisitor) *toolImportResolver {
	return newToolImportResolver(visit)
}

// typeUnitFiles returns the type units reached through the closure, in visit
// order. Build sorts them, so ordering here only has to be deterministic.
func (r *toolImportResolver) typeUnitFiles() []typesys.TypeUnitFile {
	return r.typeUnits
}

func (r *toolImportResolver) importEdges() []ToolImport {
	return append([]ToolImport(nil), r.imports...)
}
