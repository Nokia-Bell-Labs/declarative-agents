// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func checkUseCaseIndexRefs(corpus *Corpus) []Finding {
	var findings []Finding
	for _, entry := range corpus.SpecIndex.UseCaseIndex {
		if _, ok := corpus.UseCases[entry.ID]; !ok {
			findings = append(findings, Finding{
				Check:   "index-missing-use-case",
				Level:   "error",
				Message: fmt.Sprintf("SPECIFICATIONS.yaml use_case_index references %q which does not exist", entry.ID),
			})
		}
	}
	return findings
}

// checkTestSuiteIndexRefs verifies that every test_suite_index entry
// references a test suite that exists in the corpus.

func checkTestSuiteIndexRefs(corpus *Corpus) []Finding {
	var findings []Finding
	for _, entry := range corpus.SpecIndex.TestSuiteIndex {
		if _, ok := corpus.TestSuites[entry.ID]; !ok {
			findings = append(findings, Finding{
				Check:   "index-missing-test-suite",
				Level:   "error",
				Message: fmt.Sprintf("SPECIFICATIONS.yaml test_suite_index references %q which does not exist", entry.ID),
			})
		}
	}
	return findings
}

// checkRoadmapUseCaseRefs verifies that use case IDs in roadmap releases
// reference use cases that exist in the corpus.

func checkRoadmapUseCaseRefs(corpus *Corpus) []Finding {
	var findings []Finding
	for _, rel := range corpus.Roadmap.Releases {
		for _, ucRef := range rel.UseCases {
			if _, ok := corpus.UseCases[ucRef.ID]; !ok {
				findings = append(findings, Finding{
					Check:   "roadmap-missing-use-case",
					Level:   "warning",
					Message: fmt.Sprintf("roadmap release %s references use case %q which does not exist", rel.Version, ucRef.ID),
				})
			}
		}
	}
	return findings
}

// checkUseCaseTestSuiteReciprocity verifies that use case test_suite
// fields reference test suites that exist, and that those test suites
// trace back to the use case.

func checkUseCaseTestSuiteReciprocity(corpus *Corpus) []Finding {
	var findings []Finding
	for _, ucID := range corpus.UCOrder {
		uc := corpus.UseCases[ucID]
		if uc.TestSuite == "" {
			continue
		}
		ts, ok := corpus.TestSuites[uc.TestSuite]
		if !ok {
			findings = append(findings, Finding{
				Check:   "use-case-missing-test-suite",
				Level:   "error",
				Message: fmt.Sprintf("use case %s references test_suite %q which does not exist", ucID, uc.TestSuite),
			})
			continue
		}
		tracesUC := false
		for _, trace := range ts.Traces {
			if trace == ucID {
				tracesUC = true
				break
			}
		}
		if !tracesUC {
			findings = append(findings, Finding{
				Check:   "test-suite-missing-uc-trace",
				Level:   "warning",
				Message: fmt.Sprintf("use case %s references test_suite %q but the suite does not trace back to it", ucID, uc.TestSuite),
			})
		}
	}
	return findings
}

// checkTestCaseUseCaseRefs verifies that test case use_case fields
// reference use cases that exist in the corpus.

func checkTestCaseUseCaseRefs(corpus *Corpus) []Finding {
	var findings []Finding
	for _, ts := range corpus.TestSuites {
		for _, tc := range ts.TestCases {
			if tc.UseCase == "" {
				continue
			}
			if _, ok := corpus.UseCases[tc.UseCase]; !ok {
				findings = append(findings, Finding{
					Check:   "test-case-missing-use-case",
					Level:   "warning",
					Message: fmt.Sprintf("test case %s:%s references use_case %q which does not exist", ts.ID, tc.Name, tc.UseCase),
				})
			}
		}
	}
	return findings
}

// checkSpecIndexPaths verifies that path fields in spec index entries
// point to existing files.

