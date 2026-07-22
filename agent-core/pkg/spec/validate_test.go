// Copyright (c) 2026 Nokia. All rights reserved.

package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
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

func TestPackageProductionFilesStaySplit(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		require.NotEqual(t, "types.go", entry.Name())
		data, err := os.ReadFile(entry.Name())
		require.NoError(t, err)
		lines := strings.Count(string(data), "\n")
		require.LessOrEqual(t, lines, 500, "%s exceeds production file limit", entry.Name())
	}
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

// TestValidate_ObjectTouchpointNotOrphaned proves the GH-448 fix end to end: a use
// case whose touchpoint uses the object form ({id, target, reason}) -- as the
// example corpora author them -- builds a touches edge and an acceptance-criterion
// citation, so the SRD is neither orphaned nor flagged as a bare touchpoint.
func TestValidate_ObjectTouchpointNotOrphaned(t *testing.T) {
	corpus := &Corpus{
		SRDs: map[string]SRD{
			"srd004-coordinator": {
				ID:                 "srd004-coordinator",
				AcceptanceCriteria: []AcceptanceCriterion{{ID: "AC1", Criterion: "The coordinator binds the intent."}},
			},
		},
		UseCases: map[string]UseCase{
			"rel05.0-uc001": {
				ID: "rel05.0-uc001",
				// The parser has already folded the {id, target, reason} object into
				// this canonical string (see TestParseUseCase_ObjectTouchpoints).
				Touchpoints: []string{"srd004-coordinator AC1 -- The coordinator binds the intent."},
			},
		},
		SRDOrder: []string{"srd004-coordinator"},
		UCOrder:  []string{"rel05.0-uc001"},
	}

	g, err := BuildGraph(corpus)
	require.NoError(t, err)

	assert.Contains(t, g.OutgoingByRel("rel05.0-uc001", RelTouches), "srd004-coordinator",
		"the object-form touchpoint must build a touches edge")
	assert.Contains(t, g.OutgoingByRel("rel05.0-uc001", RelCites), "srd004-coordinator:AC1",
		"the cited acceptance criterion must build a cites edge")
	assert.Empty(t, checkOrphanedSRDs(g), "the touched SRD must not be orphaned")
	assert.Empty(t, checkBareTouchpoints(g, corpus), "an AC-citing touchpoint is not bare")
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
		assert.Contains(t, f.Message, "00.1", "the fixture keeps 00.1 without a test suite; 00.0 has test-rel00.0")
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

func TestValidate_FormatFindingsWithProvenance(t *testing.T) {
	findings := []Finding{
		{
			Level:   "warning",
			SuiteID: "paper-charter",
			CheckID: "citations-resolve",
			Kind:    "ref_check",
			File:    "paper/main.md",
			Line:    12,
			Message: "citation @missing does not resolve",
		},
		{
			Level:   "error",
			SuiteID: "paper-charter",
			CheckID: "no-internal-vocabulary",
			Kind:    "grep_check",
			File:    "paper/main.md",
			Line:    4,
			Message: "found forbidden term cobbler",
		},
	}

	output := FormatFindings(findings)

	assert.Contains(t, output, "[error] paper-charter/no-internal-vocabulary (grep_check):")
	assert.Contains(t, output, "  - paper/main.md:4: found forbidden term cobbler")
	assert.Contains(t, output, "[warning] paper-charter/citations-resolve (ref_check):")
	assert.Contains(t, output, "  - paper/main.md:12: citation @missing does not resolve")
	assert.Less(t, strings.Index(output, "[error]"), strings.Index(output, "[warning]"))
}

func TestValidate_FormatFindingsSortsDeterministicallyWithoutMutatingInput(t *testing.T) {
	findings := []Finding{
		{Level: "warning", SuiteID: "suite-b", CheckID: "b", File: "z.md", Line: 2, Message: "second"},
		{Level: "warning", SuiteID: "suite-a", CheckID: "a", File: "a.md", Line: 1, Message: "first"},
		{Level: "error", Check: "legacy-error", Message: "legacy"},
	}
	original := append([]Finding(nil), findings...)

	output := FormatFindings(findings)

	assert.Equal(t, original, findings, "FormatFindings must not reorder caller-owned slices")
	assert.Less(t, strings.Index(output, "[error] legacy-error"), strings.Index(output, "[warning] suite-a/a"))
	assert.Less(t, strings.Index(output, "[warning] suite-a/a"), strings.Index(output, "[warning] suite-b/b"))
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

	require.Len(t, findings, 1, "only 'bad' should have an unresolved action")
	for _, f := range findings {
		assert.Equal(t, "error", f.Level)
	}
	assert.Contains(t, findings[0].Message, "missing_tool")
}

func TestValidate_ToolMetricConfig(t *testing.T) {
	corpus := &Corpus{
		ToolDeclarations: map[string]ToolDeclaration{
			"valid": {
				Name: "valid",
				Metrics: core.MetricConfig{
					Instruments: []core.MetricInstrument{{
						Name:        "rest.request_duration",
						Kind:        "histogram",
						Description: "Duration.",
						ValueSource: "dispatch_duration",
					}},
				},
			},
			"bad": {
				Name: "bad",
				Metrics: core.MetricConfig{
					Instruments: []core.MetricInstrument{{
						Name:        "bad",
						Kind:        "summary",
						Description: "Invalid.",
						ValueSource: "dispatch_count",
					}},
				},
			},
		},
	}

	findings := checkToolMetricConfig(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "tool-metric-config-invalid", findings[0].Check)
	assert.Contains(t, findings[0].Message, "summary")
}

func TestValidate_MachineMetricLabels(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"bad-machine": {
				Name:         "bad-machine",
				MetricLabels: core.MetricLabels{"phase": "request_id"},
			},
		},
	}

	findings := checkMachineMetricLabels(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "machine-metric-label-invalid", findings[0].Check)
	assert.Contains(t, findings[0].Message, "request_id")
}

