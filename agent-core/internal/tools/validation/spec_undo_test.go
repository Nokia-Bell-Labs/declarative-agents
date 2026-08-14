// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

func TestSpecUndoRestoresStateFromInMemorySnapshot(t *testing.T) {
	originalCorpus := &spec.Corpus{}
	vs := &SpecState{
		Directory:       "/source",
		TargetDirectory: "/work",
		SuitePaths:      []string{"suite.yaml"},
		Corpus:          originalCorpus,
		Charters:        []spec.Charter{{ID: "suite"}},
		Findings:        []spec.Finding{{Message: "before"}},
		HasErrors:       true,
		CorpusOptional:  true,
	}
	snap := snapshotSpec(vs)

	vs.Directory = "/changed-source"
	vs.TargetDirectory = "/other"
	vs.SuitePaths = nil
	vs.Corpus = nil
	vs.Charters = nil
	vs.Findings = nil
	vs.HasErrors = false
	vs.CorpusOptional = false
	res := undoSpecState("validate_specs", vs, core.Result{}, snap, true, nil)

	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, "/source", vs.Directory)
	require.Equal(t, "/work", vs.TargetDirectory)
	require.Equal(t, []string{"suite.yaml"}, vs.SuitePaths)
	require.Same(t, originalCorpus, vs.Corpus)
	require.Len(t, vs.Charters, 1)
	require.Len(t, vs.Findings, 1)
	require.True(t, vs.HasErrors)
	require.True(t, vs.CorpusOptional)
}

func TestSpecReceiptRestoresFullStateFromAuthoritativeDomainReference(t *testing.T) {
	prior := fullSpecState(t)
	encoded, err := EncodeSpecState(prior)
	require.NoError(t, err)
	checkpoint := core.NewInMemoryCheckpoint("validation-run")
	require.NoError(t, checkpoint.Save(core.Position{
		Snapshot: core.AgentSnapshot{Domain: encoded},
	}, checkpointExecution("prior")))

	_, receipt, err := captureSpecUndo(prior, checkpoint)
	require.NoError(t, err)
	require.LessOrEqual(t, len(receipt), maxSpecReceiptBytes)
	require.NotContains(t, receipt, prior.TargetDirectory)
	require.NotContains(t, receipt, prior.Findings[0].Message)

	// The fresh state deliberately points at a nonexistent workspace. Graph
	// restoration must use the persisted corpus rather than reading files.
	fresh := &SpecState{Directory: filepath.Join(t.TempDir(), "missing"), HasErrors: false}
	cmd := (&ValidateSpecsBuilder{
		ToolName:         "audit_specs",
		VS:               fresh,
		SnapshotResolver: directDomainResolver{checkpoint},
	}).BuildReverser()
	res := cmd.Undo(core.Result{Receipt: receipt})

	require.Equal(t, core.ToolDone, res.Signal)
	require.Equal(t, "audit_specs", res.CommandName)
	require.Equal(t, prior.Directory, fresh.Directory)
	require.Equal(t, prior.TargetDirectory, fresh.TargetDirectory)
	require.Equal(t, prior.SuitePaths, fresh.SuitePaths)
	require.Equal(t, prior.Corpus, fresh.Corpus)
	require.NotNil(t, fresh.Graph)
	require.Equal(t, prior.Graph.NodeCount(), fresh.Graph.NodeCount())
	require.Equal(t, prior.Graph.Edges(), fresh.Graph.Edges())
	require.Equal(t, prior.Charters, fresh.Charters)
	require.Equal(t, prior.Findings, fresh.Findings)
	require.Equal(t, prior.TestInventory, fresh.TestInventory)
	require.Equal(t, prior.HasErrors, fresh.HasErrors)
	require.Equal(t, prior.CorpusOptional, fresh.CorpusOptional)
}

func TestSpecStateCodecIsDeterministicAndVersioned(t *testing.T) {
	state := fullSpecState(t)
	first, err := EncodeSpecState(state)
	require.NoError(t, err)
	second, err := EncodeSpecState(state)
	require.NoError(t, err)
	require.Equal(t, first, second)

	restored := &SpecState{}
	require.NoError(t, RestoreSpecState(restored, first))
	roundTrip, err := EncodeSpecState(restored)
	require.NoError(t, err)
	require.Equal(t, first, roundTrip)

	var envelope map[string]interface{}
	require.NoError(t, json.Unmarshal(first, &envelope))
	require.EqualValues(t, specStateCodecVersion, envelope["version"])

	var wrongVersion map[string]interface{}
	require.NoError(t, json.Unmarshal(first, &wrongVersion))
	wrongVersion["version"] = 99
	data, err := json.Marshal(wrongVersion)
	require.NoError(t, err)
	require.ErrorContains(t, RestoreSpecState(&SpecState{}, data), "unsupported version")
}

