// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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

// constitutionsDir is where component constitutions bind themselves to the
// specification, relative to the repository root.
const constitutionsDir = "docs/constitutions"

// citationIDPattern finds statement identifiers inside constitution prose;
// statementIDPattern is anchored for validating whole ids and cannot scan.
var citationIDPattern = regexp.MustCompile(`\bR-[A-Z][A-Z0-9]*-[0-9]{3}\b`)

// checkConstitutionCitations enforces the one-way binding from the other
// side: every constitution cites at least one statement identifier, and every
// identifier it cites resolves, so a renumbered or deleted statement cannot
// leave a constitution claiming conformance to nothing. A repository without
// the constitutions directory passes vacuously; the specification module does
// not require its consumers to exist.
func checkConstitutionCitations(repositoryRoot string, language languageFile) error {
	dir := filepath.Join(repositoryRoot, filepath.FromSlash(constitutionsDir))
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("list %s: %w", constitutionsDir, err)
	}
	ids := make(map[string]bool, len(language.Statements))
	for _, statement := range language.Statements {
		ids[statement.ID] = true
	}

	var findings []error
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		checked++
		slug := constitutionsDir + "/" + entry.Name()
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			findings = append(findings, fmt.Errorf("%s: %w", slug, err))
			continue
		}
		citations := citationIDPattern.FindAllString(string(data), -1)
		if len(citations) == 0 {
			findings = append(findings, fmt.Errorf("%s: cites no statement identifier", slug))
		}
		for _, citation := range citations {
			if !ids[citation] {
				findings = append(findings, fmt.Errorf("%s: citation %s resolves to no statement", slug, citation))
			}
		}
	}
	if len(findings) > 0 {
		return errors.Join(findings...)
	}
	if checked > 0 {
		fmt.Printf("validated statement citations in %d constitutions\n", checked)
	}
	return nil
}

func uniqueStrings(values []string) int {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	return len(seen)
}