func TestValidate_MachineActionResolutionIgnoresDynamicTool(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name:         "agent",
				InitialState: "Idle",
				States:       core.StateSpecs{{Name: "Idle"}, {Name: "Dispatching"}},
				Signals:      core.SignalSpecs{{Name: "Seed"}, {Name: "ToolDone"}},
				Transitions: []core.TransitionSpec{
					{State: "Idle", Signal: "Seed", Next: "Dispatching", Action: "$tool"},
					{State: "Dispatching", Signal: "ToolDone", Next: "Dispatching"},
				},
				TerminalStates: []string{"Dispatching"},
			},
		},
		ToolSelections: map[string][]string{
			"agent": {"read"},
		},
		MachineOrder: []string{"agent"},
	}

	findings := checkMachineActionResolution(corpus)
	assert.Empty(t, findings, "$tool is dynamic dispatch and should not require a tool named $tool")
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

func TestValidate_SelectedToolContractCompletenessErrorsForActiveMigratedTool(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent": {"sparse"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"sparse": {
				Name:     "sparse",
				Contract: "migrated",
				Emits:    []string{"ToolDone"},
			},
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "tool-contract-incomplete", findings[0].Check)
	assert.Equal(t, "error", findings[0].Level)
	assert.Contains(t, findings[0].Message, "sparse")
	assert.Contains(t, findings[0].Message, "problem")
	assert.Contains(t, findings[0].Message, "output.schema")
}

func TestValidate_SelectedToolContractCompletenessKeepsLegacyWarningOnly(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent": {"legacy"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"legacy": {
				Name:     "legacy",
				Contract: "legacy",
				Emits:    []string{"ToolDone"},
			},
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Len(t, findings, 1)
	assert.Equal(t, "warning", findings[0].Level)
	assert.Contains(t, findings[0].Message, "legacy")
}

func TestValidate_SelectedToolContractCompletenessIgnoresUnselectedTool(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent": {"complete"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"complete": completeToolDeclaration("complete"),
			"sparse":   {Name: "sparse"},
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Empty(t, findings)
}

func TestValidate_SelectedToolContractCompletenessPassesCompleteTool(t *testing.T) {
	corpus := &Corpus{
		ToolSelections: map[string][]string{
			"agent":       {"complete"},
			"agent:point": {"complete"},
		},
		ToolDeclarations: map[string]ToolDeclaration{
			"complete": completeToolDeclaration("complete"),
		},
	}

	findings := checkSelectedToolContractCompleteness(corpus)

	require.Empty(t, findings)
}

func completeToolDeclaration(name string) ToolDeclaration {
	return ToolDeclaration{
		Name:     name,
		Category: "word",
		Problem:  "The machine needs a complete word contract for audit validation.",
		Goals:    []string{"Run as a declared machine word."},
		Requirements: ToolDeclRequirements{
			Input:  []string{"must accept declared input"},
			Output: []string{"must return declared output"},
			Errors: []string{"must report declared errors"},
		},
		NonGoals: []string{"Does not choose the next machine state."},
		Emits:    []string{"ToolDone"},
		Output: ToolDeclOutput{Schema: map[string]any{
			"type": "object",
		}},
		SideEffects:   ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "filesystem_read"}}},
		Reversibility: ToolDeclReversibility{Classification: "reversible"},
		Undo:          ToolDeclUndo{Strategy: "noop"},
		Errors:        []ToolDeclError{{Signal: "CommandError"}},
		Relationships: ToolDeclRelationships{After: []string{"next_word"}},
	}
}

