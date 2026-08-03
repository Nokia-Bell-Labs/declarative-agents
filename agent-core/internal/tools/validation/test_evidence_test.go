// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

const (
	evidenceModule  = "example.test/fixture"
	evidencePackage = "example.test/fixture/subject"

	moduleSel   = "$from(go_module).output"
	packagesSel = "$from(go_packages).output"
	testsSel    = "$from(go_test_inventory).output"
	runSel      = "$from(go_test_run).output"
)

// claimTestClaimed is the suite fixture every command test shares: one live
// case claiming TestClaimed as its formal proof.
func claimTestClaimedSuites() map[string]spec.TestSuite {
	return map[string]spec.TestSuite{
		"test-rel09.0-example": {
			ID:        "test-rel09.0-example",
			TestCases: []spec.TestCase{{Name: "A claim", GoTest: "TestClaimed"}},
		},
	}
}

// viewFromPayloads builds a forward command-state view from label -> JSON output
// payloads, applying the same redaction boundary loop dispatch uses so selector
// resolution treats the outputs as readable.
func viewFromPayloads(payloads map[string]string) core.CommandStateView {
	exec := make(core.Execution, 0, len(payloads))
	for label, payload := range payloads {
		exec = append(exec, core.Entry{
			CommandName: label,
			Label:       label,
			Result:      core.DigestResult(core.Result{Output: payload}),
		})
	}
	return core.NewCommandStateView(exec)
}

func evidenceState(suites map[string]spec.TestSuite) *SpecState {
	return &SpecState{
		Directory: evidenceModule,
		Corpus:    &spec.Corpus{RootDir: evidenceModule, TestSuites: suites},
	}
}

func TestLoadTestClaimsBuilder(t *testing.T) {
	t.Parallel()

	writeSuiteDir := func(t *testing.T, body string) string {
		t.Helper()
		root := t.TempDir()
		dir := filepath.Join(root, spec.TSSubdir)
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "test-rel09.0-example.yaml"), []byte(body), 0o644))
		return root
	}

	t.Run("loads suites and seeds a corpus with an undo receipt", func(t *testing.T) {
		t.Parallel()
		root := writeSuiteDir(t, `id: test-rel09.0-example
title: Example
test_cases:
  - name: A claim
    go_test: TestClaimed
`)
		vs := &SpecState{Directory: root}
		builder := &LoadTestClaimsBuilder{VS: vs}
		cmd := builder.Build(core.Result{})

		result := cmd.Execute()
		require.Equal(t, core.ToolDone, result.Signal, result.Output)
		require.Contains(t, result.Output, "loaded 1 formal test suites")
		require.NotEmpty(t, result.Receipt)
		require.NotNil(t, vs.Corpus)
		require.Contains(t, vs.Corpus.TestSuites, "test-rel09.0-example")

		undo := cmd.Undo(result)
		require.Equal(t, core.ToolDone, undo.Signal, undo.Output)
		require.Nil(t, vs.Corpus, "undo clears the corpus load_test_claims created")
	})

	t.Run("malformed suite yields a command error without seeding a corpus", func(t *testing.T) {
		t.Parallel()
		root := writeSuiteDir(t, "id: [unterminated\n")
		vs := &SpecState{Directory: root}
		cmd := (&LoadTestClaimsBuilder{VS: vs}).Build(core.Result{})

		result := cmd.Execute()
		require.Equal(t, core.CommandError, result.Signal)
		require.Error(t, result.Err)
		require.Contains(t, result.Output, "load_test_claims failed")
		require.Nil(t, vs.Corpus, "a failed load must not seed a corpus")
	})
}

