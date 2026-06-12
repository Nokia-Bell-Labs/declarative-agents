// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
)

// Finding represents a single validation result.
type Finding struct {
	Check   string // short check identifier
	Level   string // "error" or "warning"
	Message string
}

// Validate runs all consistency checks on the graph and returns findings.
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
	all = append(all, checkToolSelectionDeclared(corpus)...)
	all = append(all, checkSelectedToolContractCompleteness(corpus)...)
	all = append(all, checkToolEmitsSignalSet(corpus)...)
	all = append(all, checkToolUndoConsistency(corpus)...)
	all = append(all, checkToolSideEffectVocab(corpus)...)
	all = append(all, checkToolBoundaryCategory(corpus)...)
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

// checkOrphanedSRDs finds SRDs that no use case touches.
func checkOrphanedSRDs(g *Graph) []Finding {
	var findings []Finding
	for _, srd := range g.NodesByKind(KindSRD) {
		incoming := g.IncomingByRel(srd.ID, RelTouches)
		if len(incoming) == 0 {
			findings = append(findings, Finding{
				Check:   "orphaned-srd",
				Level:   "warning",
				Message: fmt.Sprintf("SRD %s is not referenced by any use case touchpoint", srd.ID),
			})
		}
	}
	return findings
}

// checkBrokenTouchpoints verifies that use case touches/cites edges point
// to nodes that actually exist. Since BuildGraph only creates edges to
// existing nodes, we check by comparing the touchpoint SRD references
// in the corpus against the SRD nodes in the graph.
func checkBrokenTouchpoints(g *Graph) []Finding {
	var findings []Finding
	for _, uc := range g.NodesByKind(KindUseCase) {
		touches := g.OutgoingByRel(uc.ID, RelTouches)
		for _, targetID := range touches {
			if _, ok := g.Node(targetID); !ok {
				findings = append(findings, Finding{
					Check:   "broken-touchpoint",
					Level:   "error",
					Message: fmt.Sprintf("use case %s touchpoint references non-existent SRD %s", uc.ID, targetID),
				})
			}
		}
	}
	return findings
}

// checkBrokenCitations verifies that cites edges from use cases to
// requirement groups reference groups that exist in the graph.
func checkBrokenCitations(g *Graph, corpus *Corpus) []Finding {
	var findings []Finding
	for _, ucID := range corpus.UCOrder {
		uc := corpus.UseCases[ucID]
		for _, tp := range uc.Touchpoints {
			srdID, groups := parseTouchpoint(tp)
			if srdID == "" {
				continue
			}
			for _, grp := range groups {
				groupNodeID := srdID + ":" + grp
				if _, ok := g.Node(groupNodeID); !ok {
					findings = append(findings, Finding{
						Check:   "broken-citation",
						Level:   "error",
						Message: fmt.Sprintf("use case %s cites %s %s but requirement group not found", ucID, srdID, grp),
					})
				}
			}
		}
	}
	return findings
}

// checkBareTouchpoints flags use case touchpoints that cite an SRD
// without specifying any requirement group references.
func checkBareTouchpoints(g *Graph, corpus *Corpus) []Finding {
	var findings []Finding
	for _, ucID := range corpus.UCOrder {
		uc := corpus.UseCases[ucID]
		for _, tp := range uc.Touchpoints {
			srdID, groups := parseTouchpoint(tp)
			if srdID == "" {
				continue
			}
			if len(groups) == 0 {
				findings = append(findings, Finding{
					Check:   "bare-touchpoint",
					Level:   "warning",
					Message: fmt.Sprintf("use case %s cites %s without R-group references", ucID, srdID),
				})
			}
		}
	}
	return findings
}

// checkOrphanedTestSuites finds test suites whose covers edges
// don't connect to any use case node that exists in the graph.
func checkOrphanedTestSuites(g *Graph) []Finding {
	var findings []Finding
	for _, ts := range g.NodesByKind(KindTestSuite) {
		covers := g.OutgoingByRel(ts.ID, RelCovers)
		hasUC := false
		for _, targetID := range covers {
			if n, ok := g.Node(targetID); ok && n.Kind == KindUseCase {
				hasUC = true
				break
			}
		}
		if !hasUC {
			findings = append(findings, Finding{
				Check:   "orphaned-test-suite",
				Level:   "warning",
				Message: fmt.Sprintf("test suite %s traces don't reference any known use case", ts.ID),
			})
		}
	}
	return findings
}

