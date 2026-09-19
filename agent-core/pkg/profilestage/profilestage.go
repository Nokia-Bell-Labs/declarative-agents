// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package profilestage copies declaration trees into a staged profile tree and
// proves the copy resolves.
//
// A stager that copies one agent directory copies less than the profile loads.
// A declaration imports its type units and its REST units by a path relative to
// the declaring file (srd050 R1.2), and those targets sit beside the agent
// directory rather than inside it, so the copy arrives without them and the
// agent fails at startup with an unresolvable import. Six stagers have needed
// the same hand-written patch (GH-2031). This package follows the import edges
// instead, so a stager states the directories it copies and nothing else.
package profilestage

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/profileaudit"
)

// Tree is one directory a stager copies, named by its source and the place it
// takes in the staged tree. The two differ wherever a stager re-roots, and the
// destination is what a staged import path resolves against.
type Tree struct {
	Source      string
	Destination string
}

// Stage copies each tree, then every file the copied declarations import from
// outside them. An import target is placed where the staged declaration's own
// relative path resolves, so a re-rooted tree carries its imports to the
// position it re-rooted them to. Resolution is transitive: a unit importing
// another unit brings that one too.
//
// root is the directory the stager owns, and everything Stage writes must land
// inside it. Imports are placed outside the tree that declares them by design
// -- a unit beside an agent directory lands beside its copy -- so a tree's
// destination is not the boundary. The directory the stager mounts, packages,
// and removes is, and only the stager knows which directory that is (GH-2075).
func Stage(root string, trees ...Tree) error {
	closure := &stagedClosure{root: filepath.Clean(root), staged: map[string]bool{}}
	for _, tree := range trees {
		if !within(closure.root, filepath.Clean(tree.Destination)) {
			return fmt.Errorf("stage %s: destination %s is outside the staged root %s",
				tree.Source, tree.Destination, closure.root)
		}
		if err := closure.copyTree(tree); err != nil {
			return err
		}
	}
	return closure.followImports()
}

// Validate proves a staged profile resolves its whole closure. It reports only
// the load failure: the timeout policy profileaudit also checks belongs to the
// audit that owns it, not to whether a staged tree is complete.
func Validate(profilePath, coreRoot string) error {
	if _, err := profileaudit.InspectWithOptions(
		profilePath, profileaudit.Options{CoreRoot: coreRoot},
	); err != nil {
		return fmt.Errorf("staged profile %s does not resolve: %w", profilePath, err)
	}
	return nil
}

// Imported returns every file the declarations under root import from outside
// it, transitively and in a stable order. A caller that walks a directory --
// to copy it, or to hash it -- uses this to reach what those declarations
// depend on and the directory does not contain.
func Imported(root string) ([]string, error) {
	root = filepath.Clean(root)
	walk := importWalk{root: root, seen: map[string]bool{}}
	pending, err := declarationsUnder(root, walk.seen)
	if err != nil {
		return nil, err
	}
	walk.libraries = declaredRootsOf(pending)
	walk.pending = pending
	for len(walk.pending) > 0 {
		file := walk.pending[0]
		walk.pending = walk.pending[1:]
		imports, err := declaredImports(file)
		if err != nil {
			return nil, err
		}
		for _, edge := range imports {
			variants, err := variantEdges(edge, filepath.Dir(file), func(root string) (string, bool) {
				directory, declared := walk.libraries[root]
				return directory, declared
			})
			if err != nil {
				return nil, fmt.Errorf("import of %s: %w", file, err)
			}
			for _, imported := range variants {
				if err := walk.follow(file, imported); err != nil {
					return nil, err
				}
			}
		}
	}
	sort.Strings(walk.outside)
	return walk.outside, nil
}

// importWalk is the state of one Imported traversal.
type importWalk struct {
	root      string
	seen      map[string]bool
	libraries map[string]string
	pending   []string
	outside   []string
}

