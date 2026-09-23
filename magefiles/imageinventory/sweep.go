// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package imageinventory

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	helmRepository = regexp.MustCompile(`(?m)^\s*repository:\s*(\S+)\s*$`)
	helmImageLine  = regexp.MustCompile(`(?m)^\s*image:\s*(\S+)\s*$`)
	goImageConst   = regexp.MustCompile(
		`=\s*"((?:docker\.io|ghcr\.io|registry\.|quay\.io|localhost)/[^"]+)"`)
	kindrigImageRef = regexp.MustCompile(`(?i)(?:repository:\s*|=\s*")(?:docker\.io/)?kindrig/[a-z0-9._-]+`)
	smokeRepository = regexp.MustCompile(`(?i)repository:\s*\S*-smoke\b`)
	localTag        = regexp.MustCompile(`(?i):local\b`)
	latestTag       = regexp.MustCompile(`(?i):latest\b`)
	untypedLocal    = regexp.MustCompile(
		`localhost/declarative-agents/[^:\s]+:([0-9a-f]{12})\b`)
)

// SweepActivePaths scans chart values, Helm templates, rig magefiles, and the
// agent-core Dockerfile for prohibited image identities (ENG01 C7).
func SweepActivePaths(repoRoot string) ([]GrammarFinding, error) {
	repoRoot = filepath.Clean(repoRoot)
	var findings []GrammarFinding
	err := filepath.WalkDir(repoRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "build", "node_modules", "dist", "vendor", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !sweepTarget(rel) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		findings = append(findings, scanContent(rel, string(content))...)
		return nil
	})
	return findings, err
}

func sweepTarget(rel string) bool {
	if strings.HasSuffix(rel, "_test.go") {
		return false
	}
	if rel == "agent-core/Dockerfile" {
		return true
	}
	if strings.Contains(rel, "/helm/") {
		return strings.HasSuffix(rel, ".yaml") || strings.HasSuffix(rel, ".yml") ||
			strings.HasSuffix(rel, ".tpl")
	}
	if strings.HasPrefix(rel, "applications/") && strings.Contains(rel, "/magefiles/") &&
		strings.HasSuffix(rel, ".go") {
		return true
	}
	if strings.HasPrefix(rel, "magefiles/") &&
		(strings.HasSuffix(rel, ".go") || strings.HasSuffix(rel, ".yaml") || strings.HasSuffix(rel, ".yml")) {
		return !strings.HasSuffix(rel, "_test.go")
	}
	return false
}

func scanContent(rel, content string) []GrammarFinding {
	var findings []GrammarFinding
	lines := strings.Split(content, "\n")
	isHelm := strings.Contains(rel, "/helm/") || strings.HasSuffix(rel, ".yaml") || strings.HasSuffix(rel, ".yml")
	for lineNum, line := range lines {
		trimmed := strings.TrimSpace(line)
		if kindrigImageRef.MatchString(trimmed) {
			findings = append(findings, GrammarFinding{
				Path: rel, Line: lineNum + 1, Image: trimmed, Rule: "R10.3",
				Detail: "kindrig alias hides upstream ownership",
			})
		}
		if isHelm && smokeRepository.MatchString(trimmed) {
			findings = append(findings, GrammarFinding{
				Path: rel, Line: lineNum + 1, Image: trimmed, Rule: "R10.3",
				Detail: "consumer-oriented smoke repository",
			})
		}
	}
	for _, image := range extractImageLiterals(rel, content) {
		if strings.Contains(image, "${") || strings.HasPrefix(image, "KINDRIG_") {
			continue
		}
		if localTag.MatchString(image) {
			findings = append(findings, GrammarFinding{
				Path: rel, Image: image, Rule: "R10.3",
				Detail: "local is a mutable alias, not an identity",
			})
		}
		if latestTag.MatchString(image) {
			findings = append(findings, GrammarFinding{
				Path: rel, Image: image, Rule: "C1",
				Detail: "floating latest tag",
			})
		}
		if untypedLocal.MatchString(image) {
			findings = append(findings, GrammarFinding{
				Path: rel, Image: image, Rule: "R10.3",
				Detail: "untyped 12-hex local tag",
			})
		}
		if strings.HasPrefix(strings.ToLower(image), "kindrig/") ||
			strings.Contains(strings.ToLower(image), "/kindrig/") {
			findings = append(findings, GrammarFinding{
				Path: rel, Image: image, Rule: "R10.3",
				Detail: "kindrig alias hides upstream ownership",
			})
		}
	}
	return findings
}

func extractImageLiterals(rel, content string) []string {
	var images []string
	if strings.Contains(rel, "/helm/") || strings.HasSuffix(rel, ".yaml") || strings.HasSuffix(rel, ".yml") {
		for _, match := range helmRepository.FindAllStringSubmatch(content, -1) {
			images = append(images, match[1])
		}
		for _, match := range helmImageLine.FindAllStringSubmatch(content, -1) {
			if match[1] == "" || match[1] == `""` {
				continue
			}
			images = append(images, match[1])
		}
	}
	if strings.HasSuffix(rel, ".go") {
		for _, match := range goImageConst.FindAllStringSubmatch(content, -1) {
			images = append(images, match[1])
		}
	}
	if rel == "agent-core/Dockerfile" {
		for _, line := range strings.Split(content, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(strings.ToUpper(line), "FROM ") {
				continue
			}
			image := strings.TrimSpace(strings.TrimPrefix(line, "FROM"))
			if image == "" || strings.Contains(image, "${") {
				continue
			}
			images = append(images, image)
		}
	}
	return images
}