func TestSpecUndoRejectsMissingMalformedAndMismatchedReferences(t *testing.T) {
	prior := fullSpecState(t)
	checkpoint := core.NewInMemoryCheckpoint("validation-run")
	encoded, err := EncodeSpecState(prior)
	require.NoError(t, err)
	require.NoError(t, checkpoint.Save(
		core.Position{Snapshot: core.AgentSnapshot{Domain: encoded}},
		checkpointExecution("prior"),
	))
	_, validReceipt, err := captureSpecUndo(prior, checkpoint)
	require.NoError(t, err)

	_, missingReference, err := captureSpecUndo(prior, nil)
	require.NoError(t, err)

	other := *prior
	other.TargetDirectory = "/different-checkpoint"
	otherState, err := EncodeSpecState(&other)
	require.NoError(t, err)
	require.NoError(t, checkpoint.Save(
		core.Position{Snapshot: core.AgentSnapshot{Domain: otherState}},
		append(checkpointExecution("prior"), checkpointExecution("other")...),
	))
	otherReference, ok := checkpoint.DomainReference()
	require.True(t, ok)
	var mismatched specReceipt
	require.NoError(t, json.Unmarshal([]byte(validReceipt), &mismatched))
	mismatched.DomainReference = otherReference
	mismatchedReceipt, err := json.Marshal(mismatched)
	require.NoError(t, err)

	var invalidReference specReceipt
	require.NoError(t, json.Unmarshal([]byte(validReceipt), &invalidReference))
	invalidReference.DomainReference = "not-a-checkpoint-reference"
	invalidReferenceReceipt, err := json.Marshal(invalidReference)
	require.NoError(t, err)

	tests := []struct {
		name    string
		receipt string
		want    string
	}{
		{name: "missing receipt", want: "no validation state snapshot"},
		{name: "malformed receipt", receipt: "{", want: "decode receipt"},
		{name: "missing reference", receipt: missingReference, want: "missing domain reference"},
		{name: "invalid reference", receipt: string(invalidReferenceReceipt), want: "invalid domain checkpoint reference"},
		{name: "mismatched state", receipt: string(mismatchedReceipt), want: "does not match prior validation state"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fresh := &SpecState{Directory: "unchanged"}
			cmd := (&ValidateSpecsBuilder{
				VS: fresh, SnapshotResolver: directDomainResolver{checkpoint},
			}).BuildReverser()
			result := cmd.Undo(core.Result{Receipt: test.receipt})
			require.Equal(t, core.CommandError, result.Signal)
			require.ErrorContains(t, result.Err, test.want)
			require.Equal(t, "unchanged", fresh.Directory)
			require.Nil(t, fresh.Corpus)
		})
	}
}

func TestValidationFactoriesPreserveAliasesAndReversers(t *testing.T) {
	builtins := toolregistry.NewBuiltinRegistry()
	state := &SpecState{Directory: "/work", TargetDirectory: "/work"}
	RegisterSpecFactories(builtins, FactoryDeps{Directory: "/work", State: state})

	defs := []catalog.ToolDef{
		{Name: "read_corpus", Type: "builtin", Init: "load_corpus"},
		{Name: "read_claims", Type: "builtin", Init: "load_test_claims"},
		{Name: "audit_specs", Type: "builtin", Init: "validate_specs"},
		{Name: "merge_consistency", Type: "builtin", Init: "reduce_consistency_checks"},
		{Name: "merge_refs", Type: "builtin", Init: "reduce_ref_checks"},
		{Name: "merge_grep", Type: "builtin", Init: "reduce_grep_checks"},
		{Name: "resolve_inventory", Type: "builtin", Init: "resolve_test_evidence"},
		{Name: "merge_test_run", Type: "builtin", Init: "reduce_test_evidence_run"},
		{Name: "print_report", Type: "builtin", Init: "format_report"},
	}
	registry := core.NewRegistry()
	for _, def := range defs {
		require.NoError(t, toolregistry.RegisterSingleBuiltin(registry, builtins, def, nil))
		builder, ok := registry.Resolve(def.Name)
		require.True(t, ok)
		require.Equal(t, def.Name, builder.Build(core.Result{}).Name())

		if def.Init == "format_report" {
			_, reversible := builder.(core.Reverser)
			require.False(t, reversible)
			continue
		}
		reverser, reversible := builder.(core.Reverser)
		require.True(t, reversible)
		require.Equal(t, def.Name, reverser.BuildReverser().Name())
	}
}

type directDomainResolver struct {
	core.DomainSnapshotResolver
}

func (r directDomainResolver) ResolveValidationSnapshot(reference string) ([]byte, error) {
	return r.ResolveDomainSnapshot(reference)
}

func checkpointExecution(name string) core.Execution {
	return core.Execution{{
		CommandName: name,
		Result: core.ResultDigest{
			RedactionVersion: core.OutputRedactionVersion1,
			RedactionStatus:  core.OutputRedactionApplied,
		},
	}}
}

func fullSpecState(t *testing.T) *SpecState {
	t.Helper()
	root := filepath.Join("..", "..", "..", "pkg", "spec", "testdata", "valid")
	corpus, err := spec.LoadCorpus(root)
	require.NoError(t, err)
	graph, err := spec.BuildGraph(corpus)
	require.NoError(t, err)
	inventory, err := spec.ParseGoTestInventory(
		"example.test/module",
		"example.test/module/pkg\n",
		`{"Action":"output","Package":"example.test/module/pkg","Output":"TestRestored\n"}`+"\n",
	)
	require.NoError(t, err)
	return &SpecState{
		Directory:       root,
		TargetDirectory: "/original-target",
		SuitePaths:      []string{"z-suite.yaml", "a-suite.yaml"},
		Corpus:          corpus,
		Graph:           graph,
		Charters: []spec.Charter{{
			ID: "suite", Checks: []spec.CharterCheck{{ID: "check", Kind: "spec_corpus"}},
		}},
		Findings: []spec.Finding{{
			Check: "check", Level: "error", Message: "before", SuiteID: "suite",
		}},
		TestInventory:  inventory,
		HasErrors:      true,
		CorpusOptional: true,
	}
}
