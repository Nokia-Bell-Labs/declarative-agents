// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func TestLoadOTLPBatchDistinguishesReadAndDecodeFailures(t *testing.T) {
	t.Parallel()
	missing := LoadBuilder{
		ToolName: "load_otlp_batch",
		Config:   LoadConfig{Path: filepath.Join(t.TempDir(), "missing.json")},
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.CommandError, missing.Signal)
	require.ErrorContains(t, missing.Err, "read OTLP batch")

	corruptPath := filepath.Join(t.TempDir(), "corrupt.json")
	require.NoError(t, os.WriteFile(corruptPath, []byte("{"), 0o600))
	corrupt := LoadBuilder{
		ToolName: "load_otlp_batch", Config: LoadConfig{Path: corruptPath},
	}.Build(core.Result{}).Execute()
	require.Equal(t, core.CommandError, corrupt.Signal)
	require.ErrorContains(t, corrupt.Err, "decode OTLP batch")
}

func TestOTLPReplayMachineLoadsThenRelaysUnchangedBatch(t *testing.T) {
	t.Parallel()
	request := spoolRequest()
	path := writeOTLPBatch(t, request)
	service, endpoint := startRelayService(t)
	spec, err := core.LoadMachineSpec(filepath.Join(
		"..", "..", "..", "testdata", "integration", "profiles", "otlp-replay", "machine.yaml",
	))
	require.NoError(t, err)
	registry := core.NewRegistry()
	registry.Register(core.ToolSpec{Name: "load_replay_batch"}, LoadBuilder{
		ToolName: "load_replay_batch", Config: LoadConfig{Path: path},
	})
	registry.Register(core.ToolSpec{Name: "relay_replay_batch"}, RelayBuilder{
		ToolName: "relay_replay_batch",
		Config: RelayConfig{
			Endpoint: endpoint, BatchSource: "$from(loaded_batch).batch", Timeout: time.Second,
		},
	})

	result, err := core.Loop(core.LoopParams{
		MachineSpec: &spec, Registry: registry, Budget: core.Budget{MaxIterations: 3},
		Trace: tracing.NoopTracer{},
	}, context.Background())

	require.NoError(t, err)
	require.Equal(t, core.StatusSucceeded, result.Status)
	require.True(t, proto.Equal(request, <-service.requests))
}

func writeOTLPBatch(t *testing.T, request *coltracepb.ExportTraceServiceRequest) string {
	t.Helper()
	data, err := protojson.Marshal(request)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "batch.otlp.json")
	require.NoError(t, os.WriteFile(path, data, 0o600))
	return path
}
