// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func loadTestGraphAndCorpus(t *testing.T) (*Graph, *Corpus) {
	t.Helper()
	c, err := LoadCorpus(filepath.Join("testdata", "valid"))
	require.NoError(t, err)
	g, err := BuildGraph(c)
	require.NoError(t, err)
	return g, c
}

func TestValidate_ReturnsFindings(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)
	findings := Validate(g, c)

	assert.NotEmpty(t, findings, "fixture corpus has known issues (orphaned SRD, uncovered items)")

	byCheck := make(map[string]int)
	for _, f := range findings {
		byCheck[f.Check]++
	}

	assert.Greater(t, byCheck["orphaned-srd"], 0, "srd003-storage has no UC touchpoint")
	assert.Greater(t, byCheck["uncovered-req-item"], 0, "some items lack AC coverage")
}

func TestValidate_OrphanedSRD(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	findings := checkOrphanedSRDs(g)

	orphanedIDs := make(map[string]bool)
	for _, f := range findings {
		if f.Check == "orphaned-srd" {
			for _, srd := range g.NodesByKind(KindSRD) {
				if contains(f.Message, srd.ID) {
					orphanedIDs[srd.ID] = true
				}
			}
		}
	}

	assert.False(t, orphanedIDs["srd001-auth"], "srd001-auth is referenced by use case")
	assert.False(t, orphanedIDs["srd002-api"], "srd002-api is referenced by use case")
	assert.True(t, orphanedIDs["srd003-storage"], "srd003-storage has no use case touchpoint")

	_ = c
}

func TestValidate_UncoveredReqItems(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	findings := checkUncoveredReqItems(g)

	var uncovered []string
	for _, f := range findings {
		uncovered = append(uncovered, f.Message)
	}

	assert.NotEmpty(t, uncovered, "some req items should be uncovered (srd002, srd003 ACs don't trace all items)")
}

func TestValidate_UncoveredACs(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	findings := checkUncoveredACs(g)

	var uncoveredIDs []string
	for _, f := range findings {
		uncoveredIDs = append(uncoveredIDs, f.Message)
	}

	for _, f := range findings {
		assert.NotContains(t, f.Message, "srd001-auth:AC1",
			"AC1 and AC2 are covered by test cases")
	}
}

func TestValidate_TestSuiteCoversUseCase(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	findings := checkOrphanedTestSuites(g)
	assert.Empty(t, findings, "test-rel00.0 traces rel00.0-uc001-login which exists")
}

func TestValidate_BareTouchpoints(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	findings := checkBareTouchpoints(g, c)
	assert.Empty(t, findings, "all touchpoints in fixture specify R-groups")
}

func TestValidate_ReleasesWithoutTestSuites(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	findings := checkReleasesWithoutTestSuites(g, c)
	for _, f := range findings {
		assert.Contains(t, f.Message, "00.1", "release 00.1 has no test suite in fixture; 00.0 has test-rel00.0")
	}
}

func TestValidate_FormatFindings(t *testing.T) {
	findings := []Finding{
		{Check: "orphaned-srd", Level: "warning", Message: "SRD srd003-storage not referenced"},
		{Check: "broken-citation", Level: "error", Message: "use case uc1 cites missing group"},
	}

	output := FormatFindings(findings)
	assert.Contains(t, output, "[error] broken-citation")
	assert.Contains(t, output, "[warning] orphaned-srd")
}

func TestValidate_FormatEmpty(t *testing.T) {
	output := FormatFindings(nil)
	assert.Contains(t, output, "All consistency checks passed")
}

func TestValidate_ErrorsAndWarnings(t *testing.T) {
	findings := []Finding{
		{Check: "a", Level: "error", Message: "e1"},
		{Check: "b", Level: "warning", Message: "w1"},
		{Check: "c", Level: "error", Message: "e2"},
	}
	assert.Len(t, Errors(findings), 2)
	assert.Len(t, Warnings(findings), 1)
}

func TestValidate_MachineNodesInGraph(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	require.Contains(t, c.Machines, "test-agent", "test fixture should have a test-agent machine")

	machineNodes := g.NodesByKind(KindMachine)
	require.NotEmpty(t, machineNodes, "graph should contain machine nodes")

	var found bool
	for _, n := range machineNodes {
		if n.Machine == "test-agent" {
			found = true
			break
		}
	}
	assert.True(t, found, "graph should contain test-agent machine node")

	stateNodes := g.NodesByKind(KindMachineState)
	assert.GreaterOrEqual(t, len(stateNodes), 4, "test-agent has 4 states")

	signalNodes := g.NodesByKind(KindMachineSignal)
	assert.GreaterOrEqual(t, len(signalNodes), 3, "test-agent has 3 signals")

	transitionNodes := g.NodesByKind(KindMachineTransition)
	assert.GreaterOrEqual(t, len(transitionNodes), 3, "test-agent has 3 transitions")
}

