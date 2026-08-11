// Copyright (c) 2026 Nokia. All rights reserved.

package spec

func Validate(g *Graph, corpus *Corpus) []Finding {
	var all []Finding
	all = append(all, checkOrphanedSRDs(g)...)
	all = append(all, checkBrokenTouchpoints(g)...)
	all = append(all, checkBrokenCitations(g, corpus)...)
	all = append(all, checkBareTouchpoints(g, corpus)...)
	all = append(all, checkOrphanedTestSuites(g)...)
	all = append(all, checkUncoveredReqItems(g)...)
	all = append(all, checkUncoveredACs(g)...)
	all = append(all, checkUntracedSuccessCriteria(g, corpus)...)
	all = append(all, checkDependsOnViolations(g)...)
	all = append(all, checkReleasesWithoutTestSuites(g, corpus)...)
	all = append(all, checkMachineActionResolution(corpus)...)
	all = append(all, checkMachineSignalCoverage(corpus)...)
	all = append(all, checkMachineStateMetadata(corpus)...)
	all = append(all, checkMachineSignalMetadata(corpus)...)
	all = append(all, checkMachineNameConsistency(corpus)...)
	all = append(all, validateToolCorpus(corpus)...)
	all = append(all, checkMachineMetricLabels(corpus)...)
	all = append(all, checkUseCaseIndexRefs(corpus)...)
	all = append(all, checkTestSuiteIndexRefs(corpus)...)
	all = append(all, checkRoadmapUseCaseRefs(corpus)...)
	all = append(all, checkUseCaseTestSuiteReciprocity(corpus)...)
	all = append(all, checkTestCaseUseCaseRefs(corpus)...)
	all = append(all, checkSpecIndexPaths(corpus)...)
	all = append(all, checkDocSpecRequirementsSources(corpus)...)
	all = append(all, checkDocSpecRelatedDocuments(corpus)...)
	all = append(all, checkDocSpecImplementationPaths(corpus)...)
	all = append(all, checkDocSpecExamplePaths(corpus)...)
	all = append(all, checkMachineDiagnostics(corpus)...)
	return all
}

func validateToolCorpus(corpus *Corpus) []Finding {
	var findings []Finding
	findings = append(findings, checkToolSelectionDeclared(corpus)...)
	findings = append(findings, checkToolDeclarationVocabulary(corpus)...)
	findings = append(findings, checkSelectedToolContractCompleteness(corpus)...)
	findings = append(findings, checkDeclaredToolContractCompleteness(corpus)...)
	findings = append(findings, checkUnresolvedDeclarationFiles(corpus)...)
	findings = append(findings, checkToolEmitsSignalSet(corpus)...)
	findings = append(findings, checkToolUndoConsistency(corpus)...)
	findings = append(findings, checkToolSideEffectVocab(corpus)...)
	findings = append(findings, checkToolBoundaryCategory(corpus)...)
	findings = append(findings, checkToolMetricConfig(corpus)...)
	return findings
}

// checkOrphanedSRDs finds SRDs that no use case touches.
