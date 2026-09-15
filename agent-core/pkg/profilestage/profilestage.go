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
func Stage(trees ...Tree) error {
	closure := &stagedClosure{staged: map[string]bool{}}
	for _, tree := range trees {
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
	seen := map[string]bool{}
	pending, err := declarationsUnder(root, seen)
	if err != nil {
		return nil, err
	}
	var outside []string
	for len(pending) > 0 {
		file := pending[0]
		pending = pending[1:]
		imports, err := declaredImports(file)
		if err != nil {
			return nil, err
		}
		for _, imported := range imports {
			target := filepath.Clean(filepath.Join(filepath.Dir(file), imported))
			if seen[target] {
				continue
			}
			seen[target] = true
			if _, err := os.Stat(target); err != nil {
				return nil, fmt.Errorf("import %q of %s: %w", imported, file, err)
			}
			if !within(root, target) {
				outside = append(outside, target)
			}
			pending = append(pending, target)
		}
	}
	sort.Strings(outside)
	return outside, nil
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
	staged  map[string]bool
	pending []stagedFile
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
	}
}

func (c *stagedClosure) followImports() error {
	for len(c.pending) > 0 {
		file := c.pending[0]
		c.pending = c.pending[1:]
		imports, err := declaredImports(file.destination)
		if err != nil {
			return err
		}
		for _, imported := range imports {
			if err := c.stageImport(file, imported); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *stagedClosure) stageImport(file stagedFile, imported string) error {
	destination := filepath.Clean(filepath.Join(filepath.Dir(file.destination), imported))
	if c.staged[destination] {
		return nil
	}
	source := filepath.Clean(filepath.Join(filepath.Dir(file.source), imported))
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

// declaredImports reads the one import list srd050 R1.3 places at the top level
// of a tool, REST, or type unit. A file that is not a declaration decodes to
// nothing and imports nothing.
func declaredImports(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read staged declaration %s: %w", path, err)
	}
	var file struct {
		Imports []string `yaml:"imports"`
	}
	if yaml.Unmarshal(data, &file) != nil {
		return nil, nil
	}
	return file.Imports, nil
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
