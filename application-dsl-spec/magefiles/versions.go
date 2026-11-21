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
	// AuthoredTarget is the conformance target that binds what an author
	// writes: "document" in 1.0, "specification" from 2.0.
	AuthoredTarget string
	// Gate selects how much of the audit the version takes.
	Gate gateKind
}

// gateKind is how much of the audit a version takes.
type gateKind int

const (
	// gateFull runs everything: schema, terminology, fixture coverage and
	// ownership, constitution citations, every evidence entry, and the
	// chapter render. It is for the version the engine implements.
	gateFull gateKind = iota
	// gateReduced checks what this repository can honestly check of a
	// version its engine does not implement: schema, fixture coverage and
	// ownership, fixture shape, and that every evidence reference resolves.
	// It runs no fixture verdict, binds no constitution, and renders nothing.
	gateReduced
)

// engineVersion is the revision the engine in this repository implements.
// Its chapters render, its constitutions bind, and its evidence runs in full.
var engineVersion = specVersion{Dir: "1.0", AuthoredTarget: "document", Gate: gateFull}

// currentVersion is the current revision of the language. It publishes as
// statements, fixtures, and a README.
var currentVersion = specVersion{Dir: "2.0", AuthoredTarget: "specification", Gate: gateReduced}

// specVersions lists every published revision the audit visits, oldest
// first.
var specVersions = []specVersion{engineVersion, currentVersion}

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