func (w *importWalk) follow(file, imported string) error {
	target, ok := importedTarget(file, imported, w.libraries)
	if !ok || w.seen[target] {
		return nil
	}
	w.seen[target] = true
	if _, err := os.Stat(target); err != nil {
		return fmt.Errorf("import %q of %s: %w", imported, file, err)
	}
	if !within(w.root, target) {
		w.outside = append(w.outside, target)
	}
	w.pending = append(w.pending, target)
	return nil
}

// declaredRootsOf collects the library roots the profiles among files declare,
// each directory resolved against its profile.
func declaredRootsOf(files []string) map[string]string {
	libraries := map[string]string{}
	for _, file := range files {
		for name, directory := range declaredLibraries(file) {
			libraries[name] = filepath.Clean(filepath.Join(filepath.Dir(file), directory))
		}
	}
	return libraries
}

// importedTarget resolves an edge for Imported: relative to the file, under a
// declared root's directory, or not followed at all for agent-core's library,
// which the image supplies, and for an undeclared root, which the loader
// reports.
func importedTarget(file, imported string, libraries map[string]string) (string, bool) {
	if !filepath.IsAbs(imported) {
		return filepath.Clean(filepath.Join(filepath.Dir(file), imported)), true
	}
	root, rest, ok := libraryReference(imported)
	directory, declared := libraries[root]
	if isAgentCoreLibraryPath(imported) || !ok || !declared {
		return "", false
	}
	return filepath.Join(directory, rest), true
}

func declarationsUnder(root string, seen map[string]bool) ([]string, error) {
	var found []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !isYAML(path) {
			return nil
		}
		path = filepath.Clean(path)
		seen[path] = true
		found = append(found, path)
		return nil
	})
	return found, err
}

func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

// stagedClosure records where each staged file came from, because an import is
// read from the source beside the declaring file and written beside its copy.
type stagedClosure struct {
	root    string
	staged  map[string]bool
	pending []stagedFile
	// libraries maps a declared root name to where its directory sits in the
	// source and in the staged tree, learned from each staged profile's
	// libraries field (srd056 R3.3).
	libraries map[string]stagedFile
}

type stagedFile struct{ source, destination string }

func (c *stagedClosure) copyTree(tree Tree) error {
	return filepath.WalkDir(tree.Source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("stage %s: %w", tree.Source, walkErr)
		}
		relative, err := filepath.Rel(tree.Source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(tree.Destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("stage %s: %s is not a regular file", tree.Source, path)
		}
		if err := copyFile(path, destination); err != nil {
			return err
		}
		c.record(path, destination)
		return nil
	})
}

func (c *stagedClosure) record(source, destination string) {
	c.staged[filepath.Clean(destination)] = true
	if isYAML(destination) {
		c.pending = append(c.pending, stagedFile{source: source, destination: destination})
		c.recordLibraries(source, destination)
	}
}

// recordLibraries learns the roots a staged profile declares. A root's
// directory is a path relative to the profile, so it re-roots exactly as a
// relative import does: the staged directory is where the copied profile's
// relative path resolves.
func (c *stagedClosure) recordLibraries(source, destination string) {
	for name, directory := range declaredLibraries(source) {
		if c.libraries == nil {
			c.libraries = map[string]stagedFile{}
		}
		c.libraries[name] = stagedFile{
			source:      filepath.Clean(filepath.Join(filepath.Dir(source), directory)),
			destination: filepath.Clean(filepath.Join(filepath.Dir(destination), directory)),
		}
	}
}

func declaredLibraries(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var profile struct {
		Libraries map[string]string `yaml:"libraries"`
	}
	if yaml.Unmarshal(data, &profile) != nil {
		return nil
	}
	return profile.Libraries
}