// checkUncoveredReqItems finds requirement items that are not traced
// by any acceptance criterion.
func checkUncoveredReqItems(g *Graph) []Finding {
	var findings []Finding
	for _, item := range g.NodesByKind(KindReqItem) {
		incoming := g.IncomingByRel(item.ID, RelTraces)
		if len(incoming) == 0 {
			findings = append(findings, Finding{
				Check:   "uncovered-req-item",
				Level:   "error",
				Message: fmt.Sprintf("requirement item %s not covered by any acceptance criterion", item.ID),
			})
		}
	}
	return findings
}

// checkUncoveredACs finds acceptance criteria not covered by any test case.
func checkUncoveredACs(g *Graph) []Finding {
	var findings []Finding
	for _, ac := range g.NodesByKind(KindAC) {
		incoming := g.IncomingByRel(ac.ID, RelCovers)
		if len(incoming) == 0 {
			findings = append(findings, Finding{
				Check:   "uncovered-ac",
				Level:   "warning",
				Message: fmt.Sprintf("acceptance criterion %s not covered by any test case", ac.ID),
			})
		}
	}
	return findings
}

// checkUntracedSuccessCriteria finds use case success criteria that
// don't cite any AC in their traces.
func checkUntracedSuccessCriteria(g *Graph, corpus *Corpus) []Finding {
	var findings []Finding
	for _, ucID := range corpus.UCOrder {
		uc := corpus.UseCases[ucID]
		for _, sc := range uc.SuccessCriteria {
			hasACTrace := false
			for _, tr := range sc.Traces {
				parts := strings.Fields(tr)
				if len(parts) >= 2 && strings.HasPrefix(parts[0], "srd") && strings.HasPrefix(parts[1], "AC") {
					hasACTrace = true
					break
				}
			}
			if !hasACTrace {
				findings = append(findings, Finding{
					Check:   "untraced-success-criterion",
					Level:   "warning",
					Message: fmt.Sprintf("use case %s success criterion %s has no AC trace", ucID, sc.ID),
				})
			}
		}
	}
	return findings
}

// checkDependsOnViolations verifies that inter-SRD depends_on references
// point to SRDs that exist.
func checkDependsOnViolations(g *Graph) []Finding {
	var findings []Finding
	for _, srd := range g.NodesByKind(KindSRD) {
		deps := g.OutgoingByRel(srd.ID, RelDependsOn)
		for _, depID := range deps {
			if _, ok := g.Node(depID); !ok {
				findings = append(findings, Finding{
					Check:   "depends-on-violation",
					Level:   "error",
					Message: fmt.Sprintf("SRD %s depends_on %s which does not exist", srd.ID, depID),
				})
			}
		}
	}
	return findings
}

// checkReleasesWithoutTestSuites verifies that each release with use cases
// has a corresponding test suite.
func checkReleasesWithoutTestSuites(g *Graph, corpus *Corpus) []Finding {
	var findings []Finding

	testSuiteReleases := make(map[string]bool)
	for _, ts := range g.NodesByKind(KindTestSuite) {
		if ts.Release != "" {
			testSuiteReleases[ts.Release] = true
		}
	}

	for _, rel := range g.NodesByKind(KindRelease) {
		version := rel.Release
		if version == "" {
			continue
		}
		hasUCs := false
		for _, r := range corpus.Roadmap.Releases {
			if r.Version == version && len(r.UseCases) > 0 {
				hasUCs = true
				break
			}
		}
		if hasUCs && !testSuiteReleases[version] {
			findings = append(findings, Finding{
				Check:   "release-without-test-suite",
				Level:   "warning",
				Message: fmt.Sprintf("release %s has use cases but no test suite", version),
			})
		}
	}
	return findings
}