func TestResolveTestEvidenceBuilder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		payloads    map[string]string
		wantSignal  core.Signal
		wantErr     bool
		wantOutSub  string
		wantNoInv   bool // expect TestInventory left nil (failure path)
		wantFinding bool // ValidationFailed carries at least one finding
	}{
		{
			name: "resolves a present claim",
			payloads: map[string]string{
				"go_module":         outputPayloadStr(evidenceModule),
				"go_packages":       outputPayloadStr(evidencePackage + "\n"),
				"go_test_inventory": outputPayloadStr(mustListStream(evidencePackage, "TestClaimed")),
			},
			wantSignal: core.ValidationPassed,
		},
		{
			name: "missing symbol fails validation",
			payloads: map[string]string{
				"go_module":         outputPayloadStr(evidenceModule),
				"go_packages":       outputPayloadStr(evidencePackage + "\n"),
				"go_test_inventory": outputPayloadStr(mustListStream(evidencePackage, "TestOther")),
			},
			wantSignal:  core.ValidationFailed,
			wantFinding: true,
		},
		{
			name: "empty module output is a command error",
			payloads: map[string]string{
				"go_module":         outputPayloadStr(""),
				"go_packages":       outputPayloadStr(evidencePackage + "\n"),
				"go_test_inventory": outputPayloadStr(mustListStream(evidencePackage, "TestClaimed")),
			},
			wantSignal: core.CommandError,
			wantErr:    true,
			wantOutSub: "no module path",
			wantNoInv:  true,
		},
		{
			name: "empty package list is a command error",
			payloads: map[string]string{
				"go_module":         outputPayloadStr(evidenceModule),
				"go_packages":       outputPayloadStr(""),
				"go_test_inventory": outputPayloadStr(mustListStream(evidencePackage, "TestClaimed")),
			},
			wantSignal: core.CommandError,
			wantErr:    true,
			wantOutSub: "no packages",
			wantNoInv:  true,
		},
		{
			name: "unresolved selector label is a command error",
			payloads: map[string]string{
				"go_packages":       outputPayloadStr(evidencePackage + "\n"),
				"go_test_inventory": outputPayloadStr(mustListStream(evidencePackage, "TestClaimed")),
			},
			wantSignal: core.CommandError,
			wantErr:    true,
			wantOutSub: "go_module",
			wantNoInv:  true,
		},
		{
			name: "unresolved tests selector is a command error",
			payloads: map[string]string{
				"go_module":   outputPayloadStr(evidenceModule),
				"go_packages": outputPayloadStr(evidencePackage + "\n"),
			},
			wantSignal: core.CommandError,
			wantErr:    true,
			wantOutSub: "go_test_inventory",
			wantNoInv:  true,
		},
		{
			name: "non-string selector value is a command error",
			payloads: map[string]string{
				"go_module":         `{"output":123}`,
				"go_packages":       outputPayloadStr(evidencePackage + "\n"),
				"go_test_inventory": outputPayloadStr(mustListStream(evidencePackage, "TestClaimed")),
			},
			wantSignal: core.CommandError,
			wantErr:    true,
			wantOutSub: "want string",
			wantNoInv:  true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := evidenceState(claimTestClaimedSuites())
			cmd := (&ResolveTestEvidenceBuilder{
				VS: vs, ModuleFrom: moduleSel, PackagesFrom: packagesSel, TestsFrom: testsSel,
			}).Build(core.Result{})
			cmd.(core.CommandStateAware).SetCommandState(viewFromPayloads(tc.payloads))

			result := cmd.Execute()

			require.Equal(t, tc.wantSignal, result.Signal, result.Output)
			if tc.wantErr {
				require.Error(t, result.Err)
				require.Contains(t, result.Output, tc.wantOutSub)
			}
			if tc.wantNoInv {
				require.Nil(t, vs.TestInventory, "failed resolve must not mutate the inventory")
				require.Empty(t, vs.Findings, "failed resolve must not append findings")
			}
			if tc.wantFinding {
				require.NotEmpty(t, vs.Findings)
				require.Contains(t, findingMessages(vs.Findings), "TestClaimed")
			}
		})
	}
}

