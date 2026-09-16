// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/support/corepath"
)

var declarationUnitName = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)

// DeclarationSource identifies one named REST declaration unit.
type DeclarationSource struct {
	Unit string
	Path string
}

type declarationSource = DeclarationSource

// DeclarationImport is one authored dependency between REST units. Args is
// set when the edge is an instantiation rather than an import (srd052 R3.1).
type DeclarationImport struct {
	Importer DeclarationSource
	Imported DeclarationSource
	Args     map[string]string
}

type declarationUnit struct {
	source declarationSource
	rest   Definition
}

type importResolver struct {
	visit          FileVisitor
	loaded         map[string]bool
	files          map[string]DefinitionFile
	raw            map[string][]byte
	visiting       map[string]int
	units          map[string]declarationSource
	sources        map[string]declarationSource
	stack          []string
	order          []declarationUnit
	imports        []DeclarationImport
	instantiations []DeclarationInstantiation
}

// LoadDefinitionClosure resolves and compiles REST roots and their imports as
// one declaration closure.
func LoadDefinitionClosure(paths []string, visit FileVisitor) (Definition, error) {
	resolver := &importResolver{
		visit: visit, loaded: map[string]bool{}, visiting: map[string]int{},
		files: map[string]DefinitionFile{}, raw: map[string][]byte{},
		units: map[string]declarationSource{}, sources: map[string]declarationSource{},
	}
	for _, path := range paths {
		if err := resolver.load(path, false); err != nil {
			return Definition{}, err
		}
	}
	merged, openAPISources, err := mergeDeclarationUnits(resolver.order)
	if err != nil {
		return Definition{}, err
	}
	if err := compileOpenAPIImportsFromSources(&merged, openAPISources, visit); err != nil {
		return Definition{}, err
	}
	merged.declarationImports = append([]DeclarationImport(nil), resolver.imports...)
	merged.instantiations = append([]DeclarationInstantiation(nil), resolver.instantiations...)
	return merged, nil
}

func (r *importResolver) load(path string, imported bool) error {
	path, err := canonicalDeclarationPath(path)
	if err != nil {
		return err
	}
	if index, ok := r.visiting[path]; ok {
		chain := append(append([]string(nil), r.stack[index:]...), path)
		return fmt.Errorf("REST import cycle: %s", strings.Join(chain, " -> "))
	}
	if r.loaded[path] {
		return nil
	}
	file, source, err := r.readUnit(path, imported)
	if err != nil {
		return err
	}
	r.visiting[path] = len(r.stack)
	r.stack = append(r.stack, path)
	if err := r.loadImports(file, path); err != nil {
		return err
	}
	if err := r.loadInstantiations(file, path); err != nil {
		return err
	}
	r.stack = r.stack[:len(r.stack)-1]
	delete(r.visiting, path)
	r.loaded[path] = true
	r.order = append(r.order, declarationUnit{source: source, rest: file.Rest})
	return nil
}

func (r *importResolver) readUnit(
	path string, imported bool,
) (DefinitionFile, declarationSource, error) {
	file, ok := r.files[path]
	if !ok {
		read, raw, err := readDefinitionFile(path, r.visit)
		if err != nil {
			return DefinitionFile{}, declarationSource{}, err
		}
		file = read
		r.files[path], r.raw[path] = file, raw
	}
	if err := validateDeclarationUnit(file, path, imported); err != nil {
		return DefinitionFile{}, declarationSource{}, err
	}
	source := declarationSource{Unit: file.Unit, Path: path}
	if previous, exists := r.units[file.Unit]; file.Unit != "" && exists && previous.Path != path {
		return DefinitionFile{}, declarationSource{}, fmt.Errorf(
			"duplicate REST unit %q: %s and %s", file.Unit, previous.Path, path,
		)
	}
	if file.Unit != "" {
		r.units[file.Unit] = source
	}
	r.sources[path] = source
	return file, source, nil
}