// checkMachineActionResolution verifies that every transition action
// references a tool listed in the agent's tool selection file.
func checkMachineActionResolution(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		selected := corpus.ToolSelections[agentName]
		if len(selected) == 0 {
			continue
		}
		toolSet := make(map[string]bool, len(selected))
		for _, t := range selected {
			toolSet[t] = true
		}
		for _, tr := range ms.Transitions {
			if tr.Action == "" {
				continue
			}
			if tr.Action == "$tool" {
				continue
			}
			if !toolSet[tr.Action] {
				findings = append(findings, Finding{
					Check:   "machine-unresolved-action",
					Level:   "error",
					Message: fmt.Sprintf("machine %s transition %s+%s action %q not in tool selection", agentName, tr.State, tr.Signal, tr.Action),
				})
			}
		}
	}
	return findings
}

// checkMachineSignalCoverage verifies that every declared signal is
// received by at least one transition.
func checkMachineSignalCoverage(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		received := make(map[string]bool)
		for _, tr := range ms.Transitions {
			received[tr.Signal] = true
		}
		for _, sig := range ms.Signals {
			if !received[sig.Name] {
				findings = append(findings, Finding{
					Check:   "machine-unreceived-signal",
					Level:   "warning",
					Message: fmt.Sprintf("machine %s signal %q is declared but no transition receives it", agentName, sig.Name),
				})
			}
		}
	}
	return findings
}

// checkMachineStateMetadata flags machines where some states have
// meaning annotations but others do not.
func checkMachineStateMetadata(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		if len(ms.States) == 0 {
			continue
		}
		withMeaning := 0
		for _, st := range ms.States {
			if st.Meaning != "" {
				withMeaning++
			}
		}
		if withMeaning > 0 && withMeaning < len(ms.States) {
			var missing []string
			for _, st := range ms.States {
				if st.Meaning == "" {
					missing = append(missing, st.Name)
				}
			}
			findings = append(findings, Finding{
				Check:   "machine-incomplete-state-metadata",
				Level:   "warning",
				Message: fmt.Sprintf("machine %s has %d/%d states with meaning; missing: %s", agentName, withMeaning, len(ms.States), strings.Join(missing, ", ")),
			})
		}
	}
	return findings
}

// checkMachineSignalMetadata flags machines where some signals have
// trigger annotations but others do not.
func checkMachineSignalMetadata(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		if len(ms.Signals) == 0 {
			continue
		}
		withTrigger := 0
		for _, sig := range ms.Signals {
			if sig.Trigger != "" {
				withTrigger++
			}
		}
		if withTrigger > 0 && withTrigger < len(ms.Signals) {
			var missing []string
			for _, sig := range ms.Signals {
				if sig.Trigger == "" {
					missing = append(missing, sig.Name)
				}
			}
			findings = append(findings, Finding{
				Check:   "machine-incomplete-signal-metadata",
				Level:   "warning",
				Message: fmt.Sprintf("machine %s has %d/%d signals with trigger; missing: %s", agentName, withTrigger, len(ms.Signals), strings.Join(missing, ", ")),
			})
		}
	}
	return findings
}

// checkToolSelectionDeclared verifies that every tool in a selection file
// has a corresponding declaration.
func checkToolSelectionDeclared(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range sortedToolSelectionKeys(corpus.ToolSelections) {
		selected := corpus.ToolSelections[agentName]
		for _, toolName := range selected {
			if _, ok := corpus.ToolDeclarations[toolName]; !ok {
				findings = append(findings, Finding{
					Check:   "tool-selection-undeclared",
					Level:   "error",
					Message: fmt.Sprintf("agent %s selects tool %q which has no declaration", agentName, toolName),
				})
			}
		}
	}
	return findings
}

// checkSelectedToolContractCompleteness enforces the Grammar Machine word
// contract for tools that are selected by active machine/profile configuration.
func checkSelectedToolContractCompleteness(corpus *Corpus) []Finding {
	consumers := selectedToolConsumers(corpus)
	var findings []Finding
	for _, toolName := range sortedKeys(consumers) {
		td, ok := corpus.ToolDeclarations[toolName]
		if !ok {
			continue
		}
		missing := missingToolContractFields(td)
		if len(missing) == 0 {
			continue
		}
		level := "error"
		if td.Contract == "legacy" {
			level = "warning"
		}
		findings = append(findings, Finding{
			Check: "tool-contract-incomplete",
			Level: level,
			Message: fmt.Sprintf(
				"selected tool %q from %s used by %s is missing contract fields: %s",
				toolName,
				sourceOrUnknown(td.SourceFile),
				strings.Join(consumers[toolName], ", "),
				strings.Join(missing, ", "),
			),
		})
	}
	return findings
}

