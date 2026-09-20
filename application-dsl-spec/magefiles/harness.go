// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const fixturesDir = "fixtures"

// checkFixtureCoverage enforces the chapter 09 suite layout over the
// statements: every document-target statement carries at least one valid and
// one invalid fixture entry, so a grammar rule can never ship checked in one
// direction only.
func checkFixtureCoverage(language languageFile) error {
	var findings []error
	for _, statement := range language.Statements {
		if statement.Target != "document" {
			continue
		}
		verdicts := map[string]bool{}
		for _, entry := range statement.Acceptance {
			if entry.Assertion == "fixture" {
				verdicts[entry.Verdict] = true
			}
		}
		for _, verdict := range []string{"valid", "invalid"} {
			if !verdicts[verdict] {
				findings = append(findings, fmt.Errorf(
					"%s: document-target statement has no %s-verdict fixture", statement.ID, verdict))
			}
		}
	}
	return errors.Join(findings...)
}

// checkFixtureOwnership enforces the other half of the layout: every fixture
// file under fixtures/ is referenced by exactly one statement, so a suite
// cannot hold documents nothing measures and two statements cannot share a
// fixture whose meaning then blurs.
func checkFixtureOwnership(moduleRoot string, language languageFile) error {
	owners := map[string][]string{}
	for _, statement := range language.Statements {
		for _, entry := range statement.Acceptance {
			if entry.Assertion == "fixture" {
				owners[filepath.ToSlash(filepath.Clean(entry.Path))] = append(
					owners[filepath.ToSlash(filepath.Clean(entry.Path))], statement.ID)
			}
		}
	}

	var findings []error
	root := filepath.Join(moduleRoot, fixturesDir)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		rel, relErr := filepath.Rel(moduleRoot, path)
		if relErr != nil {
			return relErr
		}
		slug := filepath.ToSlash(rel)
		referents := owners[slug]
		switch {
		case len(referents) == 0:
			findings = append(findings, fmt.Errorf("fixture %s is referenced by no statement", slug))
		case uniqueStrings(referents) > 1:
			findings = append(findings, fmt.Errorf(
				"fixture %s is owned by %d statements (%s), want exactly one",
				slug, uniqueStrings(referents), strings.Join(referents, ", ")))
		}
		return nil
	})
	if err != nil {
		return err
	}
	return errors.Join(findings...)
}

func uniqueStrings(values []string) int {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	return len(seen)
}
