// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import "path/filepath"

// languageFileName and renderOutputDirName name a version directory's
// statement database and its rendered-chapter output.
const (
	languageFileName    = "language.yaml"
	renderOutputDirName = "generated-files"
)

// specVersion is one revision of the specification, held in its own
// directory under the module root. Each version's language.yaml is its own
// statement-id namespace: the databases never cross-reference.
type specVersion struct {
	// Dir is the version directory, relative to the module root.
	Dir string
}

// engineVersion is the revision the engine in this repository implements.
// Its chapters render, its constitutions bind, and its evidence runs in full.
var engineVersion = specVersion{Dir: "1.0"}

// specVersions lists every published revision the audit visits, oldest
// first.
var specVersions = []specVersion{engineVersion}

// root is the version directory under moduleRoot.
func (v specVersion) root(moduleRoot string) string {
	return filepath.Join(moduleRoot, v.Dir)
}

// languagePath is the version's statement database under moduleRoot.
func (v specVersion) languagePath(moduleRoot string) string {
	return filepath.Join(v.root(moduleRoot), languageFileName)
}

// renderOutputDir is where the version's rendered chapters land.
func (v specVersion) renderOutputDir(moduleRoot string) string {
	return filepath.Join(v.root(moduleRoot), renderOutputDirName)
}

// evidenceRoots are the three directories acceptance evidence resolves
// against: fixture paths against the version directory that owns them,
// go_test paths against the module (the shared harness lives there), and rig
// paths against the repository.
type evidenceRoots struct {
	Version    string
	Module     string
	Repository string
}

// repositoryRootOf is the repository root for a module root.
func repositoryRootOf(moduleRoot string) string {
	return filepath.Join(moduleRoot, "..")
}

// rootsFor derives a version's evidence roots from the module root.
func rootsFor(moduleRoot string, version specVersion) evidenceRoots {
	return evidenceRoots{
		Version:    version.root(moduleRoot),
		Module:     moduleRoot,
		Repository: repositoryRootOf(moduleRoot),
	}
}