func TestValidate_MachineContainsEdges(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	outgoing := g.OutgoingByRel("machine:test-agent", RelContains)
	assert.GreaterOrEqual(t, len(outgoing), 10, "machine should contain states + signals + transitions")
}

func TestValidate_MachineActionResolution(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"good": {
				Name:         "good",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Done"}},
				Signals:      core.SignalSpecs{{Name: "Seed"}},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Done", Action: "work"},
				},
				TerminalStates: []string{"Done"},
			},
			"bad": {
				Name:         "bad",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Done"}},
				Signals:      core.SignalSpecs{{Name: "Seed"}},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Done", Action: "missing_tool"},
				},
				TerminalStates: []string{"Done"},
			},
		},
		ToolSelections: map[string][]string{
			"good": {"work"},
			"bad":  {"other_tool"},
		},
		MachineOrder: []string{"bad", "good"},
	}

	findings := checkMachineActionResolution(corpus)

	var messages []string
	for _, f := range findings {
		messages = append(messages, f.Message)
		assert.Equal(t, "error", f.Level)
	}
	assert.Len(t, findings, 1, "only 'bad' should have an unresolved action")
	assert.Contains(t, findings[0].Message, "missing_tool")
}

func TestValidate_MachineSignalCoverage(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"sig-test": {
				Name:         "sig-test",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Done"}},
				Signals: core.SignalSpecs{
					{Name: "Seed"},
					{Name: "Orphan"},
				},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Done"},
				},
				TerminalStates: []string{"Done"},
			},
		},
		MachineOrder: []string{"sig-test"},
	}

	findings := checkMachineSignalCoverage(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "Orphan")
	assert.Equal(t, "warning", findings[0].Level)
}

func TestValidate_MachineStateMetadata(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"partial": {
				Name:         "partial",
				InitialState: "Idle",
				States: core.StateSpecs{
					{Name: "Idle", Meaning: "start"},
					{Name: "Done"},
				},
				Signals:        core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Done"}},
				TerminalStates: []string{"Done"},
			},
		},
		MachineOrder: []string{"partial"},
	}

	findings := checkMachineStateMetadata(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "Done")
	assert.Equal(t, "warning", findings[0].Level)
}

func TestValidate_ToolSelectionDeclared(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name: "agent", InitialState: "Idle",
				States: core.StateSpecs{{Name: "Idle"}}, Signals: core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
		},
		ToolSelections:   map[string][]string{"agent": {"exists", "missing"}},
		ToolDeclarations: map[string]ToolDeclaration{"exists": {Name: "exists"}},
		MachineOrder:     []string{"agent"},
	}

	findings := checkToolSelectionDeclared(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "error", findings[0].Level)
	assert.Contains(t, findings[0].Message, "missing")
}

func TestValidate_ToolEmitsSignalSet(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name: "agent", InitialState: "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}, {Name: "ToolDone"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
		},
		ToolSelections: map[string][]string{"agent": {"work"}},
		ToolDeclarations: map[string]ToolDeclaration{
			"work": {Name: "work", Emits: []string{"ToolDone", "UnknownSignal"}},
		},
		MachineOrder: []string{"agent"},
	}

	findings := checkToolEmitsSignalSet(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "UnknownSignal")
}

func TestValidate_ToolUndoConsistency(t *testing.T) {
	corpus := &Corpus{
		ToolDeclarations: map[string]ToolDeclaration{
			"good": {
				Name:          "good",
				Reversibility: ToolDeclReversibility{Classification: "reversible"},
				Undo:          ToolDeclUndo{Strategy: "noop"},
			},
			"bad": {
				Name:          "bad",
				Reversibility: ToolDeclReversibility{Classification: "irreversible"},
				Undo:          ToolDeclUndo{Strategy: "compensatable"},
			},
			"payload-no-captures": {
				Name: "payload-no-captures",
				Undo: ToolDeclUndo{Strategy: "compensatable", Payload: "boundary_compensation"},
			},
		},
	}

	findings := checkToolUndoConsistency(corpus)
	var checks []string
	for _, f := range findings {
		checks = append(checks, f.Check)
	}
	assert.Contains(t, checks, "tool-undo-mismatch")
	assert.Contains(t, checks, "tool-undo-payload-no-captures")
}

func TestValidate_ToolSideEffectVocab(t *testing.T) {
	corpus := &Corpus{
		ToolDeclarations: map[string]ToolDeclaration{
			"good": {
				Name:        "good",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "filesystem_read"}}},
			},
			"bad": {
				Name:        "bad",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "invented_kind"}}},
			},
		},
	}

	findings := checkToolSideEffectVocab(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "error", findings[0].Level)
	assert.Contains(t, findings[0].Message, "invented_kind")
}

