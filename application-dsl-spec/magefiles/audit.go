// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

// Audit is the specification gate: it validates the statement schema, runs
// every statement's acceptance evidence, and renders the chapters so citation
// drift fails alongside evidence drift. The root audit dispatches here like
// any other sub-module.
func Audit() error {
	language, err := loadLanguage(languagePath)
	if err != nil {
		return err
	}
	if err := validateLanguage(language); err != nil {
		return err
	}
	if err := checkFixtureCoverage(language); err != nil {
		return err
	}
	if err := checkFixtureOwnership(".", language); err != nil {
		return err
	}
	if err := runAcceptanceEvidence(".", language); err != nil {
		return err
	}
	return renderChapters(".", renderOutputDir, language)
}