func (r *importResolver) loadImports(file DefinitionFile, path string) error {
	for _, importedPath := range file.Imports {
		target, err := declarationImportTarget(path, importedPath)
		if errors.Is(err, corepath.ErrOutsideLibraryRoot) {
			return fmt.Errorf("REST unit %q at %s imports absolute path %q: %w", file.Unit, path, importedPath, err)
		}
		if err != nil {
			return err
		}
		if err := r.load(target, true); err != nil {
			return fmt.Errorf("REST unit %q at %s imports %q: %w", file.Unit, path, importedPath, err)
		}
		r.imports = append(r.imports, DeclarationImport{
			Importer: r.sources[path],
			Imported: r.sources[target],
		})
	}
	return nil
}

// declarationImportTarget resolves an import or fragment path under srd056 R1:
// relative to the importing file, or under the agent-core library root.
func declarationImportTarget(importer, importPath string) (string, error) {
	target, err := corepath.ImportTarget(importer, importPath)
	if err != nil {
		return "", err
	}
	return canonicalDeclarationPath(target)
}

func canonicalDeclarationPath(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve REST declaration path %s: %w", path, err)
	}
	return filepath.Clean(absolute), nil
}

func validateDeclarationUnit(file DefinitionFile, path string, imported bool) error {
	if file.Unit != "" && !declarationUnitName.MatchString(file.Unit) {
		return fmt.Errorf("REST declaration %s has invalid unit %q", path, file.Unit)
	}
	if (imported || file.hasImports) && file.Unit == "" {
		return fmt.Errorf("REST declaration %s must declare unit", path)
	}
	for _, importedPath := range file.Imports {
		if strings.TrimSpace(importedPath) == "" {
			return fmt.Errorf("REST unit %q at %s has an empty import path", file.Unit, path)
		}
	}
	return validateFragmentUnit(file, path, imported)
}

func mergeDeclarationUnits(
	units []declarationUnit,
) (Definition, map[string]declarationSource, error) {
	merged := Definition{}
	versions := map[string]declarationSource{}
	sources := make(map[string]map[string]declarationSource)
	openAPISources := map[string]declarationSource{}
	for _, unit := range units {
		if err := mergeVersion(&merged, versions, unit); err != nil {
			return Definition{}, nil, err
		}
		for _, family := range restMapFamilies(&merged, &unit.rest) {
			if err := mergeRESTFamily(family, unit.source, sources); err != nil {
				return Definition{}, nil, err
			}
		}
		for name := range unit.rest.OpenAPI {
			openAPISources[name] = unit.source
		}
	}
	merged.declarationSources = exportDeclarationSources(sources)
	return merged, openAPISources, nil
}

func mergeVersion(merged *Definition, sources map[string]declarationSource, unit declarationUnit) error {
	version := unit.rest.Version
	if version == "" {
		return nil
	}
	if merged.Version != "" && merged.Version != version {
		previous := sources[merged.Version]
		return fmt.Errorf(
			"conflicting REST versions %q from %s and %q from %s",
			merged.Version, formatDeclarationSource(previous), version, formatDeclarationSource(unit.source),
		)
	}
	if merged.Version != "" {
		return nil
	}
	merged.Version = version
	sources[version] = unit.source
	return nil
}

type restMapFamily struct {
	name   string
	names  []string
	assign func(string)
}

func restMapFamilies(target, source *Definition) []restMapFamily {
	return append(primaryRESTMapFamilies(target, source), policyRESTMapFamilies(target, source)...)
}

func primaryRESTMapFamilies(target, source *Definition) []restMapFamily {
	return []restMapFamily{
		mapFamily("clients", source.Clients, func(name string) {
			if target.Clients == nil {
				target.Clients = map[string]Client{}
			}
			target.Clients[name] = source.Clients[name]
		}),
		mapFamily("servers", source.Servers, func(name string) {
			if target.Servers == nil {
				target.Servers = map[string]Server{}
			}
			target.Servers[name] = source.Servers[name]
		}),
		mapFamily("openapi", source.OpenAPI, func(name string) {
			if target.OpenAPI == nil {
				target.OpenAPI = map[string]OpenAPIImport{}
			}
			target.OpenAPI[name] = source.OpenAPI[name]
		}),
	}
}

