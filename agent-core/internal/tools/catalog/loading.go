// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/envexpand"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/yamlstrict"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/typesys"
	"gopkg.in/yaml.v3"
)

// FileVisitor observes one declaration file after it is read and before it is
// decoded. Loaders use it to build the resolved profile closure without
// walking the same declarations again.
type FileVisitor func(string, []byte) error

// LoadToolSelection reads a YAML file listing tool names.
func LoadToolSelection(path string) ([]string, error) {
	return LoadToolSelectionWithVisitor(path, nil)
}

// LoadToolSelectionWithVisitor reads one selection and reports its source.
func LoadToolSelectionWithVisitor(path string, visit FileVisitor) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load tool selection %s: %w", path, err)
	}
	if visit != nil {
		if err := visit(path, data); err != nil {
			return nil, fmt.Errorf("visit tool selection %s: %w", path, err)
		}
	}
	var sel ToolSelectionFile
	if err := yamlstrict.Unmarshal(data, &sel); err != nil {
		return nil, fmt.Errorf("parse tool selection %s: %w", path, err)
	}
	return sel.Tools, nil
}

// LoadToolSelections reads multiple selection files and deduplicates names.
func LoadToolSelections(paths []string) ([]string, error) {
	return LoadToolSelectionsWithVisitor(paths, nil)
}

// LoadToolSelectionsWithVisitor reads selections and reports every source.
func LoadToolSelectionsWithVisitor(paths []string, visit FileVisitor) ([]string, error) {
	seen := map[string]bool{}
	var merged []string
	for _, p := range paths {
		names, err := LoadToolSelectionWithVisitor(p, visit)
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				merged = append(merged, n)
			}
		}
	}
	return merged, nil
}

// LoadToolDeclarations loads multiple declaration files and merges them.
func LoadToolDeclarations(paths []string) ([]ToolDef, error) {
	return LoadToolDeclarationsWithVisitor(paths, nil)
}

// LoadToolDeclarationsWithVisitor loads declarations and reports every source,
// including transitively included files.
func LoadToolDeclarationsWithVisitor(paths []string, visit FileVisitor) ([]ToolDef, error) {
	return productionToolImportResolver(visit).loadRoots(paths)
}

// LoadToolDeclarationsWithOptions loads declarations under an explicit
// runtime or audit-side parsing policy.
func LoadToolDeclarationsWithOptions(
	paths []string, options LoadOptions, visit FileVisitor,
) ([]ToolDef, error) {
	return newToolImportResolverWithOptions(visit, options).loadRoots(paths)
}

// LoadToolDeclarationsFromDirs scans directories for sorted *.yaml files.
func LoadToolDeclarationsFromDirs(dirs []string) ([]ToolDef, error) {
	return LoadToolDeclarationsFromDirsWithVisitor(dirs, nil)
}

// LoadToolDeclarationsFromDirsWithVisitor scans declaration directories and
// reports every source, including transitively included files.
func LoadToolDeclarationsFromDirsWithVisitor(dirs []string, visit FileVisitor) ([]ToolDef, error) {
	paths, err := toolDeclarationPaths(dirs)
	if err != nil {
		return nil, err
	}
	return productionToolImportResolver(visit).loadRoots(paths)
}

// LoadToolDeclarationClosure loads directory and explicit declarations with
// one source cache while preserving their separate merge precedence.
func LoadToolDeclarationClosure(
	dirs, explicit []string, visit FileVisitor,
) ([]ToolDef, []ToolDef, error) {
	closure, err := LoadToolDeclarationClosureWithImports(dirs, explicit, visit)
	return closure.FromDirs, closure.Local, err
}

// ToolClosure is the resolved tool declaration closure: the tools from config
// directories and from explicit declarations, the authored import edges
// closure-level usedness validates, and the type units reached along the way
// (srd051 R5.1).
type ToolClosure struct {
	FromDirs  []ToolDef
	Local     []ToolDef
	Imports   []ToolImport
	TypeUnits []typesys.TypeUnitFile
}