func TestDiscoverToolDeclarationsIncludesProfileFilesAndDirs(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "executor", "llm"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools", "builtin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "executor", "profile.yaml"), []byte(`
name: generator
machine: machine.yaml
tools:
  - tools.yaml
tool_config_dirs:
  - ../../tools/builtin
tool_declarations:
  - llm/default.yaml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "executor", "llm", "default.yaml"), []byte(`
tools:
  - name: invoke_llm
    type: builtin
    init: invoke_llm
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools", "builtin", "read.yaml"), []byte(`
tools:
  - name: read
    type: builtin
    init: file_read
`), 0o644))

	decls, err := discoverAndParseToolDeclarations(root)

	require.NoError(t, err)
	require.Contains(t, decls, "invoke_llm")
	require.Contains(t, decls, "read")
}

func TestDiscoverToolDeclarationsAcceptsRESTSideEffectKinds(t *testing.T) {
	root := t.TempDir()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	restDecls := filepath.Join(repoRoot, "tools", "builtin", "rest", "all.yaml")

	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "rest"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "rest", "profile.yaml"), []byte(`
name: rest
tool_declarations:
  - `+restDecls+`
`), 0o644))

	decls, err := discoverAndParseToolDeclarations(root)

	require.NoError(t, err)
	require.Contains(t, decls, "rest_server_launch")
	require.Contains(t, decls, "rest_server_stop")
	require.Contains(t, decls, "rest_await_event")
	corpus := &Corpus{ToolDeclarations: decls}
	assert.Empty(t, checkToolSideEffectVocab(corpus))
	assert.Empty(t, checkToolBoundaryCategory(corpus))
}

func TestDiscoverToolDeclarationsKeepsAgentLocalOverrides(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "bench"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "tools", "builtin"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "bench", "profile.yaml"), []byte(`
name: bench
machine: machine.yaml
tools:
  - tools.yaml
tool_config_dirs:
  - ../../tools/builtin
tool_declarations:
  - builtin.yaml
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "bench", "builtin.yaml"), []byte(`
tools:
  - name: serve_ui
    type: builtin
    init: serve_ui
    problem: local override
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "tools", "builtin", "serve-ui.yaml"), []byte(`
tools:
  - name: serve_ui
    type: builtin
    init: serve_ui
    problem: shared compatibility declaration
`), 0o644))

	decls, err := discoverAndParseToolDeclarations(root)

	require.NoError(t, err)
	require.Contains(t, decls, "serve_ui")
	assert.Equal(t, "agents/bench/builtin.yaml", decls["serve_ui"].SourceFile)
	assert.Equal(t, "local override", decls["serve_ui"].Problem)
}

func TestDiscoverMachinesIncludesConfiguredToolSelections(t *testing.T) {
	root := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(root, "agents", "critic"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "critic", "machine.yaml"), []byte(`
name: critic-session
initial_state: Idle
terminal_states: [Done]
configuration:
  point_tools: agents/critic/tools-point.yaml
states:
  - Idle
  - Done
signals:
  - name: Seed
transitions:
  - state: Idle
    signal: Seed
    next: Done
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "critic", "tools.yaml"), []byte(`
tools:
  - parse_suite_config
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "agents", "critic", "tools-point.yaml"), []byte(`
tools:
  - create_point_dir
  - run_agent
`), 0o644))

	_, selections, _, err := discoverAndParseMachines(root)

	require.NoError(t, err)
	assert.Equal(t, []string{"parse_suite_config"}, selections["critic"])
	assert.Equal(t, []string{"create_point_dir", "run_agent"}, selections["critic:point_tools"])
}

func TestValidate_ToolEmitsSignalSet(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name: "agent", InitialState: "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}, {Name: "ToolDone"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle", Action: "work"}},
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

// TestValidate_ToolEmitsSignalSetSkipsUndispatchedTool checks that a tool the
// profile selects but the machine never dispatches — the REST or
// machine_request binding case — is not validated against the machine's signal
// set, since it routes its signals in a request-scoped sentence.
func TestValidate_ToolEmitsSignalSetSkipsUndispatchedTool(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name: "agent", InitialState: "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
		},
		ToolSelections: map[string][]string{"agent": {"rest_bound"}},
		ToolDeclarations: map[string]ToolDeclaration{
			"rest_bound": {Name: "rest_bound", Emits: []string{"RESTResponded"}},
		},
		MachineOrder: []string{"agent"},
	}

	assert.Empty(t, checkToolEmitsSignalSet(corpus))
}

// TestValidate_ToolEmitsSignalSetDynamicDispatch checks that a machine using
// $tool dynamic dispatch validates every selected tool, since it can invoke any
// of them.
func TestValidate_ToolEmitsSignalSetDynamicDispatch(t *testing.T) {
	corpus := &Corpus{
		Machines: map[string]core.MachineSpec{
			"agent": {
				Name: "agent", InitialState: "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}, {Name: "ToolDone"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle", Action: "$tool"}},
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
	decls := map[string]ToolDeclaration{
		"invalid-irreversible": {
			Name:          "invalid-irreversible",
			Reversibility: ToolDeclReversibility{Classification: "irreversible"},
			Undo:          ToolDeclUndo{Strategy: "compensating_action"},
		},
		"payload-no-captures": {
			Name: "payload-no-captures",
			Undo: ToolDeclUndo{Strategy: "compensating_action", Payload: "boundary_compensation"},
		},
	}
	for _, strategy := range []string{
		"noop",
		"workspace_restore",
		"session_state_restore",
		"conversation_truncate",
		"conversation_restore",
		"parse_retry_counter_restore",
		"pipeline_state_restore",
		"evaluator_session_restore",
		"point_context_restore",
		"validation_state_restore",
	} {
		decls["reversible-"+strategy] = ToolDeclaration{
			Name:          "reversible-" + strategy,
			Reversibility: ToolDeclReversibility{Classification: "reversible"},
			Undo:          ToolDeclUndo{Strategy: strategy},
		}
	}
	for _, strategy := range []string{
		"compensating_action",
		"child_command_undo",
		"workspace_restore",
		"child_agent_workspace_restore",
		"nested_machine_rollback",
		"point_workspace_restore_and_child_process_compensation",
		"child_eval_artifact_compensation",
		"server_shutdown_or_user_action_compensation",
	} {
		decls["compensatable-"+strategy] = ToolDeclaration{
			Name:          "compensatable-" + strategy,
			Reversibility: ToolDeclReversibility{Classification: "compensatable"},
			Undo:          ToolDeclUndo{Strategy: strategy},
		}
	}

	corpus := &Corpus{ToolDeclarations: decls}

	findings := checkToolUndoConsistency(corpus)
	var checks []string
	for _, f := range findings {
		checks = append(checks, f.Check)
	}
	assert.Equal(t, 1, countFindings(checks, "tool-undo-mismatch"))
	assert.Contains(t, checks, "tool-undo-payload-no-captures")
}

func countFindings(checks []string, check string) int {
	count := 0
	for _, candidate := range checks {
		if candidate == check {
			count++
		}
	}
	return count
}

func TestValidate_ToolSideEffectVocab(t *testing.T) {
	corpus := &Corpus{
		ToolDeclarations: map[string]ToolDeclaration{
			"good": {
				Name:        "good",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "filesystem_read"}}},
			},
			"rest_launch": {
				Name:        "rest_launch",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "network_listen"}}},
			},
			"rest_stop": {
				Name:        "rest_stop",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "network_listener_shutdown"}}},
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
			"listener": {
				Name:        "listener",
				Category:    "word",
				SideEffects: ToolDeclSideEffects{Items: []ToolDeclSideEffect{{Kind: "network_listen"}}},
			},
		},
	}

	findings := checkToolBoundaryCategory(corpus)
	require.Len(t, findings, 2)
	messages := []string{findings[0].Message, findings[1].Message}
	assert.Contains(t, messages, `tool "wrong" has boundary side effects but category is "word", expected "boundary"`)
	assert.Contains(t, messages, `tool "listener" has boundary side effects but category is "word", expected "boundary"`)
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
			"critic": {
				Name:           "critic-session",
				InitialState:   "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
			"dir-name": {
				Name:           "wrong-name",
				InitialState:   "Idle",
				States:         core.StateSpecs{{Name: "Idle"}},
				Signals:        core.SignalSpecs{{Name: "Seed"}},
				Transitions:    []core.TransitionSpec{{State: "Idle", Signal: "Seed", Next: "Idle"}},
				TerminalStates: []string{"Idle"},
			},
		},
		MachineOrder: []string{"critic", "dir-name"},
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
