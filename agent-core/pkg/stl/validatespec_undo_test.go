// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/runtime/core"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/spec"
)

func TestValidateSpecUndoRestoresState(t *testing.T) {
	originalCorpus := &spec.Corpus{}
	vs := &ValidateSpecState{
		Corpus:    originalCorpus,
		Findings:  []spec.Finding{{Message: "before"}},
		HasErrors: true,
	}
	snap := snapshotValidateSpec(vs)

	vs.Corpus = nil
	vs.Findings = nil
	vs.HasErrors = false
	res := undoValidateSpecSnapshot("validate_specs", vs, snap, true)

	require.Equal(t, core.ToolDone, res.Signal)
	require.Same(t, originalCorpus, vs.Corpus)
	require.Len(t, vs.Findings, 1)
	require.True(t, vs.HasErrors)

	memento, err := validateSpecMemento("validate_specs", snap, true)
	require.NoError(t, err)
	require.Equal(t, core.UndoMementoReversible, memento.Kind)
	require.NoError(t, core.ValidateUndoMemento(memento))
	require.Contains(t, string(memento.Payload), `"findings":1`)
}
