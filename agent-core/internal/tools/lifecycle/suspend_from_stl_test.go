// Copyright (c) 2026 Nokia. All rights reserved.

package lifecycle

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/catalog"
	toolregistry "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/registry"
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

func TestSuspendRequiresCheckpointBackendWhenConfigured(t *testing.T) {
	t.Parallel()
	// RequireCheckpoint with no persistent backend (nil, or the noop default)
	// fails; only a real backend satisfies the gate.
	for _, cp := range []core.Checkpoint{nil, core.NoopCheckpoint{}} {
		cmd := (&SuspendBuilder{
			Config:     SuspendConfig{RequireCheckpoint: true},
			Checkpoint: cp,
			Tracer:     tracing.NoopTracer{},
		}).Build(core.Result{})

		res := cmd.Execute()

		require.Equal(t, core.CommandError, res.Signal)
		require.ErrorContains(t, res.Err, "persistent checkpoint backend")
		require.Contains(t, res.Output, "persistent checkpoint backend")
	}
}

func TestSuspendAllowsMissingCheckpointByDefault(t *testing.T) {
	t.Parallel()
	cmd := (&SuspendBuilder{Tracer: tracing.NoopTracer{}}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.AwaitApproval, res.Signal)
	require.Equal(t, "awaiting approval", res.Output)
}

func TestSuspendWithPersistentCheckpointSatisfiesGate(t *testing.T) {
	t.Parallel()
	cmd := (&SuspendBuilder{
		Config:     SuspendConfig{RequireCheckpoint: true},
		Checkpoint: &core.InMemoryCheckpoint{},
		Tracer:     tracing.NoopTracer{},
	}).Build(core.Result{})

	res := cmd.Execute()

	require.Equal(t, core.AwaitApproval, res.Signal)
}

func TestRegisterLifecycleFactoriesRegistersSuspend(t *testing.T) {
	t.Parallel()
	br := toolregistry.NewBuiltinRegistry()
	RegisterFactories(br, FactoryDeps{Tracer: tracing.NoopTracer{}})
	factory, ok := br.Resolve("suspend")
	require.True(t, ok)

	builder, err := factory(catalog.ToolDef{
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