func TestResolveTestEvidenceUndoRestoresState(t *testing.T) {
	t.Parallel()
	vs := evidenceState(claimTestClaimedSuites())
	cmd := (&ResolveTestEvidenceBuilder{
		VS: vs, ModuleFrom: moduleSel, PackagesFrom: packagesSel, TestsFrom: testsSel,
	}).Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{
		"go_module":         outputPayloadStr(evidenceModule),
		"go_packages":       outputPayloadStr(evidencePackage + "\n"),
		"go_test_inventory": outputPayloadStr(mustListStream(evidencePackage, "TestClaimed")),
	}))

	result := cmd.Execute()
	require.Equal(t, core.ValidationPassed, result.Signal, result.Output)
	require.NotNil(t, vs.TestInventory)
	require.NotEmpty(t, result.Receipt)

	undo := cmd.Undo(result)
	require.Equal(t, core.ToolDone, undo.Signal, undo.Output)
	require.Nil(t, vs.TestInventory, "undo clears the resolved inventory")
	require.Empty(t, vs.Findings, "undo clears appended findings")
}

func TestReduceTestEvidenceRunBuilder(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		runOutput   string
		wantSignal  core.Signal
		wantErr     bool
		wantOutSub  string
		wantMessage string
	}{
		{
			name:       "passing run validates",
			runOutput:  mustRunEvent("pass", evidencePackage, "TestClaimed"),
			wantSignal: core.ValidationPassed,
		},
		{
			name:        "failing run is reported",
			runOutput:   mustRunEvent("fail", evidencePackage, "TestClaimed"),
			wantSignal:  core.ValidationFailed,
			wantMessage: "TestClaimed failed",
		},
		{
			name:        "skipped run is not evidence",
			runOutput:   mustRunEvent("skip", evidencePackage, "TestClaimed"),
			wantSignal:  core.ValidationFailed,
			wantMessage: "was skipped",
		},
		{
			name:        "claim that never ran is reported",
			runOutput:   mustRunEvent("pass", evidencePackage, "TestOther"),
			wantSignal:  core.ValidationFailed,
			wantMessage: "did not run",
		},
		{
			name:       "no top-level results is a command error",
			runOutput:  "",
			wantSignal: core.CommandError,
			wantErr:    true,
			wantOutSub: "no top-level test results",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			vs := resolvedEvidenceState(t)
			startFindings := len(vs.Findings)

			cmd := (&ReduceTestEvidenceRunBuilder{VS: vs, RunFrom: runSel}).Build(core.Result{})
			cmd.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{
				"go_test_run": outputPayloadStr(tc.runOutput),
			}))

			result := cmd.Execute()
			require.Equal(t, tc.wantSignal, result.Signal, result.Output)
			if tc.wantErr {
				require.Error(t, result.Err)
				require.Contains(t, result.Output, tc.wantOutSub)
				require.Len(t, vs.Findings, startFindings, "a failed reduce must not append findings")
			}
			if tc.wantMessage != "" {
				require.Contains(t, findingMessages(vs.Findings), tc.wantMessage)
			}
		})
	}
}

func TestReduceTestEvidenceRunRequiresResolvedInventory(t *testing.T) {
	t.Parallel()
	vs := evidenceState(claimTestClaimedSuites()) // no inventory resolved yet
	cmd := (&ReduceTestEvidenceRunBuilder{VS: vs, RunFrom: runSel}).Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{
		"go_test_run": outputPayloadStr(mustRunEvent("pass", evidencePackage, "TestClaimed")),
	}))

	result := cmd.Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "test inventory was not resolved")
}

func TestReduceTestEvidenceRunUnresolvedSelector(t *testing.T) {
	t.Parallel()
	vs := resolvedEvidenceState(t)
	cmd := (&ReduceTestEvidenceRunBuilder{VS: vs, RunFrom: runSel}).Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{})) // empty view

	result := cmd.Execute()
	require.Equal(t, core.CommandError, result.Signal)
	require.Contains(t, result.Output, "go_test_run")
}

func TestReduceTestEvidenceRunUndoRestoresState(t *testing.T) {
	t.Parallel()
	vs := resolvedEvidenceState(t)
	before := append([]spec.Finding(nil), vs.Findings...)

	cmd := (&ReduceTestEvidenceRunBuilder{VS: vs, RunFrom: runSel}).Build(core.Result{})
	cmd.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{
		"go_test_run": outputPayloadStr(mustRunEvent("fail", evidencePackage, "TestClaimed")),
	}))

	result := cmd.Execute()
	require.Equal(t, core.ValidationFailed, result.Signal, result.Output)
	require.NotEmpty(t, result.Receipt)
	require.Greater(t, len(vs.Findings), len(before), "reduce appended a failure finding")

	undo := cmd.Undo(result)
	require.Equal(t, core.ToolDone, undo.Signal, undo.Output)
	require.Len(t, vs.Findings, len(before), "undo drops the appended run findings")
	require.NotNil(t, vs.TestInventory, "undo keeps the previously resolved inventory")
}