func selectedToolConsumers(corpus *Corpus) map[string][]string {
	consumers := make(map[string][]string)
	for _, selectionName := range sortedToolSelectionKeys(corpus.ToolSelections) {
		seenInSelection := make(map[string]bool)
		for _, toolName := range corpus.ToolSelections[selectionName] {
			if toolName == "" || seenInSelection[toolName] {
				continue
			}
			seenInSelection[toolName] = true
			consumers[toolName] = append(consumers[toolName], selectionName)
		}
	}
	return consumers
}

func missingToolContractFields(td ToolDeclaration) []string {
	checks := []struct {
		field   string
		present bool
	}{
		{"category", td.Category != ""},
		{"problem", td.Problem != ""},
		{"goals", len(td.Goals) > 0},
		{"requirements.input", len(td.Requirements.Input) > 0},
		{"requirements.output", len(td.Requirements.Output) > 0},
		{"requirements.errors", len(td.Requirements.Errors) > 0},
		{"non_goals", len(td.NonGoals) > 0},
		{"emits", len(td.Emits) > 0},
		{"output.schema", len(td.Output.Schema) > 0},
		{"side_effects", len(td.SideEffects.Items) > 0},
		{"reversibility.classification", td.Reversibility.Classification != ""},
		{"undo.strategy", td.Undo.Strategy != ""},
		{"errors", len(td.Errors) > 0},
		{"relationships", len(td.Relationships.Before) > 0 || len(td.Relationships.After) > 0 || len(td.Relationships.Overlaps) > 0},
	}
	missing := make([]string, 0, len(checks))
	for _, check := range checks {
		if !check.present {
			missing = append(missing, check.field)
		}
	}
	return missing
}

func sortedToolSelectionKeys(selections map[string][]string) []string {
	keys := make([]string, 0, len(selections))
	for key := range selections {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sourceOrUnknown(source string) string {
	if source == "" {
		return "<unknown source>"
	}
	return source
}

// checkToolEmitsSignalSet verifies that tool emits signals are valid
// signal names in the machine that uses the tool.
func checkToolEmitsSignalSet(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		signalSet := make(map[string]bool, len(ms.Signals))
		for _, sig := range ms.Signals {
			signalSet[sig.Name] = true
		}
		selected := corpus.ToolSelections[agentName]
		for _, toolName := range selected {
			td, ok := corpus.ToolDeclarations[toolName]
			if !ok {
				continue
			}
			for _, emitted := range td.Emits {
				if !signalSet[emitted] {
					findings = append(findings, Finding{
						Check:   "tool-emits-unknown-signal",
						Level:   "warning",
						Message: fmt.Sprintf("agent %s tool %q emits signal %q not in machine signal set", agentName, toolName, emitted),
					})
				}
			}
		}
	}
	return findings
}

// checkToolUndoConsistency verifies that undo strategy aligns with
// reversibility classification.
func checkToolUndoConsistency(corpus *Corpus) []Finding {
	var findings []Finding
	for name, td := range corpus.ToolDeclarations {
		rev := td.Reversibility.Classification
		strat := td.Undo.Strategy
		if rev != "" && strat != "" {
			switch rev {
			case "irreversible":
				if strat != "irreversible" {
					findings = append(findings, Finding{
						Check:   "tool-undo-mismatch",
						Level:   "warning",
						Message: fmt.Sprintf("tool %q reversibility is %q but undo strategy is %q", name, rev, strat),
					})
				}
			case "reversible":
				if strat != "noop" && strat != "reversible" && strat != "snapshot_restore" {
					findings = append(findings, Finding{
						Check:   "tool-undo-mismatch",
						Level:   "warning",
						Message: fmt.Sprintf("tool %q reversibility is %q but undo strategy is %q", name, rev, strat),
					})
				}
			case "compensatable":
				if strat != "compensatable" && strat != "boundary_compensation" {
					findings = append(findings, Finding{
						Check:   "tool-undo-mismatch",
						Level:   "warning",
						Message: fmt.Sprintf("tool %q reversibility is %q but undo strategy is %q", name, rev, strat),
					})
				}
			}
		}
		if td.Undo.Payload != "" && len(td.Undo.Captures) == 0 {
			findings = append(findings, Finding{
				Check:   "tool-undo-payload-no-captures",
				Level:   "warning",
				Message: fmt.Sprintf("tool %q has undo payload %q but no captures listed", name, td.Undo.Payload),
			})
		}
	}
	return findings
}

// checkToolSideEffectVocab verifies that side_effects kind values use
// the known vocabulary.
func checkToolSideEffectVocab(corpus *Corpus) []Finding {
	var findings []Finding
	for name, td := range corpus.ToolDeclarations {
		for _, se := range td.SideEffects.Items {
			if se.Kind != "" && !KnownSideEffectKinds[se.Kind] {
				findings = append(findings, Finding{
					Check:   "tool-unknown-side-effect-kind",
					Level:   "error",
					Message: fmt.Sprintf("tool %q side_effects kind %q not in known vocabulary", name, se.Kind),
				})
			}
		}
	}
	return findings
}

// checkToolBoundaryCategory verifies that tools with boundary-class
// side effects declare category: boundary.
func checkToolBoundaryCategory(corpus *Corpus) []Finding {
	boundaryKinds := map[string]bool{
		"child_agent_execution":    true,
		"child_process":            true,
		"nested_machine_execution": true,
		"external_api":             true,
		"external_api_call":        true,
		"human_boundary":           true,
	}
	var findings []Finding
	for name, td := range corpus.ToolDeclarations {
		hasBoundarySE := false
		for _, se := range td.SideEffects.Items {
			if boundaryKinds[se.Kind] {
				hasBoundarySE = true
				break
			}
		}
		if hasBoundarySE && td.Category != "boundary" {
			findings = append(findings, Finding{
				Check:   "tool-boundary-category-missing",
				Level:   "warning",
				Message: fmt.Sprintf("tool %q has boundary side effects but category is %q, expected %q", name, td.Category, "boundary"),
			})
		}
	}
	return findings
}

// checkMachineNameConsistency verifies that the machine.yaml name field
// matches the agent directory name.
func checkMachineNameConsistency(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		if ms.Name != agentName && !strings.HasPrefix(ms.Name, agentName+"-") {
			findings = append(findings, Finding{
				Check:   "machine-name-mismatch",
				Level:   "error",
				Message: fmt.Sprintf("machine %s directory name does not match spec name %q", agentName, ms.Name),
			})
		}
	}
	return findings
}

