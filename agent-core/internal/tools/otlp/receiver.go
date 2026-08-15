// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package otlp implements the srd042 OTLP trace receiver vocabulary.
package otlp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

const (
	// InitReceiverLaunch identifies the OTLP receiver launch factory.
	InitReceiverLaunch = "otlp_receiver_launch"
	// InitReceiverStop identifies the OTLP receiver stop factory.
	InitReceiverStop = "otlp_receiver_stop"

	defaultAddress         = "0.0.0.0:4317"
	defaultQueueCapacity   = 16
	defaultShutdownTimeout = 5 * time.Second
)

// OverflowPolicy controls how a full receiver queue handles a new batch.
type OverflowPolicy string

const (
	OverflowReject     OverflowPolicy = "reject"
	OverflowDropOldest OverflowPolicy = "drop_oldest"
	OverflowDropNewest OverflowPolicy = "drop_newest"
)

// DrainPolicy controls queued batches when a receiver stops.
type DrainPolicy string

const (
	DrainPreserve DrainPolicy = "preserve"
	DrainDrop     DrainPolicy = "drop"
)

// ReceiverConfig is trusted, typed configuration for one OTLP listener.
type ReceiverConfig struct {
	Name            string
	Address         string
	QueueCapacity   int
	OverflowPolicy  OverflowPolicy
	ShutdownTimeout time.Duration
	DrainPolicy     DrainPolicy
}

// Batch is one complete OTLP trace export request plus bounded intake metadata.
type Batch struct {
	ID       string
	Request  *coltracepb.ExportTraceServiceRequest
	Received time.Time
}

// SpanCount returns the number of spans in the complete request.
func (b Batch) SpanCount() int {
	return requestSpanCount(b.Request)
}

// State owns all OTLP listeners launched by one runtime.
type State struct {
	mu        sync.Mutex
	receivers map[string]*receiverRuntime
}

// NewState creates empty shared OTLP receiver state.
func NewState() *State {
	return &State{receivers: make(map[string]*receiverRuntime)}
}

// Launch binds and serves one configured OTLP/gRPC receiver.
func (s *State) Launch(cfg ReceiverConfig) (map[string]any, error) {
	cfg = withReceiverDefaults(cfg)
	if err := validateReceiverConfig(cfg); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", cfg.Address)
	if err != nil {
		return nil, fmt.Errorf("bind OTLP receiver %q: %w", cfg.Name, err)
	}
	runtime := &receiverRuntime{
		name: cfg.Name, config: cfg, listener: listener,
		queue:       make(chan Batch, cfg.QueueCapacity),
		metricQueue: make(chan MetricBatch, cfg.QueueCapacity),
		stopped:     make(chan struct{}),
	}
	runtime.server = grpc.NewServer()
	coltracepb.RegisterTraceServiceServer(runtime.server, runtime)
	colmetricpb.RegisterMetricsServiceServer(runtime.server, &metricServiceServer{runtime: runtime})

	s.mu.Lock()
	if current, exists := s.receivers[cfg.Name]; exists && !current.isStopped() {
		s.mu.Unlock()
		_ = listener.Close()
		return nil, fmt.Errorf("OTLP receiver %q is already launched", cfg.Name)
	}
	s.receivers[cfg.Name] = runtime
	s.mu.Unlock()

	go runtime.serve()
	return runtime.launchOutput(), nil
}

// Stop shuts down a receiver and applies its declared queue drain policy.
func (s *State) Stop(name string) (map[string]any, error) {
	runtime, err := s.runtime(name)
	if err != nil {
		return nil, err
	}
	return runtime.stop()
}

func (s *State) stopIfRunning(name string) (map[string]any, error) {
	s.mu.Lock()
	runtime, ok := s.receivers[name]
	s.mu.Unlock()
	if !ok || runtime.isStopped() {
		return map[string]any{"receiver": name, "status": "already_stopped"}, nil
	}
	return runtime.stop()
}

// Next waits for one complete FIFO batch, receiver stop, or cancellation.
func (s *State) Next(ctx context.Context, name string) (Batch, error) {
	runtime, err := s.runtime(name)
	if err != nil {
		return Batch{}, err
	}
	select {
	case batch := <-runtime.queue:
		return batch, nil
	case <-runtime.stopped:
		return Batch{}, ErrReceiverStopped
	case <-ctx.Done():
		return Batch{}, ctx.Err()
	}
}

func (s *State) runtime(name string) (*receiverRuntime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime, ok := s.receivers[name]
	if !ok {
		return nil, fmt.Errorf("OTLP receiver %q is not launched", name)
	}
	return runtime, nil
}

// ErrReceiverStopped distinguishes lifecycle shutdown from transport faults.
var ErrReceiverStopped = fmt.Errorf("OTLP receiver stopped")

type receiverRuntime struct {
	coltracepb.UnimplementedTraceServiceServer

	name        string
	config      ReceiverConfig
	listener    net.Listener
	server      *grpc.Server
	queue       chan Batch
	metricQueue chan MetricBatch
	stopped     chan struct{}
	stopOnce    sync.Once
	stoppedOnce sync.Once
	sequence    atomic.Uint64
	metricSeq   atomic.Uint64

	mu                   sync.Mutex
	droppedBatches       int
	droppedSpans         int
	droppedMetricBatches int
	droppedDataPoints    int
	stopOutput           map[string]any
}

