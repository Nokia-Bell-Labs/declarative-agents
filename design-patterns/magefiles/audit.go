// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const referenceImplementationCitation = "reference-implementation"

type patternLanguageEvidence struct {
	Patterns []struct {
		ID       string `yaml:"id"`
		Examples []struct {
			System   string             `yaml:"system"`
			Cite     string             `yaml:"cite"`
			Kind     string             `yaml:"kind"`
			Note     string             `yaml:"note"`
			Evidence *referenceEvidence `yaml:"evidence"`
		} `yaml:"examples"`
	} `yaml:"patterns"`
}

type referenceEvidence struct {
	Classification string          `yaml:"classification"`
	Checks         []evidenceCheck `yaml:"checks"`
}

type evidenceCheck struct {
	Path     string   `yaml:"path"`
	Contains []string `yaml:"contains"`
}

// Audit verifies reference-implementation claims, then renders figures and builds the PDF.
func Audit() error {
	return auditDesignPatterns(
		func() error { return auditReferenceImplementationEvidence("pattern-language.yaml", "..") },
		All,
	)
}

func auditDesignPatterns(evidence, build func() error) error {
	if err := evidence(); err != nil {
		return err
	}
	return build()
}

func auditReferenceImplementationEvidence(languagePath, repositoryRoot string) error {
	data, err := os.ReadFile(languagePath)
	if err != nil {
		return fmt.Errorf("read pattern language: %w", err)
	}
	var language patternLanguageEvidence
	if err := yaml.Unmarshal(data, &language); err != nil {
		return fmt.Errorf("parse pattern language: %w", err)
	}

	var findings []error
	claims := 0
	for _, pattern := range language.Patterns {
		for _, example := range pattern.Examples {
			if example.Cite != referenceImplementationCitation || example.Kind != "internal" {
				continue
			}
			claims++
			label := fmt.Sprintf("%s / %s", pattern.ID, example.System)
			if err := validateReferenceEvidence(repositoryRoot, label, example.Note, example.Evidence); err != nil {
				findings = append(findings, err)
			}
		}
	}
	if claims == 0 {
		findings = append(findings, errors.New("pattern language has no internal reference-implementation claims"))
	}
	if len(findings) > 0 {
		return fmt.Errorf("reference-implementation evidence audit failed: %w", errors.Join(findings...))
	}
	fmt.Printf("validated %d reference-implementation claims\n", claims)
	return nil
}

func validateReferenceEvidence(repositoryRoot, label, note string, evidence *referenceEvidence) error {
	if evidence == nil {
		return fmt.Errorf("%s: evidence is required", label)
	}
	classification := strings.TrimSpace(evidence.Classification)
	switch classification {
	case "shipped":
		if len(evidence.Checks) == 0 {
			return fmt.Errorf("%s: shipped evidence requires at least one check", label)
		}
	case "conformance_fixture":
		if !strings.Contains(strings.ToLower(note), "conformance fixture") {
			return fmt.Errorf("%s: conformance_fixture note must say \"conformance fixture\"", label)
		}
		if len(evidence.Checks) == 0 {
			return fmt.Errorf("%s: conformance_fixture evidence requires at least one check", label)
		}
	case "design_intent":
		if !strings.Contains(strings.ToLower(note), "design intent") {
			return fmt.Errorf("%s: design_intent note must say \"design intent\"", label)
		}
	default:
		return fmt.Errorf("%s: unknown evidence classification %q", label, classification)
	}

	var findings []error
	for _, check := range evidence.Checks {
		if err := runEvidenceCheck(repositoryRoot, label, check); err != nil {
			findings = append(findings, err)
		}
	}
	return errors.Join(findings...)
}

func runEvidenceCheck(repositoryRoot, label string, check evidenceCheck) error {
	if strings.TrimSpace(check.Path) == "" {
		return fmt.Errorf("%s: evidence check path is required", label)
	}
	root, err := filepath.Abs(repositoryRoot)
	if err != nil {
		return fmt.Errorf("%s: resolve repository root: %w", label, err)
	}
	path, err := filepath.Abs(filepath.Join(root, filepath.Clean(check.Path)))
	if err != nil {
		return fmt.Errorf("%s: resolve evidence path %q: %w", label, check.Path, err)
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("%s: evidence path %q escapes repository root", label, check.Path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: evidence path %q: %w", label, check.Path, err)
	}
	if len(check.Contains) == 0 {
		return nil
	}
	if info.IsDir() {
		return fmt.Errorf("%s: evidence path %q is a directory but contains checks were requested", label, check.Path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("%s: read evidence path %q: %w", label, check.Path, err)
	}
	content := string(data)
	var findings []error
	for _, token := range check.Contains {
		if !strings.Contains(content, token) {
			findings = append(findings, fmt.Errorf("%s: evidence path %q does not contain %q", label, check.Path, token))
		}
	}
	return errors.Join(findings...)
}
