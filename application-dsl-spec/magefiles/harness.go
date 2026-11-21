// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

const fixturesDir = "fixtures"

// unitsDir holds the templates and fragments fixtures include. Units are
// scaffolding: no statement owns them and no check judges them.
const unitsDir = fixturesDir + "/units"

// retiredFixturesDir holds the fixtures of retired statements, resolved but
// not judged.
const retiredFixturesDir = fixturesDir + "/retired"

// checkFixtureCoverage enforces the chapter 09 suite layout over the
// statements: every statement in force that binds what an author writes
// (authoredTarget) carries at least one valid and one invalid fixture entry,
// so a grammar rule can never ship checked in one direction only. A statement
// that records a pending gap has said why it falls short, and passes.
func checkFixtureCoverage(language languageFile, authoredTarget string) error {
	var findings []error
	for _, statement := range language.Statements {
		if statement.Target != authoredTarget || statement.retired() || statement.hasPending() {
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
					"%s: %s-target statement has no %s-verdict fixture", statement.ID, authoredTarget, verdict))
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
		if strings.HasPrefix(slug, unitsDir+"/") {
			return nil
		}
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

// checkRetiredFixturePlacement keeps the retired suite apart from the suite
// in force: a retired statement's fixtures live under fixtures/retired/, and
// no statement in force draws on that directory.
func checkRetiredFixturePlacement(language languageFile) error {
	var findings []error
	for _, statement := range language.Statements {
		for _, entry := range statement.Acceptance {
			if entry.Assertion != "fixture" {
				continue
			}
			underRetired := strings.HasPrefix(filepath.ToSlash(filepath.Clean(entry.Path)), retiredFixturesDir+"/")
			switch {
			case statement.retired() && !underRetired:
				findings = append(findings, fmt.Errorf(
					"%s: retired statement's fixture %s lies outside %s/", statement.ID, entry.Path, retiredFixturesDir))
			case !statement.retired() && underRetired:
				findings = append(findings, fmt.Errorf(
					"%s: statement in force draws on retired fixture %s", statement.ID, entry.Path))
			}
		}
	}
	return errors.Join(findings...)
}

// checkFixturesParse proves every file under the version's fixtures/ is
// well-formed YAML, units and retired fixtures included. It is the shape
// floor for a version whose fixture verdicts this repository cannot execute.
func checkFixturesParse(versionRoot string) error {
	var findings []error
	parsed := 0
	err := filepath.Walk(filepath.Join(versionRoot, fixturesDir), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("walk %s: %w", path, err)
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(versionRoot, path)
		if !strings.HasSuffix(path, ".yaml") {
			findings = append(findings, fmt.Errorf("fixture %s is not a .yaml file", filepath.ToSlash(rel)))
			return nil
		}
		if err := parseYAMLStream(path); err != nil {
			findings = append(findings, fmt.Errorf("fixture %s: %w", filepath.ToSlash(rel), err))
		}
		parsed++
		return nil
	})
	if err != nil {
		return err
	}
	if len(findings) > 0 {
		return errors.Join(findings...)
	}
	fmt.Printf("parsed %d fixture files\n", parsed)
	return nil
}

// parseYAMLStream decodes every document in the file at path.
func parseYAMLStream(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	decoder := yaml.NewDecoder(file)
	for {
		var node yaml.Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// checkEvidenceReferences resolves every evidence reference without running
// it: fixture paths exist under the version directory, and go_test and rig
// paths name declared tests. It reports the pending gaps the version records.
func checkEvidenceReferences(roots evidenceRoots, language languageFile) error {
	var findings []error
	pending := 0
	for _, statement := range language.Statements {
		for index, entry := range statement.Acceptance {
			var err error
			switch entry.Assertion {
			case "fixture":
				_, err = resolveModulePath(roots.Version, entry.Path)
			case "go_test":
				var path string
				if path, err = resolveModulePath(roots.Module, entry.Path); err == nil {
					err = checkGoTestDeclared(path, entry.Test)
				}
			case "rig":
				err = runRigEvidence(roots.Repository, entry)
			case assertionPending:
				pending++
			}
			if err != nil {
				findings = append(findings, fmt.Errorf("%s acceptance[%d]: %w", statement.ID, index, err))
			}
		}
	}
	if len(findings) > 0 {
		return fmt.Errorf("evidence references failed: %w", errors.Join(findings...))
	}
	fmt.Printf("resolved evidence references across %d statements; %d pending gaps recorded\n",
		len(language.Statements), pending)
	return nil
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
// leave a constitution claiming conformance to nothing. Constitutions are
// read in either corpus format (eng10). A repository without the
// constitutions directory passes vacuously, since the specification module
// does not require its consumers to exist; a directory holding none fails.
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
		if entry.IsDir() || !isConstitutionFile(entry.Name()) {
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
	if checked == 0 {
		// A constitutions directory with nothing this check reads has lost
		// its documents or changed their format: say so instead of
		// validating nothing.
		findings = append(findings, fmt.Errorf("%s: no constitution to check", constitutionsDir))
	}
	if len(findings) > 0 {
		return errors.Join(findings...)
	}
	fmt.Printf("validated statement citations in %d constitutions\n", checked)
	return nil
}

// isConstitutionFile reports a constitution, a Markdown corpus document
// (eng10).
func isConstitutionFile(name string) bool {
	return strings.HasSuffix(name, ".md")
}

func uniqueStrings(values []string) int {
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
	}
	return len(seen)
}