func (c *stagedClosure) followImports() error {
	for len(c.pending) > 0 {
		file := c.pending[0]
		c.pending = c.pending[1:]
		imports, err := declaredImports(file.destination)
		if err != nil {
			return err
		}
		for _, edge := range imports {
			variants, err := variantEdges(edge, filepath.Dir(file.source), func(root string) (string, bool) {
				library, declared := c.libraries[root]
				return library.source, declared
			})
			if err != nil {
				return fmt.Errorf("stage import of %s: %w", file.source, err)
			}
			for _, imported := range variants {
				if err := c.stageImport(file, imported); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (c *stagedClosure) stageImport(file stagedFile, imported string) error {
	source := filepath.Clean(filepath.Join(filepath.Dir(file.source), imported))
	destination := filepath.Clean(filepath.Join(filepath.Dir(file.destination), imported))
	if filepath.IsAbs(imported) {
		if isAgentCoreLibraryPath(imported) {
			// The runtime image supplies agent-core's library (srd056 R3.3).
			return nil
		}
		root, rest, ok := libraryReference(imported)
		library, declared := c.libraries[root]
		if !ok || !declared {
			return fmt.Errorf("stage import %q of %s: library root %q is declared by no staged profile",
				imported, file.source, root)
		}
		source = filepath.Join(library.source, rest)
		destination = filepath.Join(library.destination, rest)
	}
	if !within(c.root, destination) {
		return fmt.Errorf(
			"stage import %q of %s: resolves to %s, outside the staged root %s",
			imported, file.source, destination, c.root,
		)
	}
	if c.staged[destination] {
		return nil
	}
	if _, err := os.Stat(source); err != nil {
		return fmt.Errorf("stage import %q of %s: %w", imported, file.source, err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := copyFile(source, destination); err != nil {
		return err
	}
	c.record(source, destination)
	return nil
}

// declaredImports reads the edges a declaration file follows to other files:
// the import list srd050 R1.3 places at the top level of a tool, REST, or type
// unit, and the fragments its instantiate list applies (srd052 R2.1), which
// includes a machine's stage fragments. An instantiation is an import edge
// whose unit is filled in on the way, so a stager that copied imports and not
// fragments would stage a tree that fails at startup the same way. A file that
// is not a declaration decodes to nothing and imports nothing.
func declaredImports(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read staged declaration %s: %w", path, err)
	}
	type instantiations []struct {
		Fragment string `yaml:"fragment"`
	}
	var file struct {
		Imports     []string       `yaml:"imports"`
		Instantiate instantiations `yaml:"instantiate"`
		// A machine template's body instantiates its stages (srd054 R2.2).
		Machine struct {
			Instantiate instantiations `yaml:"instantiate"`
		} `yaml:"machine"`
	}
	if yaml.Unmarshal(data, &file) != nil {
		return nil, nil
	}
	var edges []string
	for _, edge := range file.Imports {
		edges = appendStagedEdge(edges, edge)
	}
	for _, instantiation := range append(file.Instantiate, file.Machine.Instantiate...) {
		edges = appendStagedEdge(edges, instantiation.Fragment)
	}
	return edges, nil
}

// appendStagedEdge keeps the edges a stager follows: every non-empty import
// and fragment path, relative or library-rooted.
func appendStagedEdge(edges []string, edge string) []string {
	if edge == "" {
		return edges
	}
	return append(edges, edge)
}

// libraryPrefix is the directory every library root sits under as written,
// and agentCoreRoot the implicit root the runtime image installs (srd056).
const (
	libraryPrefix = "/opt"
	agentCoreRoot = "agent-core"
)

func isAgentCoreLibraryPath(path string) bool {
	root, _, ok := libraryReference(path)
	return ok && root == agentCoreRoot
}

// libraryReference splits /opt/<root>/<rest>.
func libraryReference(path string) (root, rest string, ok bool) {
	clean := filepath.ToSlash(filepath.Clean(path))
	if !strings.HasPrefix(clean, libraryPrefix+"/") {
		return "", "", false
	}
	root, rest, _ = strings.Cut(strings.TrimPrefix(clean, libraryPrefix+"/"), "/")
	return root, filepath.FromSlash(rest), root != ""
}

func copyFile(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read %s: %w", source, err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("stat %s: %w", source, err)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destination, data, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write %s: %w", destination, err)
	}
	return nil
}

func isYAML(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return true
	}
	return false
}
