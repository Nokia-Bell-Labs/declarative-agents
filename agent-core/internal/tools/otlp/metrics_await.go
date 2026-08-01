// Copyright (c) 2026 Nokia. All rights reserved.

package otlp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

// InitAwaitMetrics identifies the blocking metric batch await factory.
const InitAwaitMetrics = "await_metrics"

// MetricAwaitBuilder constructs blocking metric batch await commands.
type MetricAwaitBuilder struct {
	ToolName string
	Config   AwaitConfig
	State    *State
}

// Build creates one metric await command.
func (b MetricAwaitBuilder) Build(_ core.Result) core.Command {
	return &metricAwaitCommand{toolName: b.ToolName, config: b.Config, state: b.State}
}

type metricAwaitCommand struct {
	toolName string
	config   AwaitConfig
	state    *State
}

func (c *metricAwaitCommand) Name() string { return c.toolName }

func (c *metricAwaitCommand) Execute() core.Result {
	return c.ExecuteContext(context.Background())
}

func (c *metricAwaitCommand) ExecuteContext(ctx context.Context) core.Result {
	timeout := c.config.Timeout
	if timeout == 0 {
		timeout = defaultBatchAwaitTimeout
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	batch, err := c.state.NextMetric(waitCtx, c.config.Receiver)
	switch {
	case err == nil:
		output, encodeErr := awaitMetricOutput(batch)
		if encodeErr != nil {
			return receiverError(c.Name(), encodeErr)
		}
		return core.Result{
			Signal: core.Signal("MetricsReceived"), CommandName: c.Name(), Output: output,
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

func (c *metricAwaitCommand) Undo(_ core.Result) core.Result {
	return core.NoopUndo(c.Name())
}

var _ core.ContextCommand = (*metricAwaitCommand)(nil)

func awaitMetricOutput(batch MetricBatch) (string, error) {
	payload, err := protojson.Marshal(batch.Request)
	if err != nil {
		return "", fmt.Errorf("encode OTLP metric batch %q: %w", batch.ID, err)
	}
	output := struct {
		Batch              json.RawMessage `json:"batch"`
		BatchID            string          `json:"batch_id"`
		DataPointCount     int             `json:"data_point_count"`
		ServiceNames       []string        `json:"service_names"`
		MetricNames        []string        `json:"metric_names"`
		ResourceAttributes map[string]any  `json:"resource_attributes"`
		ReceivedAt         time.Time       `json:"received_at"`
	}{
		Batch: payload, BatchID: batch.ID, DataPointCount: batch.DataPointCount(),
		ServiceNames: metricServiceNames(batch.Request), MetricNames: metricNames(batch.Request),
		ResourceAttributes: metricResourceAttributes(batch.Request), ReceivedAt: batch.Received,
	}
	encoded, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("encode metric await output for batch %q: %w", batch.ID, err)
	}
	return string(encoded), nil
}

func metricServiceNames(request *colmetricpb.ExportMetricsServiceRequest) []string {
	set := make(map[string]bool)
	for _, resourceMetrics := range request.GetResourceMetrics() {
		for _, item := range resourceMetrics.GetResource().GetAttributes() {
			if item.GetKey() == "service.name" {
				if name := item.GetValue().GetStringValue(); name != "" {
					set[name] = true
				}
			}
		}
	}
	return sortedSet(set)
}

func metricNames(request *colmetricpb.ExportMetricsServiceRequest) []string {
	set := make(map[string]bool)
	for _, resourceMetrics := range request.GetResourceMetrics() {
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			for _, metric := range scopeMetrics.GetMetrics() {
				if name := metric.GetName(); name != "" {
					set[name] = true
				}
			}
		}
	}
	return sortedSet(set)
}

func metricResourceAttributes(request *colmetricpb.ExportMetricsServiceRequest) map[string]any {
	out := make(map[string]any)
	for _, resourceMetrics := range request.GetResourceMetrics() {
		mergeResourceAttributes(out, resourceMetrics.GetResource())
	}
	return out
}
