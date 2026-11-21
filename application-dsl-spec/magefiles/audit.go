// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import "fmt"

// Audit is the specification gate: for every published version it validates
// the statement schema, runs every statement's acceptance evidence, and
// renders the chapters so citation drift fails alongside evidence drift. The
// root audit dispatches here like any other sub-module.
func Audit() error {
	for _, version := range specVersions {
		if err := auditVersion(".", version); err != nil {
			return fmt.Errorf("version %s: %w", version.Dir, err)
		}
	}
	return nil
}

// auditVersion runs the full gate over one version directory.
func auditVersion(moduleRoot string, version specVersion) error {
	language, err := loadLanguage(version.languagePath(moduleRoot))
	if err != nil {
		return err
	}
	if err := validateLanguage(language); err != nil {
		return err
	}
	if err := checkFixtureCoverage(language); err != nil {
		return err
	}
	roots := rootsFor(moduleRoot, version)
	if err := checkFixtureOwnership(roots.Version, language); err != nil {
		return err
	}
	if err := checkConstitutionCitations(roots.Repository, language); err != nil {
		return err
	}
	if err := runAcceptanceEvidence(roots, language); err != nil {
		return err
	}
	return renderChapters(roots.Version, version.renderOutputDir(moduleRoot), roots.Repository, language)
}