// LoadToolDeclarationClosureWithImports also returns authored import edges for
// closure-level usedness validation and every type unit the closure reaches.
func LoadToolDeclarationClosureWithImports(
	dirs, explicit []string, visit FileVisitor,
) (ToolClosure, error) {
	paths, err := toolDeclarationPaths(dirs)
	if err != nil {
		return ToolClosure{}, err
	}
	resolver := productionToolImportResolver(visit)
	fromDirs, err := resolver.loadRoots(paths)
	if err != nil {
		return ToolClosure{}, err
	}
	local, err := resolver.loadRoots(explicit)
	if err != nil {
		return ToolClosure{}, err
	}
	return ToolClosure{
		FromDirs: fromDirs, Local: local,
		Imports: resolver.importEdges(), TypeUnits: resolver.typeUnitFiles(),
	}, nil
}

func toolDeclarationPaths(dirs []string) ([]string, error) {
	var paths []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, fmt.Errorf("scan tool config dir %s: %w", dir, err)
		}
		var dirPaths []string
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
				continue
			}
			dirPaths = append(dirPaths, filepath.Join(dir, e.Name()))
		}
		sort.Strings(dirPaths)
		paths = append(paths, dirPaths...)
	}
	return paths, nil
}

// SelectTools filters declarations to selected names.
func SelectTools(declarations []ToolDef, selection []string) ([]ToolDef, error) {
	index := make(map[string]ToolDef, len(declarations))
	for _, d := range declarations {
		index[d.Name] = d
	}
	var result []ToolDef
	for _, name := range selection {
		d, ok := index[name]
		if !ok {
			return nil, fmt.Errorf("tool %q is selected but not declared", name)
		}
		result = append(result, d)
	}
	return result, nil
}

// LoadToolDefs reads one declaration file and resolves includes.
func LoadToolDefs(path string) ([]ToolDef, error) {
	return productionToolImportResolver(nil).loadRoots([]string{path})
}

func readToolDefsFile(
	path string, options LoadOptions, visit FileVisitor,
) (ToolDefsFile, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ToolDefsFile{}, nil, fmt.Errorf("load tool defs %s: %w", path, err)
	}
	if visit != nil {
		if err := visit(path, data); err != nil {
			return ToolDefsFile{}, nil, fmt.Errorf("visit tool defs %s: %w", path, err)
		}
	}
	// Expanded before parsing, by the same rules the REST definition loader
	// applies, so an address that differs between a local run and a deployment
	// is an environment reference rather than a literal the deployment cannot
	// reach (srd013 R5.6). The expanded bytes are returned as well: a fragment
	// is expanded from them, not from the decoded file (srd052 R2.4).
	expanded := data
	if options.ExpandEnv {
		expanded = envexpand.Expand(data)
	}
	file, err := parseToolDefsFileRaw(expanded, false, options.TolerateNonToolFiles)
	if err != nil {
		return ToolDefsFile{}, nil, fmt.Errorf("parse tool defs %s: %w", path, err)
	}
	return file, expanded, nil
}

// ParseToolDefs parses YAML bytes into tool definitions without resolving includes.
func ParseToolDefs(data []byte) ([]ToolDef, error) {
	file, err := parseToolDefsFileRaw(data, true, false)
	if err != nil {
		return nil, fmt.Errorf("parse tool defs: %w", err)
	}
	return file.Tools, validateAndDefaultToolDefs(file.Tools)
}

func parseToolDefsFileRaw(
	data []byte, expandEnv, tolerateNonTool bool,
) (ToolDefsFile, error) {
	expanded := data
	if expandEnv {
		expanded = envexpand.Expand(data)
	}
	root, err := toolDocumentRoot(expanded)
	if err != nil {
		return ToolDefsFile{}, err
	}
	hasTools := yamlstrict.FieldPresent(root, "tools")
	hasTypes := yamlstrict.FieldPresent(root, "types")
	hasParams := yamlstrict.FieldPresent(root, "params")
	if !hasTools && !hasTypes && !hasParams && tolerateNonTool {
		return ToolDefsFile{}, nil
	}
	file, err := decodeToolDefsFile(expanded, hasParams)
	if err != nil {
		return ToolDefsFile{}, err
	}
	file.hasTools = hasTools
	file.hasTypes = hasTypes
	file.hasImports = yamlstrict.FieldPresent(root, "imports")
	file.hasParams = hasParams
	file.hasExpand = yamlstrict.FieldPresent(root, "expand")
	// A unit is a fragment or it is not; half of one declares nothing
	// (srd052 R1.3).
	if file.hasParams && !file.hasTools && !file.hasTypes {
		return ToolDefsFile{}, fmt.Errorf("fragment declares params but no tools or types body")
	}
	if !file.hasTools && !file.hasTypes {
		return ToolDefsFile{}, fmt.Errorf("top-level tools field is required")
	}
	if !file.hasParams && fragments.References(expanded) {
		return ToolDefsFile{}, fmt.Errorf("references $param but declares no params")
	}
	return file, nil
}

