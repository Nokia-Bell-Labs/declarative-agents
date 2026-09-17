// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package definition

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/fragments"
)

// Instantiating REST-definition fragments (srd052 R2). The fragment's expanded
// bytes are filled, its produced names are prefixed when the instantiation
// says so, and the result is parsed and merged like a hand-written unit at the
// fragment's own path.

// DeclarationInstantiation is one application of a REST fragment: where it
// came from, what filled it, and what it produced as family/name (srd052 R3.2).
type DeclarationInstantiation struct {
	Fragment string
	As       string
	Args     map[string]string
	Produces []string
}

// DeclarationInstantiations returns every instantiation in traversal order.
func (d Definition) DeclarationInstantiations() []DeclarationInstantiation {
	return append([]DeclarationInstantiation(nil), d.instantiations...)
}

// restFamilies are the named-entry families a REST body declares, with the
// reference fields that point into each, so a prefix renames a produced entry
// and every reference the fragment itself makes to it (srd052 R2.5).
var restFamilies = []struct {
	family string
	refs   []string
}{
	{"clients", nil},
	{"servers", nil},
	{"openapi", nil},
	{"auth", []string{"auth_ref", "require_auth_ref"}},
	{"limits", []string{"limits_ref"}},
	{"retry_policies", []string{"retry_ref"}},
	{"response_mappings", []string{"response_ref"}},
	{"document_resources", []string{"document_resources"}},
	// Nested names, renamed by prefixNestedNames; bind maps an OpenAPI
	// operation to an endpoint name.
	{"endpoints", []string{"bind"}},
	{"operations", nil},
}

func validateFragmentUnit(file DefinitionFile, path string, imported bool) error {
	if !file.IsFragment() {
		return nil
	}
	if imported {
		return fmt.Errorf("REST fragment %s is instantiated, not imported (srd052 R2.6)", path)
	}
	if file.hasInstantiate {
		return fmt.Errorf("REST fragment %s instantiates another fragment; nesting is not supported (srd052 R1.2)", path)
	}
	if file.Unit == "" {
		return fmt.Errorf("REST fragment %s must declare unit", path)
	}
	if err := fragments.ValidateParams(file.Params); err != nil {
		return fmt.Errorf("REST fragment %s: %w", path, err)
	}
	return nil
}

func (r *importResolver) loadInstantiations(file DefinitionFile, path string) error {
	for _, instantiation := range file.Instantiate {
		if err := r.instantiate(path, instantiation); err != nil {
			return fmt.Errorf("REST unit %q at %s instantiates %q: %w",
				file.Unit, path, instantiation.Fragment, err)
		}
	}
	return nil
}

func (r *importResolver) instantiate(path string, instantiation fragments.Instantiation) error {
	if strings.TrimSpace(instantiation.Fragment) == "" {
		return fmt.Errorf("fragment path must be non-empty")
	}
	target, err := declarationImportTarget(path, instantiation.Fragment)
	if err != nil {
		return fmt.Errorf("fragment path %q: %w", instantiation.Fragment, err)
	}
	fragment, source, err := r.readUnit(target, false)
	if err != nil {
		return err
	}
	if !fragment.IsFragment() {
		return fmt.Errorf("%s declares no params, so it is imported, not instantiated", target)
	}
	args, err := fragments.ResolveArgs(fragment.Params, instantiation.Args)
	if err != nil {
		return fmt.Errorf("fragment %s: %w", target, err)
	}
	instantiated, err := r.substitute(target, args, instantiation.As)
	if err != nil {
		return err
	}
	if err := r.loadFragmentImports(instantiated, target); err != nil {
		return err
	}
	values := make(map[string]string, len(args))
	for name, arg := range args {
		values[name] = arg.Value
	}
	r.order = append(r.order, declarationUnit{source: source, rest: instantiated.Rest})
	r.imports = append(r.imports, DeclarationImport{Importer: r.sources[path], Imported: source, Args: values})
	r.instantiations = append(r.instantiations, DeclarationInstantiation{
		Fragment: target, As: instantiation.As, Args: values, Produces: producedNames(instantiated.Rest),
	})
	return nil
}

// loadFragmentImports resolves the fragment's own imports from the fragment's
// path, with the fragment on the stack so a cycle through it is reported.
func (r *importResolver) loadFragmentImports(instantiated DefinitionFile, target string) error {
	r.visiting[target] = len(r.stack)
	r.stack = append(r.stack, target)
	err := r.loadImports(instantiated, target)
	r.stack = r.stack[:len(r.stack)-1]
	delete(r.visiting, target)
	return err
}

