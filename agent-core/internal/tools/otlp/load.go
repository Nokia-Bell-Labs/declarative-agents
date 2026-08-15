// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// InitLoadOTLPBatch identifies the declared OTLP protobuf-JSON loader.
const InitLoadOTLPBatch = "load_otlp_batch"

// LoadConfig configures one trusted offline OTLP batch file.
type LoadConfig struct {
	Path string
}

// LoadBuilder constructs offline batch load commands.
type LoadBuilder struct {
	ToolName string
	Config   LoadConfig
}

func (b LoadBuilder) Build(core.Result) core.Command {
	return loadCommand{toolName: b.ToolName, config: b.Config}
}

type loadCommand struct {
	toolName string
	config   LoadConfig
}

func (c loadCommand) Name() string { return c.toolName }

func (c loadCommand) Execute() core.Result {
	data, err := os.ReadFile(c.config.Path)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("read OTLP batch %s: %w", c.config.Path, err))
	}
	var request coltracepb.ExportTraceServiceRequest
	if err := protojson.Unmarshal(data, &request); err != nil {
		return receiverError(c.Name(), fmt.Errorf("decode OTLP batch %s: %w", c.config.Path, err))
	}
	batch, err := protojson.Marshal(&request)
	if err != nil {
		return receiverError(c.Name(), fmt.Errorf("encode OTLP batch %s: %w", c.config.Path, err))
	}
	output, err := json.Marshal(struct {
		Path      string          `json:"path"`
		SpanCount int             `json:"span_count"`
		Batch     json.RawMessage `json:"batch"`
	}{
		Path: c.config.Path, SpanCount: requestSpanCount(&request), Batch: batch,
	})
	if err != nil {
		return receiverError(c.Name(), err)
	}
	return core.Result{Signal: core.Signal("BatchLoaded"), CommandName: c.Name(), Output: string(output)}
}

func (c loadCommand) Undo(core.Result) core.Result { return core.NoopUndo(c.Name()) }
