// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package spec

import (
	"fmt"
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
				Message: fmt.Sprintf("SPECIFICATIONS.md use_case_index references %q which does not exist", entry.ID),
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
				Message: fmt.Sprintf("SPECIFICATIONS.md test_suite_index references %q which does not exist", entry.ID),
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

// indexPath is one path an index names, with the table and entry it came from.
type indexPath struct {
	table, id, path string
}

// indexPaths lists the paths every SPECIFICATIONS.md index table names.
func (si *SpecIndex) indexPaths() []indexPath {
	var paths []indexPath
	for _, e := range si.FoundationDocumentIndex {
		paths = append(paths, indexPath{"foundation_document_index", e.ID, e.Path})
	}
	for _, e := range si.SRDIndex {
		paths = append(paths, indexPath{"srd_index", e.ID, e.Path})
	}
	for _, e := range si.ConfigFormatIndex {
		paths = append(paths, indexPath{"config_format_index", e.ID, e.Path})
	}
	for _, e := range si.SemanticModelIndex {
		paths = append(paths, indexPath{"semantic_model_index", e.ID, e.Path})
	}
	for _, e := range si.UseCaseIndex {
		paths = append(paths, indexPath{"use_case_index", e.ID, e.Path})
	}
	for _, e := range si.TestSuiteIndex {
		paths = append(paths, indexPath{"test_suite_index", e.ID, e.Path})
	}
	return paths
}

// checkSpecIndexPaths verifies that every path an index table or the
// architecture's related_documents names is an existing file. Paths are
// relative to the corpus root, never to the document that writes them.

func checkSpecIndexPaths(corpus *Corpus) []Finding {
	if corpus.RootDir == "" {
		return nil
	}
	var findings []Finding
	for _, p := range corpus.SpecIndex.indexPaths() {
		if p.path != "" && !fileExists(filepath.Join(corpus.RootDir, p.path)) {
			findings = append(findings, Finding{
				Check:   "index-broken-path",
				Level:   "error",
				Message: fmt.Sprintf("%s entry %s path %q does not exist", p.table, p.id, p.path),
			})
		}
	}
	for _, path := range corpus.RelatedDocuments {
		if !fileExists(filepath.Join(corpus.RootDir, path)) {
			findings = append(findings, Finding{
				Check:   "index-broken-path",
				Level:   "error",
				Message: fmt.Sprintf("%s related_documents entry %q does not exist", ArchFile, path),
			})
		}
	}
	return findings
}

// validateIndexPaths runs the checks on paths the index tables and the
// architecture's related_documents name.
func validateIndexPaths(corpus *Corpus) []Finding {
	return append(checkSpecIndexPaths(corpus), checkMalformedRelatedDocuments(corpus)...)
}

// checkMalformedRelatedDocuments reports related_documents entries that name
// no path, which the corpus load kept rather than failing on.

func checkMalformedRelatedDocuments(corpus *Corpus) []Finding {
	findings := make([]Finding, 0, len(corpus.MalformedRelatedDocuments))
	for _, msg := range corpus.MalformedRelatedDocuments {
		findings = append(findings, Finding{
			Check:   "related-document-malformed",
			Level:   "error",
			File:    ArchFile,
			Message: msg + "; write a path or a {doc, purpose} row",
		})
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
			if !fileExists(filepath.Join(corpus.RootDir, path)) {
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
			if !fileExists(filepath.Join(corpus.RootDir, path)) {
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
			if !fileExists(filepath.Join(corpus.RootDir, ex.File)) {
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

// checkRoadmapSummaryCoverage verifies that every release in road-map.md has
// a row in SPECIFICATIONS.md's roadmap_summary. The two are maintained by
// hand, and a release that reaches the road map without reaching the summary
// leaves the index quietly describing a smaller project than the one that
// exists (GH-2310).
func checkRoadmapSummaryCoverage(corpus *Corpus) []Finding {
	if len(corpus.Roadmap.Releases) == 0 {
		return nil
	}
	summarized := make(map[string]bool, len(corpus.SpecIndex.RoadmapSummary))
	for _, entry := range corpus.SpecIndex.RoadmapSummary {
		summarized[entry.Version] = true
	}
	var findings []Finding
	for _, release := range corpus.Roadmap.Releases {
		if release.Version == "" || summarized[release.Version] {
			continue
		}
		findings = append(findings, Finding{
			Check: "roadmap-summary-missing-release",
			Level: "error",
			Message: fmt.Sprintf(
				"road-map.md release %s has no roadmap_summary row in SPECIFICATIONS.md",
				release.Version),
		})
	}
	return findings
}

// checkIndexDocumentAgreement verifies that what SPECIFICATIONS.md says
// about a use case or test suite matches what that document says about
// itself. An index is only useful while it agrees with what it indexes, and
// nothing else compares the two (GH-2310).
func checkIndexDocumentAgreement(corpus *Corpus) []Finding {
	var findings []Finding
	for _, entry := range corpus.SpecIndex.UseCaseIndex {
		document, ok := corpus.UseCases[entry.ID]
		if !ok {
			continue // index-missing-use-case reports this
		}
		findings = append(findings,
			indexFieldMismatch("use_case_index", entry.ID, "title", entry.Title, document.Title)...)
		findings = append(findings,
			indexFieldMismatch("use_case_index", entry.ID, "status", entry.Status, document.Status)...)
	}
	for _, entry := range corpus.SpecIndex.TestSuiteIndex {
		document, ok := corpus.TestSuites[entry.ID]
		if !ok {
			continue // index-missing-test-suite reports this
		}
		findings = append(findings,
			indexFieldMismatch("test_suite_index", entry.ID, "title", entry.Title, document.Title)...)
		findings = append(findings,
			indexFieldMismatch("test_suite_index", entry.ID, "release", entry.Release, document.Release)...)
	}
	return findings
}

// indexFieldMismatch reports one disagreeing field. Silence on either side is
// not disagreement: an index may summarize less than the document states, and
// a document that omits a field is not contradicting the index that carries
// one. Only two present and different values are a mismatch; the checks that
// require an entry to exist at all are separate.
func indexFieldMismatch(index, id, field, indexed, documented string) []Finding {
	indexed, documented = strings.TrimSpace(indexed), strings.TrimSpace(documented)
	if indexed == "" || documented == "" || indexed == documented {
		return nil
	}
	return []Finding{{
		Check: "index-document-mismatch",
		Level: "error",
		Message: fmt.Sprintf(
			"SPECIFICATIONS.md %s entry %s states %s %q but the document states %q",
			index, id, field, indexed, documented),
	}}
}