// substitute fills the fragment's expanded bytes, applies the prefix, and
// parses the result the way a hand-written file is parsed (srd052 R2.3, R2.4).
func (r *importResolver) substitute(
	target string, args map[string]fragments.Arg, prefix string,
) (DefinitionFile, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(r.raw[target], &document); err != nil {
		return DefinitionFile{}, fmt.Errorf("fragment %s: %w", target, err)
	}
	if err := fragments.Substitute(&document, args); err != nil {
		return DefinitionFile{}, fmt.Errorf("fragment %s: %w", target, err)
	}
	if line, found := fragments.Leftover(&document); found {
		return DefinitionFile{}, fmt.Errorf("fragment %s: line %d: $param( survives substitution", target, line)
	}
	fragments.RemoveField(&document, "params")
	if prefix != "" {
		prefixRESTNames(&document, prefix)
	}
	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return DefinitionFile{}, fmt.Errorf("fragment %s: encode instantiation: %w", target, err)
	}
	instantiated, err := parseDefinitionFileExpanded(output.Bytes())
	if err != nil {
		return DefinitionFile{}, fmt.Errorf("fragment %s: %w", target, err)
	}
	return instantiated, nil
}

// prefixRESTNames renames every name the fragment's rest body produces to
// <prefix>_<name>, and rewrites the fragment's own references to them, so one
// fragment can be instantiated more than once and each result still resolves
// (srd052 R2.5). Produced names are the family entries and, because the
// closure holds them unique across every client and server, the operation
// and endpoint names nested under clients and servers. References to names
// the fragment does not declare are left for the importer's closure to supply.
func prefixRESTNames(document *yaml.Node, prefix string) {
	rest := mappingEntry(documentRoot(document), "rest")
	if rest == nil {
		return
	}
	renamed := map[string]map[string]bool{}
	for _, family := range restFamilies {
		entries := mappingEntry(rest, family.family)
		if entries == nil || entries.Kind != yaml.MappingNode {
			continue
		}
		renamed[family.family] = map[string]bool{}
		for index := 0; index+1 < len(entries.Content); index += 2 {
			name := entries.Content[index]
			renamed[family.family][name.Value] = true
			name.Value = prefix + "_" + name.Value
		}
	}
	renamed["endpoints"] = prefixNestedNames(mappingEntry(rest, "servers"), prefix, "endpoints")
	renamed["operations"] = prefixNestedNames(mappingEntry(rest, "clients"), prefix, "operations")
	for _, client := range mappingValues(mappingEntry(rest, "clients")) {
		for name := range prefixNestedNames(mappingEntry(client, "resources"), prefix, "operations") {
			renamed["operations"][name] = true
		}
	}
	rewriteRESTReferences(rest, prefix, renamed)
}

// prefixNestedNames renames the keys of the named mapping under every entry
// of parent -- the endpoints of every server, the operations of every client
// -- and reports the names it renamed.
func prefixNestedNames(parent *yaml.Node, prefix, nested string) map[string]bool {
	renamed := map[string]bool{}
	for _, entry := range mappingValues(parent) {
		names := mappingEntry(entry, nested)
		if names == nil || names.Kind != yaml.MappingNode {
			continue
		}
		for index := 0; index+1 < len(names.Content); index += 2 {
			name := names.Content[index]
			renamed[name.Value] = true
			name.Value = prefix + "_" + name.Value
		}
	}
	return renamed
}

func mappingValues(mapping *yaml.Node) []*yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	values := make([]*yaml.Node, 0, len(mapping.Content)/2)
	for index := 1; index < len(mapping.Content); index += 2 {
		values = append(values, mapping.Content[index])
	}
	return values
}

func rewriteRESTReferences(node *yaml.Node, prefix string, renamed map[string]map[string]bool) {
	if node == nil {
		return
	}
	if node.Kind == yaml.MappingNode {
		for index := 0; index+1 < len(node.Content); index += 2 {
			key, value := node.Content[index], node.Content[index+1]
			if family := referenceFamily(key.Value); family != "" {
				rewriteReference(value, prefix, renamed[family])
			}
			rewriteRESTReferences(value, prefix, renamed)
		}
		return
	}
	for _, child := range node.Content {
		rewriteRESTReferences(child, prefix, renamed)
	}
}

func rewriteReference(value *yaml.Node, prefix string, names map[string]bool) {
	switch value.Kind {
	case yaml.ScalarNode:
		if names[value.Value] {
			value.Value = prefix + "_" + value.Value
		}
	case yaml.SequenceNode:
		for _, item := range value.Content {
			rewriteReference(item, prefix, names)
		}
	case yaml.MappingNode:
		for _, item := range mappingValues(value) {
			rewriteReference(item, prefix, names)
		}
	}
}

func referenceFamily(key string) string {
	for _, family := range restFamilies {
		for _, ref := range family.refs {
			if ref == key {
				return family.family
			}
		}
	}
	return ""
}

func documentRoot(document *yaml.Node) *yaml.Node {
	if document != nil && document.Kind == yaml.DocumentNode && len(document.Content) > 0 {
		return document.Content[0]
	}
	return document
}

func mappingEntry(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return mapping.Content[index+1]
		}
	}
	return nil
}

func producedNames(def Definition) []string {
	var names []string
	for _, family := range restMapFamilies(&Definition{}, &def) {
		for _, name := range family.names {
			names = append(names, family.name+"/"+name)
		}
	}
	sort.Strings(names)
	return names
}