func checkSpecIndexPaths(corpus *Corpus) []Finding {
	if corpus.RootDir == "" {
		return nil
	}
	var findings []Finding
	for _, entry := range corpus.SpecIndex.SRDIndex {
		if entry.Path != "" {
			if _, err := os.Stat(filepath.Join(corpus.RootDir, entry.Path)); err != nil {
				findings = append(findings, Finding{
					Check:   "index-broken-path",
					Level:   "error",
					Message: fmt.Sprintf("srd_index entry %s path %q does not exist", entry.ID, entry.Path),
				})
			}
		}
	}
	for _, entry := range corpus.SpecIndex.UseCaseIndex {
		if entry.Path != "" {
			if _, err := os.Stat(filepath.Join(corpus.RootDir, entry.Path)); err != nil {
				findings = append(findings, Finding{
					Check:   "index-broken-path",
					Level:   "error",
					Message: fmt.Sprintf("use_case_index entry %s path %q does not exist", entry.ID, entry.Path),
				})
			}
		}
	}
	for _, entry := range corpus.SpecIndex.TestSuiteIndex {
		if entry.Path != "" {
			if _, err := os.Stat(filepath.Join(corpus.RootDir, entry.Path)); err != nil {
				findings = append(findings, Finding{
					Check:   "index-broken-path",
					Level:   "error",
					Message: fmt.Sprintf("test_suite_index entry %s path %q does not exist", entry.ID, entry.Path),
				})
			}
		}
	}
	return findings
}

// checkDocSpecRequirementsSources verifies that canonical requirement
// source paths in doc specs point to existing files.

func checkDocSpecRequirementsSources(corpus *Corpus) []Finding {
	if corpus.RootDir == "" {
		return nil
	}
	var findings []Finding
	for id, ds := range corpus.DocSpecs {
		for _, path := range ds.RequirementsSource.Canonical {
			if _, err := os.Stat(filepath.Join(corpus.RootDir, path)); err != nil {
				findings = append(findings, Finding{
					Check:   "docspec-broken-requirement-source",
					Level:   "error",
					Message: fmt.Sprintf("doc spec %s canonical requirements_source %q does not exist", id, path),
				})
			}
		}
	}
	return findings
}

// checkDocSpecRelatedDocuments verifies that related_documents IDs
// resolve to known SRDs or doc specs.

func checkDocSpecRelatedDocuments(corpus *Corpus) []Finding {
	knownIDs := make(map[string]bool)
	for srdID := range corpus.SRDs {
		knownIDs[srdID] = true
		parts := strings.SplitN(srdID, "-", 2)
		if len(parts) > 0 {
			knownIDs[parts[0]] = true
		}
	}
	for dsID := range corpus.DocSpecs {
		knownIDs[dsID] = true
	}

	var findings []Finding
	for id, ds := range corpus.DocSpecs {
		for _, ref := range ds.RelatedDocuments {
			if !knownIDs[ref] {
				findings = append(findings, Finding{
					Check:   "docspec-broken-related-document",
					Level:   "warning",
					Message: fmt.Sprintf("doc spec %s related_documents references %q which is not a known SRD or spec", id, ref),
				})
			}
		}
	}
	return findings
}

// checkDocSpecImplementationPaths verifies that implementation file
// paths in doc specs exist on disk.

func checkDocSpecImplementationPaths(corpus *Corpus) []Finding {
	if corpus.RootDir == "" {
		return nil
	}
	var findings []Finding
	for id, ds := range corpus.DocSpecs {
		for _, path := range ds.Implementation.Paths {
			if _, err := os.Stat(filepath.Join(corpus.RootDir, path)); err != nil {
				findings = append(findings, Finding{
					Check:   "docspec-broken-implementation-path",
					Level:   "error",
					Message: fmt.Sprintf("doc spec %s implementation path %q does not exist", id, path),
				})
			}
		}
	}
	return findings
}

// checkDocSpecExamplePaths verifies that example file paths in doc specs
// exist on disk.

func checkDocSpecExamplePaths(corpus *Corpus) []Finding {
	if corpus.RootDir == "" {
		return nil
	}
	var findings []Finding
	for id, ds := range corpus.DocSpecs {
		for _, ex := range ds.Examples {
			if ex.File == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(corpus.RootDir, ex.File)); err != nil {
				findings = append(findings, Finding{
					Check:   "docspec-broken-example-path",
					Level:   "warning",
					Message: fmt.Sprintf("doc spec %s example file %q does not exist", id, ex.File),
				})
			}
		}
	}
	return findings
}

// checkMachineDiagnostics runs core.DiagnoseMachineSpec on each loaded
// machine and surfaces its diagnostics as findings.