func policyRESTMapFamilies(target, source *Definition) []restMapFamily {
	return []restMapFamily{
		mapFamily("auth", source.Auth, func(name string) {
			if target.Auth == nil {
				target.Auth = map[string]AuthProfile{}
			}
			target.Auth[name] = source.Auth[name]
		}),
		mapFamily("limits", source.Limits, func(name string) {
			if target.Limits == nil {
				target.Limits = map[string]LimitProfile{}
			}
			target.Limits[name] = source.Limits[name]
		}),
		mapFamily("retry_policies", source.RetryPolicies, func(name string) {
			if target.RetryPolicies == nil {
				target.RetryPolicies = map[string]RetryPolicy{}
			}
			target.RetryPolicies[name] = source.RetryPolicies[name]
		}),
		mapFamily("response_mappings", source.ResponseMappings, func(name string) {
			if target.ResponseMappings == nil {
				target.ResponseMappings = map[string]ResponseMapping{}
			}
			target.ResponseMappings[name] = source.ResponseMappings[name]
		}),
		mapFamily("document_resources", source.DocumentResources, func(name string) {
			if target.DocumentResources == nil {
				target.DocumentResources = map[string]DocumentResource{}
			}
			target.DocumentResources[name] = source.DocumentResources[name]
		}),
	}
}

func mapFamily[T any](name string, values map[string]T, assign func(string)) restMapFamily {
	names := make([]string, 0, len(values))
	for entry := range values {
		names = append(names, entry)
	}
	sort.Strings(names)
	return restMapFamily{name: name, names: names, assign: assign}
}

func mergeRESTFamily(
	family restMapFamily,
	source declarationSource,
	sources map[string]map[string]declarationSource,
) error {
	if sources[family.name] == nil {
		sources[family.name] = map[string]declarationSource{}
	}
	for _, name := range family.names {
		if previous, exists := sources[family.name][name]; exists {
			return fmt.Errorf(
				"duplicate REST %s %q: %s and %s",
				family.name, name, formatDeclarationSource(previous), formatDeclarationSource(source),
			)
		}
		family.assign(name)
		sources[family.name][name] = source
	}
	return nil
}

func formatDeclarationSource(source declarationSource) string {
	return fmt.Sprintf("unit %q at %s", source.Unit, source.Path)
}

func exportDeclarationSources(
	sources map[string]map[string]declarationSource,
) map[string]map[string]DeclarationSource {
	result := make(map[string]map[string]DeclarationSource, len(sources))
	for family, entries := range sources {
		result[family] = make(map[string]DeclarationSource, len(entries))
		for name, source := range entries {
			result[family][name] = source
		}
	}
	return result
}

// DeclarationImports returns the authored import edges in traversal order.
func (d Definition) DeclarationImports() []DeclarationImport {
	return append([]DeclarationImport(nil), d.declarationImports...)
}

// DeclarationSource returns the owner of one top-level REST declaration.
func (d Definition) DeclarationSource(family, name string) (DeclarationSource, bool) {
	source, ok := d.declarationSources[family][name]
	return source, ok
}

// OpenAPISourcesFor returns OpenAPI units compiled into one client or server.
func (d Definition) OpenAPISourcesFor(kind, name string) []DeclarationSource {
	consumer := kind + ":" + name
	var sources []DeclarationSource
	for openAPIName, consumers := range d.openAPIConsumers {
		if !consumers[consumer] {
			continue
		}
		if source, ok := d.DeclarationSource("openapi", openAPIName); ok {
			sources = append(sources, source)
		}
	}
	return sources
}

func recordOpenAPIConsumers(
	def *Definition,
	name string,
	operations map[string]openAPIOperation,
) {
	if def.openAPIConsumers == nil {
		def.openAPIConsumers = map[string]map[string]bool{}
	}
	consumers := map[string]bool{}
	for clientName, client := range def.Clients {
		for _, operation := range client.Operations {
			if _, ok := operations[operation.OpenAPIOperationID]; ok {
				consumers["client:"+clientName] = true
			}
		}
		for _, resource := range client.Resources {
			for _, operation := range resource.Operations {
				if _, ok := operations[operation.OpenAPIOperationID]; ok {
					consumers["client:"+clientName] = true
				}
			}
		}
	}
	for serverName, server := range def.Servers {
		for _, endpoint := range server.Endpoints {
			if _, ok := operations[endpoint.OpenAPIOperationID]; ok {
				consumers["server:"+serverName] = true
			}
		}
	}
	def.openAPIConsumers[name] = consumers
}
