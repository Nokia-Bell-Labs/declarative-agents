// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	// InitAwaitSpans identifies the blocking batch await factory.
	InitAwaitSpans           = "await_spans"
	defaultBatchAwaitTimeout = 30 * time.Second
)

// AwaitConfig configures one receiver queue read.
type AwaitConfig struct {
	Receiver string
	Timeout  time.Duration
}

// AwaitBuilder constructs blocking batch await commands.
type AwaitBuilder struct {
	ToolName string
	Config   AwaitConfig
	State    *State
}

// Build creates one await command.
func (b AwaitBuilder) Build(_ core.Result) core.Command {
	return &awaitCommand{toolName: b.ToolName, config: b.Config, state: b.State}
}

type awaitCommand struct {
	toolName string
	config   AwaitConfig
	state    *State
}

func (c *awaitCommand) Name() string { return c.toolName }

func (c *awaitCommand) Execute() core.Result {
	return c.ExecuteContext(context.Background())
}

func (c *awaitCommand) ExecuteContext(ctx context.Context) core.Result {
	timeout := c.config.Timeout
	if timeout == 0 {
		timeout = defaultBatchAwaitTimeout
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	batch, err := c.state.Next(waitCtx, c.config.Receiver)
	switch {
	case err == nil:
		output, encodeErr := awaitOutput(batch)
		if encodeErr != nil {
			return receiverError(c.Name(), encodeErr)
		}
		return core.Result{
			Signal: core.Signal("SpansReceived"), CommandName: c.Name(), Output: output,
		}
	case errors.Is(err, ErrReceiverStopped):
		return core.Result{
			Signal: core.Signal("ReceiverStopped"), CommandName: c.Name(),
			Output: fmt.Sprintf(`{"receiver":%q,"status":"stopped"}`, c.config.Receiver),
		}
	case errors.Is(err, context.DeadlineExceeded):
		return core.Result{
			Signal: core.Signal("AwaitTimedOut"), CommandName: c.Name(),
			Output: fmt.Sprintf(`{"receiver":%q,"status":"timed_out"}`, c.config.Receiver),
		}
	case errors.Is(err, context.Canceled):
		return receiverError(c.Name(), err)
	default:
		return receiverError(c.Name(), err)
	}
}

func (c *awaitCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

var _ core.ContextCommand = (*awaitCommand)(nil)

func awaitOutput(batch Batch) (string, error) {
	payload, err := protojson.Marshal(batch.Request)
	if err != nil {
		return "", fmt.Errorf("encode OTLP batch %q: %w", batch.ID, err)
	}
	output := struct {
		Batch              json.RawMessage `json:"batch"`
		BatchID            string          `json:"batch_id"`
		SpanCount          int             `json:"span_count"`
		ServiceNames       []string        `json:"service_names"`
		TraceIDs           []string        `json:"trace_ids"`
		ResourceAttributes map[string]any  `json:"resource_attributes"`
		ReceivedAt         time.Time       `json:"received_at"`
	}{
		Batch: payload, BatchID: batch.ID, SpanCount: batch.SpanCount(),
		ServiceNames: serviceNames(batch.Request), TraceIDs: traceIDs(batch.Request),
		ResourceAttributes: resourceAttributes(batch.Request), ReceivedAt: batch.Received,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("encode await output for batch %q: %w", batch.ID, err)
	}
	return string(encoded), nil
}

func serviceNames(request *coltracepb.ExportTraceServiceRequest) []string {
	set := make(map[string]bool)
	for _, resourceSpans := range request.GetResourceSpans() {
		for _, item := range resourceSpans.GetResource().GetAttributes() {
			if item.GetKey() == "service.name" {
				if name := item.GetValue().GetStringValue(); name != "" {
					set[name] = true
				}
			}
		}
	}
	return sortedSet(set)
}

func traceIDs(request *coltracepb.ExportTraceServiceRequest) []string {
	set := make(map[string]bool)
	for _, resourceSpans := range request.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			for _, span := range scopeSpans.GetSpans() {
				if len(span.GetTraceId()) > 0 {
					set[hex.EncodeToString(span.GetTraceId())] = true
				}
			}
		}
	}
	return sortedSet(set)
}

func resourceAttributes(request *coltracepb.ExportTraceServiceRequest) map[string]any {
	out := make(map[string]any)
	for _, resourceSpans := range request.GetResourceSpans() {
		mergeResourceAttributes(out, resourceSpans.GetResource())
	}
	return out
}

func mergeResourceAttributes(out map[string]any, resource *resourcepb.Resource) {
	for _, item := range resource.GetAttributes() {
		if _, exists := out[item.GetKey()]; !exists {
			out[item.GetKey()] = anyValue(item.GetValue())
		}
	}
}

func anyValue(value *commonpb.AnyValue) any {
	if value == nil {
		return nil
	}
	switch typed := value.Value.(type) {
	case *commonpb.AnyValue_StringValue:
		return typed.StringValue
	case *commonpb.AnyValue_BoolValue:
		return typed.BoolValue
	case *commonpb.AnyValue_IntValue:
		return typed.IntValue
	case *commonpb.AnyValue_DoubleValue:
		return typed.DoubleValue
	case *commonpb.AnyValue_BytesValue:
		return hex.EncodeToString(typed.BytesValue)
	case *commonpb.AnyValue_ArrayValue:
		values := make([]any, 0, len(typed.ArrayValue.GetValues()))
		for _, item := range typed.ArrayValue.GetValues() {
			values = append(values, anyValue(item))
		}
		return values
	case *commonpb.AnyValue_KvlistValue:
		values := make(map[string]any)
		for _, item := range typed.KvlistValue.GetValues() {
			values[item.GetKey()] = anyValue(item.GetValue())
		}
		return values
	default:
		return nil
	}
}

func sortedSet(set map[string]bool) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}