// decodeToolDefsFile decodes a file strictly. A fragment's body is left
// undecoded: it holds $param references where typed fields stand, and it is
// only a declaration once its arguments arrive (srd052 R2.4). Its header is
// still checked strictly, and an unknown top-level field is still rejected.
func decodeToolDefsFile(expanded []byte, fragment bool) (ToolDefsFile, error) {
	if !fragment {
		var file ToolDefsFile
		return file, yamlstrict.Unmarshal(expanded, &file)
	}
	var header struct {
		Unit    string                `yaml:"unit,omitempty"`
		Imports []string              `yaml:"imports,omitempty"`
		Params  []fragments.Param     `yaml:"params,omitempty"`
		Expand  []fragments.Expansion `yaml:"expand,omitempty"`
		Tools   yaml.Node             `yaml:"tools,omitempty"`
		Types   yaml.Node             `yaml:"types,omitempty"`
	}
	if err := yamlstrict.Unmarshal(expanded, &header); err != nil {
		return ToolDefsFile{}, err
	}
	return ToolDefsFile{
		Unit: header.Unit, Imports: header.Imports,
		Params: header.Params, Expand: header.Expand,
	}, nil
}

func toolDocumentRoot(data []byte) (*yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, err
	}
	if document.Kind == yaml.DocumentNode && len(document.Content) > 0 {
		return document.Content[0], nil
	}
	return &document, nil
}

// validateAndDefaultToolDefs checks each declaration and then fills the
// contract blocks its signature discharges, so every load path yields the
// effective contract rather than only the paths that also resolve schemas
// (srd051 R6.6 to R6.11).
func validateAndDefaultToolDefs(defs []ToolDef) error {
	for i, td := range defs {
		if td.Name == "" {
			return fmt.Errorf("tool at index %d has no name", i)
		}
		switch td.Type {
		case "builtin":
			if td.Init == "" {
				return fmt.Errorf("builtin tool %q has no init field", td.Name)
			}
		case "exec", "":
			if td.Binary == "" {
				return fmt.Errorf("tool %q has no binary", td.Name)
			}
		default:
			return fmt.Errorf("tool %q: unknown type %q", td.Name, td.Type)
		}
		if err := validateToolVocabulary(td); err != nil {
			return err
		}
		if err := validateToolSignature(td); err != nil {
			return err
		}
		if !validPreconditions[td.Precondition] {
			return fmt.Errorf("tool %q: unknown precondition %q", td.Name, td.Precondition)
		}
		if err := validateUndoStrategy(td); err != nil {
			return err
		}
		if err := core.ValidateMetricConfig(td.Name, td.Metrics); err != nil {
			return fmt.Errorf("tool %q: %w", td.Name, err)
		}
		defs[i] = applyContractDefaults(td)
	}
	return nil
}

func validateToolVocabulary(def ToolDef) error {
	switch def.Visibility {
	case "", "internal", "external":
	default:
		return fmt.Errorf("tool %q: unknown visibility %q", def.Name, def.Visibility)
	}
	switch def.Reversibility.Classification {
	case "", "reversible", "compensatable", "irreversible":
	default:
		return fmt.Errorf(
			"tool %q: unknown reversibility classification %q",
			def.Name, def.Reversibility.Classification,
		)
	}
	return nil
}

func validateUndoStrategy(def ToolDef) error {
	strategy := def.Undo.Strategy
	if strategy == "" {
		return nil
	}
	if !core.KnownUndoStrategy(strategy) {
		return fmt.Errorf("tool %q: unknown undo strategy %q", def.Name, strategy)
	}
	if !core.UndoStrategySupported(def.Type, strategy) {
		return fmt.Errorf(
			"tool %q: undo strategy %q is not supported for type %q; supported: %s",
			def.Name, strategy, def.Type,
			strings.Join(core.SupportedUndoStrategies(def.Type), ", "),
		)
	}
	return nil
}

// validPreconditions enumerates the precondition gates an exec tool may
// declare; the exec builder interprets each one before launch. Load rejects
// anything else so a typo like "git-repo" fails at load instead of silently
// falling through to the git check at dispatch (GH-1381).
var validPreconditions = map[string]bool{
	"":         true,
	"git_repo": true,
}