// checkUseCaseIndexRefs verifies that every use_case_index entry in
// SPECIFICATIONS.yaml references a use case that exists in the corpus.
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
func checkMachineDiagnostics(corpus *Corpus) []Finding {
	var findings []Finding
	for _, agentName := range corpus.MachineOrder {
		ms := corpus.Machines[agentName]
		for _, diag := range core.DiagnoseMachineSpec(ms) {
			level := "warning"
			if diag.Severity == core.MachineDiagnosticWarning {
				level = "warning"
			}
			findings = append(findings, Finding{
				Check:   "machine-diagnostic-" + diag.Code,
				Level:   level,
				Message: fmt.Sprintf("machine %s: %s", agentName, diag.Message),
			})
		}
	}
	return findings
}

// Errors returns only error-level findings.
func Errors(findings []Finding) []Finding {
	var errs []Finding
	for _, f := range findings {
		if f.Level == "error" {
			errs = append(errs, f)
		}
	}
	return errs
}

// Warnings returns only warning-level findings.
func Warnings(findings []Finding) []Finding {
	var warns []Finding
	for _, f := range findings {
		if f.Level == "warning" {
			warns = append(warns, f)
		}
	}
	return warns
}

// FormatFindings produces a sorted human-readable report.
func FormatFindings(findings []Finding) string {
	if len(findings) == 0 {
		return "All consistency checks passed.\n"
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Level != findings[j].Level {
			return findings[i].Level == "error"
		}
		if findings[i].Check != findings[j].Check {
			return findings[i].Check < findings[j].Check
		}
		return findings[i].Message < findings[j].Message
	})

	var b strings.Builder
	currentCheck := ""
	for _, f := range findings {
		if f.Check != currentCheck {
			currentCheck = f.Check
			fmt.Fprintf(&b, "\n[%s] %s:\n", f.Level, f.Check)
		}
		fmt.Fprintf(&b, "  - %s\n", f.Message)
	}
	return b.String()
}
