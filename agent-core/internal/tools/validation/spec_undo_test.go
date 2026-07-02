// Copyright (c) 2026 Nokia. All rights reserved.

package validation

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/pkg/spec"
)

func TestSpecUndoRestoresState(t *testing.T) {
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
	res := undoSpecSnapshot("validate_specs", vs, snap, true)

	require.Equal(t, core.ToolDone, res.Signal)
	require.Same(t, originalCorpus, vs.Corpus)
	require.Len(t, vs.Findings, 1)
	require.True(t, vs.HasErrors)

	memento, err := specMemento("validate_specs", snap, true)
	require.NoError(t, err)
	require.Equal(t, core.UndoMementoReversible, memento.Kind)
	require.NoError(t, core.ValidateUndoMemento(memento))
	require.Contains(t, string(memento.Payload), `"findings":1`)
}
