// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package declstyle

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// classSingleImporterUnit marks a unit reached by exactly one import edge and
// never instantiated. Such a unit is an include by another name: it adds a
// file, a unit header, and an edge, and removes no duplication, so the fragment
// rule folds it back into its importer (GH-2079, GH-2107).
//
// Two kinds of unit are outside the class. A type-only unit cannot fold back,
// because a declaration may not carry both tools and types. agent-core's
// library under tools/ is published for importers outside this repository, so
// an in-repo count does not measure it. The class had no entries when it
// landed, so any new single-importer unit fails.
const classSingleImporterUnit = "single-importer-unit"

// agentCoreLibraryPrefix is the install path of agent-core's library; in a
// checkout it resolves under the agent-core module (srd056 R1.1).
const agentCoreLibraryPrefix = "/opt/agent-core/"

// environmentPlaceholder matches a ${NAME:-default} reference the loader expands
// before parsing. Unexpanded, one inside a flow sequence is not valid YAML.
var environmentPlaceholder = regexp.MustCompile(`\$\{[^}]*\}`)

// placeholderSafe replaces each placeholder with a plain scalar so a declaration
// that is valid after expansion parses here too.
func placeholderSafe(data []byte) []byte {
	return environmentPlaceholder.ReplaceAll(data, []byte("placeholder"))
}

// unitGraph is the import graph over the declaration roots, built from YAML
// nodes rather than the catalog loader so the gate stays test-only.
type unitGraph struct {
	units        map[string]bool
	importers    map[string]map[string]bool
	instantiated map[string]bool
	profileRoots map[string]bool
}

func newUnitGraph() *unitGraph {
	return &unitGraph{
		units: map[string]bool{}, importers: map[string]map[string]bool{},
		instantiated: map[string]bool{}, profileRoots: map[string]bool{},
	}
}

// singleImporterEntries classifies every declaration file under roots.
// coreModule maps /opt/agent-core/ references; repo makes paths relative.
func singleImporterEntries(paths []string, coreModule, repo string) ([]string, error) {
	graph := newUnitGraph()
	for _, path := range paths {
		if err := graph.addFile(path, coreModule); err != nil {
			return nil, err
		}
	}
	var entries []string
	library := filepath.Join(coreModule, "tools") + string(filepath.Separator)
	for unit := range graph.units {
		if len(graph.importers[unit]) != 1 || graph.instantiated[unit] || graph.profileRoots[unit] {
			continue
		}
		// agent-core's library is published for importers outside this
		// repository, so its in-repo importer count does not measure it
		// (srd056); its index files import each word once by design.
		if strings.HasPrefix(unit, library) {
			continue
		}
		rel, err := filepath.Rel(repo, unit)
		if err != nil {
			return nil, err
		}
		entries = append(entries, fmt.Sprintf("%s:%s", classSingleImporterUnit, filepath.ToSlash(rel)))
	}
	sort.Strings(entries)
	return entries, nil
}

func (g *unitGraph) addFile(path, coreModule string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document yaml.Node
	// A file that does not parse is not a declaration this graph can read.
	if yaml.Unmarshal(placeholderSafe(data), &document) != nil || len(document.Content) == 0 {
		return nil
	}
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil
	}
	resolve := func(reference string) (string, bool) { return resolveUnitReference(path, reference, coreModule) }
	// A type-only unit cannot fold back: the loader rejects a declaration that
	// carries both tools and types, so the one importer has nowhere to take it.
	if value := topValue(root, "unit"); value != nil && value.Kind == yaml.ScalarNode && !typeOnlyUnit(root) {
		g.units[filepath.Clean(path)] = true
	}
	for _, reference := range scalarList(topValue(root, "imports")) {
		if target, ok := resolve(reference); ok {
			g.addImporter(target, path)
		}
	}
	g.markInstantiated(topValue(root, "expand"), resolve)
	if machine := topValue(root, "machine"); machine != nil && machine.Kind == yaml.MappingNode {
		g.markInstantiated(topValue(machine, "expand"), resolve)
	}
	g.markProfileRoots(root, resolve)
	return nil
}

func typeOnlyUnit(root *yaml.Node) bool {
	return topValue(root, "types") != nil && topValue(root, "tools") == nil
}

func (g *unitGraph) addImporter(unit, importer string) {
	if g.importers[unit] == nil {
		g.importers[unit] = map[string]bool{}
	}
	g.importers[unit][filepath.Clean(importer)] = true
}