func TestValidate_ToolBoundaryCategory(t *testing.T) {
	corpus := &Corpus{
		ToolDeclarations: map[string]ToolDeclaration{
			"correct": {
				Name:        "correct",
				Category:    "boundary",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "child_agent_execution"}}},
			},
			"wrong": {
				Name:        "wrong",
				Category:    "word",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "external_api"}}},
			},
		},
	}

	findings := checkToolBoundaryCategory(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "wrong")
	assert.Contains(t, findings[0].Message, "boundary")
}

func TestValidate_ToolDeclNodesInGraph(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	require.Contains(t, c.ToolDeclarations, "do_work")

	declNodes := g.NodesByKind(KindToolDecl)
	require.NotEmpty(t, declNodes)

	var found bool
	for _, n := range declNodes {
		if n.ID == "tool-decl:do_work" {
			found = true
			break
		}
	}
	assert.True(t, found, "graph should contain tool-decl:do_work node")
}

func TestValidate_ActionResolvesEdges(t *testing.T) {
	g, _ := loadTestGraphAndCorpus(t)

	resolvesEdges := g.EdgesByRel(RelResolves)
	assert.NotEmpty(t, resolvesEdges, "transition actions should resolve to tool declarations")
}

func TestValidate_UseCaseIndexRefs(t *testing.T) {
	corpus := &Corpus{
		SpecIndex: SpecIndex{
			UseCaseIndex: []UseCaseEntry{
				{ID: "exists"},
				{ID: "missing"},
			},
		},
		UseCases: map[string]UseCase{"exists": {ID: "exists"}},
	}
	findings := checkUseCaseIndexRefs(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "missing")
	assert.Equal(t, "error", findings[0].Level)
}

func TestValidate_TestSuiteIndexRefs(t *testing.T) {
	corpus := &Corpus{
		SpecIndex: SpecIndex{
			TestSuiteIndex: []TestSuiteEntry{
				{ID: "exists"},
				{ID: "missing"},
			},
		},
		TestSuites: map[string]TestSuite{"exists": {ID: "exists"}},
	}
	findings := checkTestSuiteIndexRefs(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "missing")
}

func TestValidate_RoadmapUseCaseRefs(t *testing.T) {
	corpus := &Corpus{
		Roadmap: Roadmap{
			Releases: []Release{
				{Version: "1.0", UseCases: []UseCaseRef{{ID: "exists"}, {ID: "missing"}}},
			},
		},
		UseCases: map[string]UseCase{"exists": {ID: "exists"}},
	}
	findings := checkRoadmapUseCaseRefs(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "missing")
}

func TestValidate_UseCaseTestSuiteReciprocity(t *testing.T) {
	corpus := &Corpus{
		UseCases: map[string]UseCase{
			"uc1": {ID: "uc1", TestSuite: "ts-good"},
			"uc2": {ID: "uc2", TestSuite: "ts-no-trace"},
			"uc3": {ID: "uc3", TestSuite: "ts-missing"},
		},
		TestSuites: map[string]TestSuite{
			"ts-good":     {ID: "ts-good", Traces: []string{"uc1"}},
			"ts-no-trace": {ID: "ts-no-trace", Traces: []string{"other"}},
		},
		UCOrder: []string{"uc1", "uc2", "uc3"},
	}
	findings := checkUseCaseTestSuiteReciprocity(corpus)
	require.Len(t, findings, 2)

	var checks []string
	for _, f := range findings {
		checks = append(checks, f.Check)
	}
	assert.Contains(t, checks, "use-case-missing-test-suite")
	assert.Contains(t, checks, "test-suite-missing-uc-trace")
}

func TestValidate_SpecIndexPaths(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)
	_ = g

	findings := checkSpecIndexPaths(c)
	for _, f := range findings {
		t.Logf("finding: %s", f.Message)
	}
	assert.Empty(t, findings, "all fixture paths should exist")
}

func TestValidate_SpecIndexPaths_Broken(t *testing.T) {
	corpus := &Corpus{
		RootDir: t.TempDir(),
		SpecIndex: SpecIndex{
			SRDIndex: []SRDEntry{
				{ID: "srd-bad", Path: "docs/nonexistent.yaml"},
			},
		},
	}
	findings := checkSpecIndexPaths(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "index-broken-path", findings[0].Check)
}

func TestValidate_FixtureIndexConsistency(t *testing.T) {
	g, c := loadTestGraphAndCorpus(t)

	ucFindings := checkUseCaseIndexRefs(c)
	assert.Empty(t, ucFindings, "fixture UC index should be consistent")

	tsFindings := checkTestSuiteIndexRefs(c)
	assert.Empty(t, tsFindings, "fixture TS index should be consistent")

	_ = g
}

