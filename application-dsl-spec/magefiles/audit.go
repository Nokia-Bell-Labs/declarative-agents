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

// auditVersion runs the version's gate over its directory.
func auditVersion(moduleRoot string, version specVersion) error {
	language, err := loadLanguage(version.languagePath(moduleRoot))
	if err != nil {
		return err
	}
	if version.Gate == gateReduced {
		return auditReduced(moduleRoot, version, language)
	}
	if err := validateLanguage(language); err != nil {
		return err
	}
	if err := checkFixtureCoverage(language, version.AuthoredTarget); err != nil {
		return err
	}
	roots := rootsFor(moduleRoot, version)
	if err := checkFixtureOwnership(roots.Version, language); err != nil {
		return err
	}
	if err := checkRetiredFixturePlacement(language); err != nil {
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

// auditReduced is the gate for a version the engine does not implement.
func auditReduced(moduleRoot string, version specVersion, language languageFile) error {
	roots := rootsFor(moduleRoot, version)
	checks := []func() error{
		func() error { return validateLanguageSchema(language) },
		func() error { return checkFixtureCoverage(language, version.AuthoredTarget) },
		func() error { return checkFixtureOwnership(roots.Version, language) },
		func() error { return checkRetiredFixturePlacement(language) },
		func() error { return checkFixturesParse(roots.Version) },
		func() error { return checkEvidenceReferences(roots, language) },
	}
	for _, check := range checks {
		if err := check(); err != nil {
			return err
		}
	}
	fmt.Printf("version %s: reduced gate passed over %d statements\n", version.Dir, len(language.Statements))
	return nil
}