func (g *unitGraph) markInstantiated(instantiate *yaml.Node, resolve func(string) (string, bool)) {
	if instantiate == nil || instantiate.Kind != yaml.SequenceNode {
		return
	}
	for _, entry := range instantiate.Content {
		if fragment := topValue(entry, "fragment"); fragment != nil && fragment.Kind == yaml.ScalarNode {
			if target, ok := resolve(fragment.Value); ok {
				g.instantiated[target] = true
			}
		}
	}
}

// markProfileRoots records the declaration files a profile lists directly.
// A root is loaded by the profile, not imported, so it is never an include.
func (g *unitGraph) markProfileRoots(root *yaml.Node, resolve func(string) (string, bool)) {
	for _, field := range []string{"tool_declarations", "tools", "rest_definitions"} {
		for _, reference := range scalarList(topValue(root, field)) {
			if target, ok := resolve(reference); ok {
				g.profileRoots[target] = true
			}
		}
	}
}

// resolveUnitReference resolves an import relative to the importing file and
// agent-core library paths under the agent-core module. Other library roots
// resolve per profile and are not followed here.
func resolveUnitReference(importer, reference, coreModule string) (string, bool) {
	reference = strings.TrimSpace(reference)
	switch {
	case reference == "":
		return "", false
	case strings.HasPrefix(reference, agentCoreLibraryPrefix):
		return filepath.Join(coreModule, filepath.FromSlash(strings.TrimPrefix(reference, agentCoreLibraryPrefix))), true
	case filepath.IsAbs(reference):
		return "", false
	default:
		return filepath.Clean(filepath.Join(filepath.Dir(importer), filepath.FromSlash(reference))), true
	}
}

func topValue(mapping *yaml.Node, key string) *yaml.Node {
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

func scalarList(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.SequenceNode {
		return nil
	}
	var values []string
	for _, child := range node.Content {
		if child.Kind == yaml.ScalarNode {
			values = append(values, child.Value)
		}
	}
	return values
}

func TestSingleImporterUnitClassification(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write("units/shared.yaml", "unit: shared\ntools: []\n")
	write("units/lonely.yaml", "unit: lonely\ntools: []\n")
	write("units/fragment.yaml", "unit: fragment\nparams: []\ntools: []\n")
	write("units/root.yaml", "unit: root\ntools: []\n")
	write("units/unused.yaml", "unit: unused\ntools: []\n")
	write("core/tools/units/words.yaml", "unit: core-words\ntools: []\n")
	write("units/templated.yaml", "unit: templated\ntools: []\n")
	write("c/rest.yaml", "unit: c\nimports: [../units/templated.yaml, ../units/shared-rest.yaml]\nrest: {hosts: [${HOST:-127.0.0.1}, localhost]}\n")
	write("d/rest.yaml", "unit: d\nimports: [../units/shared-rest.yaml]\nrest: {}\n")
	write("units/shared-rest.yaml", "unit: shared-rest\nrest: {}\n")
	write("units/types.yaml", "unit: lonely-types\ntypes: []\n")
	write("a/declarations.yaml", "unit: a\nimports: [../units/shared.yaml, ../units/lonely.yaml, ../units/root.yaml]\ntools: []\n")
	write("b/declarations.yaml", "unit: b\nimports: [../units/shared.yaml, /opt/agent-core/tools/units/words.yaml, ../units/types.yaml]\n"+
		"expand: [{fragment: ../units/fragment.yaml, args: {}}]\ntools: []\n")
	write("b/profile.yaml", "name: b\nmachine: machine.yaml\ntool_declarations: [declarations.yaml, ../units/root.yaml]\n")

	paths, err := discoverLegacyDeclarationFiles([]string{root})
	require.NoError(t, err)
	entries, err := singleImporterEntries(paths, filepath.Join(root, "core"), root)
	require.NoError(t, err)
	// shared and shared-rest have two importers (one through a templated file),
	// fragment is instantiated, root is also a profile root, types holds only
	// types, the core library is exempt, and unused and the declaration files
	// have no importer: lonely and templated are the only single-importer units.
	require.Equal(t, []string{
		classSingleImporterUnit + ":units/lonely.yaml",
		classSingleImporterUnit + ":units/templated.yaml",
	}, entries)
}