func (r *receiverRuntime) Export(
	_ context.Context,
	request *coltracepb.ExportTraceServiceRequest,
) (*coltracepb.ExportTraceServiceResponse, error) {
	if r.isStopped() {
		return nil, ErrReceiverStopped
	}
	cloned, ok := proto.Clone(request).(*coltracepb.ExportTraceServiceRequest)
	if !ok {
		return nil, fmt.Errorf("clone OTLP trace request")
	}
	batch := Batch{
		ID:      fmt.Sprintf("%s-%d", r.name, r.sequence.Add(1)),
		Request: cloned, Received: time.Now().UTC(),
	}
	return r.enqueue(batch), nil
}

func (r *receiverRuntime) enqueue(batch Batch) *coltracepb.ExportTraceServiceResponse {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case r.queue <- batch:
		return &coltracepb.ExportTraceServiceResponse{}
	default:
	}

	switch r.config.OverflowPolicy {
	case OverflowDropOldest:
		oldest := <-r.queue
		r.recordDrop(oldest)
		r.queue <- batch
		return partialSuccess(0, fmt.Sprintf("dropped oldest batch %s", oldest.ID))
	case OverflowDropNewest:
		r.recordDrop(batch)
		return partialSuccess(0, fmt.Sprintf("dropped newest batch %s", batch.ID))
	default:
		count := batch.SpanCount()
		r.recordDrop(batch)
		return partialSuccess(count, "receiver queue is full")
	}
}

func (r *receiverRuntime) recordDrop(batch Batch) {
	r.droppedBatches++
	r.droppedSpans += batch.SpanCount()
}

func (r *receiverRuntime) serve() {
	if err := r.server.Serve(r.listener); err != nil && !r.isStopped() {
		r.closeStopped()
	}
}

func (r *receiverRuntime) stop() (map[string]any, error) {
	var shutdownErr error
	r.stopOnce.Do(func() {
		r.closeStopped()
		shutdownErr = r.stopServer()
		_ = r.listener.Close()
		r.recordStopOutput()
	})
	r.mu.Lock()
	output := cloneMap(r.stopOutput)
	r.mu.Unlock()
	return output, shutdownErr
}

func (r *receiverRuntime) stopServer() error {
	done := make(chan struct{})
	go func() {
		r.server.GracefulStop()
		close(done)
	}()
	timer := time.NewTimer(r.config.ShutdownTimeout)
	defer timer.Stop()
	select {
	case <-done:
		return nil
	case <-timer.C:
		r.server.Stop()
		<-done
		return fmt.Errorf("shutdown OTLP receiver %q exceeded %s", r.name, r.config.ShutdownTimeout)
	}
}

func (r *receiverRuntime) recordStopOutput() {
	r.mu.Lock()
	defer r.mu.Unlock()
	queued := len(r.queue)
	queuedMetrics := len(r.metricQueue)
	droppedOnStop := r.dropQueuedBatches()
	droppedMetricsOnStop := r.dropQueuedMetrics()
	r.stopOutput = map[string]any{
		"receiver": r.name, "address": r.listener.Addr().String(),
		"queued_batches": queued, "dropped_on_stop": droppedOnStop,
		"dropped_batches": r.droppedBatches, "dropped_spans": r.droppedSpans,
		"queued_metrics": queuedMetrics, "dropped_metrics_on_stop": droppedMetricsOnStop,
		"dropped_metric_batches": r.droppedMetricBatches, "dropped_data_points": r.droppedDataPoints,
		"drain_policy": string(r.config.DrainPolicy), "status": "stopped",
	}
}

func (r *receiverRuntime) dropQueuedBatches() int {
	if r.config.DrainPolicy != DrainDrop {
		return 0
	}
	dropped := 0
	for len(r.queue) > 0 {
		r.recordDrop(<-r.queue)
		dropped++
	}
	return dropped
}

func (r *receiverRuntime) closeStopped() {
	r.stoppedOnce.Do(func() { close(r.stopped) })
}

func (r *receiverRuntime) isStopped() bool {
	select {
	case <-r.stopped:
		return true
	default:
		return false
	}
}

func (r *receiverRuntime) launchOutput() map[string]any {
	return map[string]any{
		"receiver": r.name, "address": r.listener.Addr().String(),
		"queue_capacity": cap(r.queue), "overflow_policy": string(r.config.OverflowPolicy),
	}
}

// ReceiverBuilder constructs receiver launch and stop boundary commands.
type ReceiverBuilder struct {
	ToolName string
	Init     string
	Config   ReceiverConfig
	State    *State
}

// Build constructs one receiver lifecycle command.
func (b ReceiverBuilder) Build(_ core.Result) core.Command {
	return receiverCommand{toolName: b.ToolName, init: b.Init, config: b.Config, state: b.State}
}

