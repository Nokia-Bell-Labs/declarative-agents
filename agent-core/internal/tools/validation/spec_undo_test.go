// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

func TestSpecUndoRestoresStateFromInMemorySnapshot(t *testing.T) {
	originalCorpus := &spec.Corpus{}
	vs := &SpecState{
		Corpus:    originalCorpus,
		Findings:  []spec.Finding{{Message: "before"}},
		HasErrors: true,
	}
	snap := snapshotSpec(vs)

	vs.Corpus = nil
	vs.Findings = nil
	vs.HasErrors = false
	res := undoSpecState("validate_specs", vs, core.Result{}, snap, true)

	require.Equal(t, core.ToolDone, res.Signal)
	require.Same(t, originalCorpus, vs.Corpus)
	require.Len(t, vs.Findings, 1)
	require.True(t, vs.HasErrors)
}

func TestSpecReceiptRestoresStateFromFreshInstance(t *testing.T) {
	// Prior state before validate_specs runs: corpus loaded, no graph, one finding.
	prior := &SpecState{
		Corpus:   &spec.Corpus{},
		Findings: []spec.Finding{{Check: "c", Level: "warning", Message: "before"}},
	}
	receipt := encodeSpecReceipt(snapshotSpec(prior))
	require.NotEmpty(t, receipt)

	cp := &core.InMemoryCheckpoint{}
	require.NoError(t, cp.Save(core.Position{}, core.Execution{{CommandName: "validate_specs", Receipt: receipt}}))
	_, exec, err := cp.Load()
	require.NoError(t, err)
	require.Len(t, exec, 1)

	// A fresh command instance sharing a freshly reconstructed SpecState that has
	// already been mutated (graph built, findings changed) undoes purely from the receipt.
	fresh := &SpecState{
		Corpus:    &spec.Corpus{},
		Graph:     &spec.Graph{},
		Findings:  []spec.Finding{{Message: "after"}, {Message: "after2"}},
		HasErrors: true,
	}
	cmd := (&ValidateSpecsBuilder{VS: fresh}).Build(core.Result{}).(*validateSpecsCmd)
	res := cmd.Undo(core.Result{Receipt: exec[0].Receipt})

	require.Equal(t, core.ToolDone, res.Signal)
	require.Nil(t, fresh.Graph)     // graph_loaded=false in receipt -> cleared
	require.NotNil(t, fresh.Corpus) // corpus_loaded=true -> left intact
	require.Len(t, fresh.Findings, 1)
	require.Equal(t, "before", fresh.Findings[0].Message)
	require.False(t, fresh.HasErrors)
}
