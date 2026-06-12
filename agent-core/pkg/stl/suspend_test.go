// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/internal/observability/tracing"
	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/core"
)

func TestSuspendBuilderEmitsAwaitApproval(t *testing.T) {
	t.Parallel()
	cmd := (&SuspendBuilder{
		Config: SuspendConfig{Reason: "needs review"},
		Tracer: tracing.NoopTracer{},
	}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.AwaitApproval, res.Signal)
	require.Equal(t, "suspend", res.CommandName)
	require.Equal(t, "needs review", res.Output)
}

func TestSuspendUndoMementoCapturesApprovalCompensation(t *testing.T) {
	t.Parallel()
	cmd := (&SuspendBuilder{
		Config: SuspendConfig{Reason: "needs approval", RequireCheckpoint: true},
		Tracer: tracing.NoopTracer{},
	}).Build(core.Result{})

	provider, ok := cmd.(core.UndoMementoProvider)
	require.True(t, ok)
	memento, err := provider.UndoMemento()
	require.NoError(t, err)
	require.NoError(t, core.ValidateUndoMemento(memento))
	require.Equal(t, core.UndoMementoCompensatable, memento.Kind)

	var payload BoundaryCompensationPayload
	require.NoError(t, json.Unmarshal(memento.Payload, &payload))
	require.Equal(t, "resume_or_checkpoint_rollback", payload.BoundaryCompensation.Strategy)
	require.True(t, payload.BoundaryCompensation.CheckpointRequired)
}

func TestSuspendRequiresStateStoreWhenConfigured(t *testing.T) {
	t.Parallel()
	cmd := (&SuspendBuilder{
		Config: SuspendConfig{RequireCheckpoint: true},
		Tracer: tracing.NoopTracer{},
	}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.CommandError, res.Signal)
	require.ErrorContains(t, res.Err, "StateStore")
	require.Contains(t, res.Output, "StateStore")
}

func TestSuspendAllowsMissingStateStoreByDefault(t *testing.T) {
	t.Parallel()
	cmd := (&SuspendBuilder{Tracer: tracing.NoopTracer{}}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.AwaitApproval, res.Signal)
	require.Equal(t, "awaiting approval", res.Output)
}

func TestRegisterLifecycleFactoriesRegistersSuspend(t *testing.T) {
	t.Parallel()
	br := NewBuiltinRegistry()
	RegisterLifecycleFactories(br, LifecycleFactoryDeps{Tracer: tracing.NoopTracer{}})
	factory, ok := br.Resolve("suspend")
	require.True(t, ok)

	builder, err := factory(ToolDef{
		Name: "suspend",
		Type: "builtin",
		Init: "suspend",
		Config: map[string]interface{}{
			"label":              "approval",
			"reason":             "human approval required",
			"require_checkpoint": false,
		},
	}, nil)
	require.NoError(t, err)

	res := builder.Build(core.Result{}).Execute()
	require.Equal(t, core.AwaitApproval, res.Signal)
	require.Equal(t, "human approval required", res.Output)
}
