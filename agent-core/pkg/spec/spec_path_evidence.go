// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const specPathEvidenceCheck = "spec-path-evidence"

type srdPathEvidence struct {
	ID             string   `yaml:"id"`
	Implementation []string `yaml:"implementation"`
	TestTracePlan  []struct {
		ExistingGoTests []string `yaml:"existing_go_tests"`
	} `yaml:"test_trace_plan"`
}

// hasRuntimeImplementationCorpus distinguishes agent-core's implementation
// evidence from external profile repositories that reuse the binary's formal
// go_test audit but do not own runtime source paths.
func hasRuntimeImplementationCorpus(rootDir string) bool {
	info, err := os.Stat(filepath.Join(rootDir, "internal", "runtime", "core"))
	return err == nil && info.IsDir()
}

// ValidateSpecCorpusPaths verifies existing implementation and Go-test file
// paths named by SRDs. Entries explicitly annotated as planned do not claim
// existing evidence and are skipped.
func ValidateSpecCorpusPaths(rootDir string) ([]Finding, error) {
	matches, err := filepath.Glob(filepath.Join(rootDir, SRDSubdir, SRDGlob))
	if err != nil {
		return nil, fmt.Errorf("glob SRD path evidence: %w", err)
	}
	var findings []Finding
	for _, source := range matches {
		data, err := os.ReadFile(source)
		if err != nil {
			return nil, fmt.Errorf("read SRD path evidence %s: %w", source, err)
		}
		var srd srdPathEvidence
		if err := yaml.Unmarshal(data, &srd); err != nil {
			return nil, fmt.Errorf("parse SRD path evidence %s: %w", source, err)
		}
		for _, raw := range srd.Implementation {
			path, planned := evidencePath(raw)
			if planned || path == "" {
				continue
			}
			findings = appendMissingSpecPath(findings, rootDir, srd.ID, "implementation", path)
		}
		for _, plan := range srd.TestTracePlan {
			for _, raw := range plan.ExistingGoTests {
				path, _ := evidencePath(raw)
				if path == "" {
					continue
				}
				findings = appendMissingSpecPath(findings, rootDir, srd.ID, "existing_go_tests", path)
			}
		}
	}
	sort.Slice(findings, func(i, j int) bool { return findings[i].Message < findings[j].Message })
	return findings, nil
}

func evidencePath(raw string) (path string, planned bool) {
	value := strings.TrimSpace(raw)
	planned = strings.Contains(strings.ToLower(value), "(planned")
	if idx := strings.Index(value, " ("); idx >= 0 {
		value = value[:idx]
	}
	return strings.TrimSpace(value), planned
}

func appendMissingSpecPath(findings []Finding, rootDir, srdID, field, path string) []Finding {
	if _, err := os.Stat(filepath.Join(rootDir, filepath.FromSlash(path))); err == nil {
		return findings
	}
	return append(findings, Finding{
		Check:   specPathEvidenceCheck,
		Level:   "error",
		Message: fmt.Sprintf("SRD %s %s path %q does not exist", srdID, field, path),
	})
}