func (b ReceiverBuilder) BuildReverser() core.Command {
	return receiverCommand{toolName: b.ToolName, init: b.Init, config: b.Config, state: b.State}
}

var _ core.Reverser = ReceiverBuilder{}

type receiverReceipt struct {
	Name    string `json:"name"`
	Address string `json:"address"`
}

type receiverCommand struct {
	toolName string
	init     string
	config   ReceiverConfig
	state    *State
}

func (c receiverCommand) Name() string { return c.toolName }

func (c receiverCommand) Execute() core.Result {
	var (
		output map[string]any
		err    error
		signal core.Signal
	)
	switch c.init {
	case InitReceiverLaunch:
		output, err = c.state.Launch(c.config)
		signal = core.Signal("ReceiverLaunched")
	case InitReceiverStop:
		output, err = c.state.Stop(c.config.Name)
		signal = core.Signal("ReceiverStopped")
	default:
		err = fmt.Errorf("unsupported OTLP receiver init %q", c.init)
	}
	if err != nil {
		return receiverError(c.toolName, err)
	}
	data, err := json.Marshal(output)
	if err != nil {
		return receiverError(c.toolName, err)
	}
	result := core.Result{Signal: signal, CommandName: c.toolName, Output: string(data)}
	if c.init == InitReceiverLaunch {
		result.Receipt = encodeReceiverReceipt(output)
	}
	return result
}

func (c receiverCommand) Undo(prior core.Result) core.Result {
	if c.init != InitReceiverLaunch {
		return core.NoopUndo(c.toolName)
	}
	receipt, err := decodeReceiverReceipt(prior.Receipt)
	if err != nil {
		return receiverError(c.toolName, err)
	}
	output, err := c.state.stopIfRunning(receipt.Name)
	if err != nil {
		return receiverError(c.toolName, err)
	}
	data, _ := json.Marshal(output)
	return core.Result{Signal: core.Signal("ReceiverStopped"), CommandName: c.toolName, Output: string(data)}
}

func encodeReceiverReceipt(output map[string]any) string {
	receipt := receiverReceipt{
		Name: fmt.Sprint(output["receiver"]), Address: fmt.Sprint(output["address"]),
	}
	data, _ := json.Marshal(receipt)
	return string(data)
}

func decodeReceiverReceipt(value string) (receiverReceipt, error) {
	var receipt receiverReceipt
	if err := json.Unmarshal([]byte(value), &receipt); err != nil {
		return receipt, fmt.Errorf("decode OTLP receiver receipt: %w", err)
	}
	if receipt.Name == "" {
		return receipt, fmt.Errorf("decode OTLP receiver receipt: receiver name is required")
	}
	return receipt, nil
}

func receiverError(name string, err error) core.Result {
	return core.Result{Signal: core.CommandError, CommandName: name, Output: err.Error(), Err: err}
}

func withReceiverDefaults(cfg ReceiverConfig) ReceiverConfig {
	if cfg.Address == "" {
		cfg.Address = defaultAddress
	}
	if cfg.QueueCapacity == 0 {
		cfg.QueueCapacity = defaultQueueCapacity
	}
	if cfg.OverflowPolicy == "" {
		cfg.OverflowPolicy = OverflowReject
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = defaultShutdownTimeout
	}
	if cfg.DrainPolicy == "" {
		cfg.DrainPolicy = DrainPreserve
	}
	return cfg
}

func validateReceiverConfig(cfg ReceiverConfig) error {
	if strings.TrimSpace(cfg.Name) == "" {
		return fmt.Errorf("OTLP receiver name is required")
	}
	if strings.TrimSpace(cfg.Address) == "" {
		return fmt.Errorf("OTLP receiver %q address is required", cfg.Name)
	}
	if cfg.QueueCapacity < 1 {
		return fmt.Errorf("OTLP receiver %q queue_capacity must be at least 1", cfg.Name)
	}
	switch cfg.OverflowPolicy {
	case OverflowReject, OverflowDropOldest, OverflowDropNewest:
	default:
		return fmt.Errorf("OTLP receiver %q has unsupported overflow_policy %q", cfg.Name, cfg.OverflowPolicy)
	}
	if cfg.ShutdownTimeout <= 0 {
		return fmt.Errorf("OTLP receiver %q shutdown_timeout must be positive", cfg.Name)
	}
	switch cfg.DrainPolicy {
	case DrainPreserve, DrainDrop:
	default:
		return fmt.Errorf("OTLP receiver %q has unsupported drain_policy %q", cfg.Name, cfg.DrainPolicy)
	}
	return nil
}

func requestSpanCount(request *coltracepb.ExportTraceServiceRequest) int {
	count := 0
	for _, resourceSpans := range request.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			count += len(scopeSpans.GetSpans())
		}
	}
	return count
}

func partialSuccess(rejected int, message string) *coltracepb.ExportTraceServiceResponse {
	return &coltracepb.ExportTraceServiceResponse{
		PartialSuccess: &coltracepb.ExportTracePartialSuccess{
			RejectedSpans: int64(rejected), ErrorMessage: message,
		},
	}
}

func cloneMap(source map[string]any) map[string]any {
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}