func TestValidate_DocSpecsLoaded(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)

	require.Contains(t, c.DocSpecs, "sm-test-model")
	require.Contains(t, c.DocSpecs, "cfg-test-format")

	sm := c.DocSpecs["sm-test-model"]
	assert.Equal(t, "Test Semantic Model", sm.Title)
	assert.Len(t, sm.RequirementsSource.Canonical, 1)
	assert.Contains(t, sm.RelatedDocuments, "cfg-test-format")

	cf := c.DocSpecs["cfg-test-format"]
	assert.Len(t, cf.Implementation.Paths, 1)
	assert.Len(t, cf.Examples, 1)
}

func TestValidate_DocSpecRequirementsSources(t *testing.T) {
	corpus := &Corpus{
		RootDir: t.TempDir(),
		DocSpecs: map[string]DocSpec{
			"bad": {
				ID: "bad",
				RequirementsSource: DocSpecSources{
					Canonical: []string{"docs/nonexistent.yaml"},
				},
			},
		},
	}
	findings := checkDocSpecRequirementsSources(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "docspec-broken-requirement-source", findings[0].Check)
}

func TestValidate_DocSpecRequirementsSources_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkDocSpecRequirementsSources(c)
	assert.Empty(t, findings, "fixture canonical sources should exist")
}

func TestValidate_DocSpecRelatedDocuments(t *testing.T) {
	corpus := &Corpus{
		SRDs: map[string]SRD{"srd001-auth": {ID: "srd001-auth"}},
		DocSpecs: map[string]DocSpec{
			"spec-a": {
				ID:               "spec-a",
				RelatedDocuments: []string{"srd001-auth", "unknown-ref"},
			},
		},
	}
	findings := checkDocSpecRelatedDocuments(corpus)
	require.Len(t, findings, 1)
	assert.Contains(t, findings[0].Message, "unknown-ref")
}

func TestValidate_DocSpecRelatedDocuments_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkDocSpecRelatedDocuments(c)
	assert.Empty(t, findings, "fixture related documents should all resolve")
}

func TestValidate_DocSpecImplementationPaths(t *testing.T) {
	corpus := &Corpus{
		RootDir: t.TempDir(),
		DocSpecs: map[string]DocSpec{
			"bad": {
				ID:             "bad",
				Implementation: DocSpecImpl{Paths: []string{"pkg/nonexistent.go"}},
			},
		},
	}
	findings := checkDocSpecImplementationPaths(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "docspec-broken-implementation-path", findings[0].Check)
}

func TestValidate_DocSpecImplementationPaths_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkDocSpecImplementationPaths(c)
	assert.Empty(t, findings, "fixture implementation paths should exist")
}

func TestValidate_DocSpecExamplePaths(t *testing.T) {
	corpus := &Corpus{
		RootDir: t.TempDir(),
		DocSpecs: map[string]DocSpec{
			"bad": {
				ID:       "bad",
				Examples: []DocSpecExample{{File: "nonexistent/file.yaml"}},
			},
		},
	}
	findings := checkDocSpecExamplePaths(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "docspec-broken-example-path", findings[0].Check)
}

func TestValidate_DocSpecExamplePaths_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkDocSpecExamplePaths(c)
	assert.Empty(t, findings, "fixture example paths should exist")
}

func TestValidate_MachineDiagnostics(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"diag-test": {
				Name:         "diag-test",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Done"}, {Name: "Orphan"}},
				Signals: core.SignalSpecs{
					{Name: "Seed"},
					{Name: "Unused"},
				},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Done"},
				},
				TerminalStates: []string{"Done"},
			},
		},
		MachineOrder: []string{"diag-test"},
	}
	findings := checkMachineDiagnostics(corpus)
	require.NotEmpty(t, findings)

	var codes []string
	for _, f := range findings {
		codes = append(codes, f.Check)
	}
	assert.Contains(t, codes, "machine-diagnostic-unreachable_state")
	assert.Contains(t, codes, "machine-diagnostic-unused_signal")
}

func TestValidate_MachineDiagnostics_Fixture(t *testing.T) {
	_, c := loadTestGraphAndCorpus(t)
	findings := checkMachineDiagnostics(c)
	assert.Empty(t, findings, "fixture machines should have no diagnostics")
}

func TestValidate_MachineNameConsistency(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"dir-name": {
				Name:           "wrong-name",
				InitialState:   "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
		},
		MachineOrder: []string{"dir-name"},
	}

	findings := checkMachineNameConsistency(corpus)
	require.Len(t, findings, 1)
	assert.Equal(t, "error", findings[0].Level)
	assert.Contains(t, findings[0].Message, "wrong-name")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