// TestTestEvidenceSentenceEndToEnd runs the complete formal-evidence sentence the
// audit machine declares -- load_test_claims -> resolve_test_evidence ->
// reduce_test_evidence_run -- through the shared SpecState, proving the three
// words compose into a green audit when every claim resolves and passes.
func TestTestEvidenceSentenceEndToEnd(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, spec.TSSubdir)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test-rel09.0-example.yaml"), []byte(`id: test-rel09.0-example
title: Example
test_cases:
  - name: A claim
    go_test: TestClaimed
`), 0o644))

	vs := &SpecState{Directory: root}

	loaded := (&LoadTestClaimsBuilder{VS: vs}).Build(core.Result{}).Execute()
	require.Equal(t, core.ToolDone, loaded.Signal, loaded.Output)

	resolve := (&ResolveTestEvidenceBuilder{
		VS: vs, ModuleFrom: moduleSel, PackagesFrom: packagesSel, TestsFrom: testsSel,
	}).Build(core.Result{})
	resolve.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{
		"go_module":         outputPayloadStr(evidenceModule),
		"go_packages":       outputPayloadStr(evidencePackage + "\n"),
		"go_test_inventory": outputPayloadStr(mustListStream(evidencePackage, "TestClaimed")),
	}))
	resolveResult := resolve.Execute()
	require.Equal(t, core.ValidationPassed, resolveResult.Signal, resolveResult.Output)

	reduce := (&ReduceTestEvidenceRunBuilder{VS: vs, RunFrom: runSel}).Build(core.Result{})
	reduce.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{
		"go_test_run": outputPayloadStr(mustRunEvent("pass", evidencePackage, "TestClaimed")),
	}))
	reduceResult := reduce.Execute()
	require.Equal(t, core.ValidationPassed, reduceResult.Signal, reduceResult.Output)
	require.False(t, vs.HasErrors, "the full sentence must leave no errors when every claim passes")
}

// resolvedEvidenceState returns a SpecState whose inventory has already been
// resolved for the shared TestClaimed suite, ready for a reduce step.
func resolvedEvidenceState(t *testing.T) *SpecState {
	t.Helper()
	vs := evidenceState(claimTestClaimedSuites())
	resolve := (&ResolveTestEvidenceBuilder{
		VS: vs, ModuleFrom: moduleSel, PackagesFrom: packagesSel, TestsFrom: testsSel,
	}).Build(core.Result{})
	resolve.(core.CommandStateAware).SetCommandState(viewFromPayloads(map[string]string{
		"go_module":         outputPayloadStr(evidenceModule),
		"go_packages":       outputPayloadStr(evidencePackage + "\n"),
		"go_test_inventory": outputPayloadStr(mustListStream(evidencePackage, "TestClaimed")),
	}))
	require.Equal(t, core.ValidationPassed, resolve.Execute().Signal)
	require.NotNil(t, vs.TestInventory)
	return vs
}

func findingMessages(findings []spec.Finding) string {
	var b strings.Builder
	for _, f := range findings {
		b.WriteString(f.Message)
		b.WriteByte('\n')
	}
	return b.String()
}

// outputPayloadStr, mustListStream, and mustRunEvent wrap the *testing.T helpers
// for use inside table literals, where a *testing.T is not in scope.
func outputPayloadStr(raw string) string {
	data, _ := json.Marshal(map[string]string{"output": raw})
	return string(data)
}

func mustListStream(pkg string, names ...string) string {
	var b strings.Builder
	for _, name := range names {
		data, _ := json.Marshal(map[string]string{"Action": "output", "Package": pkg, "Output": name})
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func mustRunEvent(action, pkg, test string) string {
	data, _ := json.Marshal(map[string]string{"Action": action, "Package": pkg, "Test": test})
	return string(data) + "\n"
}
