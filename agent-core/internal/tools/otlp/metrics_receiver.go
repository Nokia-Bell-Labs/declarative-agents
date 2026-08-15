// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package otlp

import (
	"context"
	"fmt"
	"time"

	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"
)

// MetricBatch is one complete OTLP metric export request plus bounded intake
// metadata. It is the metric-signal parallel of Batch (srd042 R9.1, R9.2).
type MetricBatch struct {
	ID       string
	Request  *colmetricpb.ExportMetricsServiceRequest
	Received time.Time
}

// DataPointCount returns the number of data points across every metric in the
// complete request, summed over all metric data shapes.
func (b MetricBatch) DataPointCount() int {
	return requestDataPointCount(b.Request)
}

// metricServiceServer adapts a receiverRuntime to the OTLP MetricsService.
// It is a separate type from receiverRuntime because a single Go type cannot
// carry two methods named Export; the trace service keeps Export on the
// runtime, and this adapter delegates the metric Export to the same runtime.
type metricServiceServer struct {
	colmetricpb.UnimplementedMetricsServiceServer
	runtime *receiverRuntime
}

func (s *metricServiceServer) Export(
	_ context.Context,
	request *colmetricpb.ExportMetricsServiceRequest,
) (*colmetricpb.ExportMetricsServiceResponse, error) {
	r := s.runtime
	if r.isStopped() {
		return nil, ErrReceiverStopped
	}
	cloned, ok := proto.Clone(request).(*colmetricpb.ExportMetricsServiceRequest)
	if !ok {
		return nil, fmt.Errorf("clone OTLP metric request")
	}
	batch := MetricBatch{
		ID:      fmt.Sprintf("%s-metric-%d", r.name, r.metricSeq.Add(1)),
		Request: cloned, Received: time.Now().UTC(),
	}
	return r.enqueueMetric(batch), nil
}

func (r *receiverRuntime) enqueueMetric(batch MetricBatch) *colmetricpb.ExportMetricsServiceResponse {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case r.metricQueue <- batch:
		return &colmetricpb.ExportMetricsServiceResponse{}
	default:
	}

	switch r.config.OverflowPolicy {
	case OverflowDropOldest:
		oldest := <-r.metricQueue
		r.recordMetricDrop(oldest)
		r.metricQueue <- batch
		return partialMetricSuccess(0, fmt.Sprintf("dropped oldest metric batch %s", oldest.ID))
	case OverflowDropNewest:
		r.recordMetricDrop(batch)
		return partialMetricSuccess(0, fmt.Sprintf("dropped newest metric batch %s", batch.ID))
	default:
		count := batch.DataPointCount()
		r.recordMetricDrop(batch)
		return partialMetricSuccess(count, "receiver metric queue is full")
	}
}

func (r *receiverRuntime) recordMetricDrop(batch MetricBatch) {
	r.droppedMetricBatches++
	r.droppedDataPoints += batch.DataPointCount()
}

func (r *receiverRuntime) dropQueuedMetrics() int {
	if r.config.DrainPolicy != DrainDrop {
		return 0
	}
	dropped := 0
	for len(r.metricQueue) > 0 {
		r.recordMetricDrop(<-r.metricQueue)
		dropped++
	}
	return dropped
}

// NextMetric waits for one complete FIFO metric batch, receiver stop, or
// cancellation. It is the metric-signal parallel of State.Next.
func (s *State) NextMetric(ctx context.Context, name string) (MetricBatch, error) {
	runtime, err := s.runtime(name)
	if err != nil {
		return MetricBatch{}, err
	}
	select {
	case batch := <-runtime.metricQueue:
		return batch, nil
	case <-runtime.stopped:
		return MetricBatch{}, ErrReceiverStopped
	case <-ctx.Done():
		return MetricBatch{}, ctx.Err()
	}
}

func requestDataPointCount(request *colmetricpb.ExportMetricsServiceRequest) int {
	count := 0
	for _, resourceMetrics := range request.GetResourceMetrics() {
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			for _, metric := range scopeMetrics.GetMetrics() {
				count += metricDataPointCount(metric)
			}
		}
	}
	return count
}

// metricDataPointCount counts the data points in one metric across every
// OTLP metric data shape (gauge, sum, histogram, exponential histogram,
// summary).
func metricDataPointCount(metric *metricpb.Metric) int {
	switch {
	case metric.GetGauge() != nil:
		return len(metric.GetGauge().GetDataPoints())
	case metric.GetSum() != nil:
		return len(metric.GetSum().GetDataPoints())
	case metric.GetHistogram() != nil:
		return len(metric.GetHistogram().GetDataPoints())
	case metric.GetExponentialHistogram() != nil:
		return len(metric.GetExponentialHistogram().GetDataPoints())
	case metric.GetSummary() != nil:
		return len(metric.GetSummary().GetDataPoints())
	default:
		return 0
	}
}

func partialMetricSuccess(rejected int, message string) *colmetricpb.ExportMetricsServiceResponse {
	return &colmetricpb.ExportMetricsServiceResponse{
		PartialSuccess: &colmetricpb.ExportMetricsPartialSuccess{
			RejectedDataPoints: int64(rejected), ErrorMessage: message,
		},
	}
}
