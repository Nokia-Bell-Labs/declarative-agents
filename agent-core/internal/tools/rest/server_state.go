// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/monitor"
)

const (
	defaultQueueCapacity = 16
	defaultAwaitTimeout  = 30 * time.Second
	defaultStopTimeout   = 5 * time.Second
)

// ServerState tracks launched REST servers and their inbound queues.
type ServerState struct {
	mu      sync.Mutex
	servers map[string]*serverRuntime
}

// NewServerState creates shared REST server state.
func NewServerState() *ServerState {
	return &ServerState{servers: map[string]*serverRuntime{}}
}

// InboundEvent is one validated HTTP request visible to MachineSpec.
type InboundEvent struct {
	Source    string                 `json:"source"`
	Route     string                 `json:"route"`
	Method    string                 `json:"method"`
	Signal    string                 `json:"signal"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
}

// StoppedSourceBehavior controls how fan-in await handles stopped servers.
type StoppedSourceBehavior string

const (
	// StoppedSourceIgnore keeps waiting on other sources when one server stops.
	StoppedSourceIgnore StoppedSourceBehavior = "ignore"
	// StoppedSourceEmitServerStopped emits ServerStopped when one source stops.
	StoppedSourceEmitServerStopped StoppedSourceBehavior = "emit_server_stopped"
	// StoppedSourceCommandError treats a stopped source as CommandError.
	StoppedSourceCommandError StoppedSourceBehavior = "command_error"
)

// AwaitSource selects one launched server queue for fan-in await.
type AwaitSource struct {
	Server          string
	Routes          []string
	Signals         []string
	StoppedBehavior StoppedSourceBehavior
}

// AwaitAnyOptions configures waiting across multiple REST server sources.
type AwaitAnyOptions struct {
	Sources         []AwaitSource
	Timeout         time.Duration
	StoppedBehavior StoppedSourceBehavior
}

type serverRuntime struct {
	name           string
	def            ServerDefinition
	httpServer     *http.Server
	listener       net.Listener
	queue          chan InboundEvent
	runner         MachineRequestRunner
	requestMonitor monitor.RuntimeRecorder
	pending        []InboundEvent
	stopped        chan struct{}
	stopOnce       sync.Once
	activeStreams  int
	droppedEvents  int
	owned          bool
	mu             sync.Mutex
}

// Launch starts a configured REST server without waiting for requests.
func (s *ServerState) Launch(def ServerDefinition) (map[string]interface{}, error) {
	runtime, err := newServerRuntime(def)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if _, exists := s.servers[def.Name]; exists {
		s.mu.Unlock()
		_ = runtime.listener.Close()
		return nil, fmt.Errorf("REST server %q is already launched", def.Name)
	}
	s.servers[def.Name] = runtime
	s.mu.Unlock()
	go serveRuntime(runtime)
	return runtime.launchOutput(), nil
}

// Await waits for one inbound event, timeout, or shutdown.
func (s *ServerState) Await(name string) (InboundEvent, string, error) {
	runtime, err := s.runtime(name)
	if err != nil {
		return InboundEvent{}, "CommandError", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtime.awaitTimeout())
	defer cancel()
	result := runtime.awaitMatching(ctx, awaitFilter{server: name}, StoppedSourceEmitServerStopped)
	if result.signal == "" && result.err == nil {
		return InboundEvent{Source: name}, "AwaitTimedOut", nil
	}
	return result.event, result.signal, result.err
}

// AwaitAny waits across multiple launched REST server queues.
func (s *ServerState) AwaitAny(options AwaitAnyOptions) (InboundEvent, string, error) {
	sources, err := s.resolveAwaitSources(options)
	if err != nil {
		return InboundEvent{}, "CommandError", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), awaitAnyTimeout(options))
	defer cancel()
	result := waitAnySource(ctx, cancel, sources)
	return result.event, result.signal, result.err
}

// Stop shuts down a configured REST server and drains queued events.
func (s *ServerState) Stop(name string) (map[string]interface{}, error) {
	runtime, err := s.runtime(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtime.stopTimeout())
	defer cancel()
	runtime.closeStopped()
	shutdownErr := runtime.httpServer.Shutdown(ctx)
	s.mu.Lock()
	delete(s.servers, name)
	s.mu.Unlock()
	output := runtime.stopOutput()
	if shutdownErr != nil {
		return output, fmt.Errorf("shutdown REST server %q: %w", name, shutdownErr)
	}
	return output, nil
}

func (s *ServerState) runtime(name string) (*serverRuntime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	runtime, ok := s.servers[name]
	if !ok {
		return nil, fmt.Errorf("REST server %q is not launched", name)
	}
	return runtime, nil
}

func (s *ServerState) resolveAwaitSources(options AwaitAnyOptions) ([]resolvedAwaitSource, error) {
	if len(options.Sources) == 0 {
		return nil, fmt.Errorf("at least one REST await source is required")
	}
	sources := make([]resolvedAwaitSource, 0, len(options.Sources))
	for _, source := range options.Sources {
		runtime, err := s.runtime(source.Server)
		if err != nil {
			return nil, err
		}
		sources = append(sources, resolvedAwaitSource{
			runtime: runtime,
			filter:  awaitFilter{server: source.Server, routes: source.Routes, signals: source.Signals},
			stopped: stoppedBehavior(source.StoppedBehavior, options.StoppedBehavior),
		})
	}
	return sources, nil
}

func newServerRuntime(def ServerDefinition) (*serverRuntime, error) {
	if err := validateRouteConflicts(def.Server.Endpoints); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", def.Server.Address)
	if err != nil {
		return nil, fmt.Errorf("bind REST server %q: %w", def.Name, err)
	}
	var requestMon monitor.RuntimeRecorder
	if def.Monitor.Store != nil {
		requestMon = monitor.NewRecorder(def.Monitor.Store, nil)
	}
	runtime := &serverRuntime{
		name: def.Name, def: def, listener: listener, stopped: make(chan struct{}),
		runner:         machineRequestRunner(def.MachineRequestRunner),
		requestMonitor: requestMon,
		queue:          make(chan InboundEvent, queueCapacity(def.Server.Queue)), owned: true,
	}
	runtime.httpServer = &http.Server{
		Handler:           runtime,
		ReadTimeout:       parseDuration(def.Limits.ReadTimeout, 0),
		ReadHeaderTimeout: parseDuration(def.Limits.ConnectTimeout, 0),
		MaxHeaderBytes:    def.Limits.MaxHeaderBytes,
	}
	return runtime, nil
}

func serveRuntime(runtime *serverRuntime) {
	err := runtime.httpServer.Serve(runtime.listener)
	if err != nil && err != http.ErrServerClosed {
		runtime.closeStopped()
	}
}

func validateRouteConflicts(endpoints map[string]Endpoint) error {
	seen := map[string]string{}
	for name, endpoint := range endpoints {
		key := endpoint.Method + " " + endpoint.Path
		if previous, ok := seen[key]; ok {
			return fmt.Errorf("route conflict between %q and %q", previous, name)
		}
		seen[key] = name
	}
	return nil
}

func (r *serverRuntime) closeStopped() {
	r.stopOnce.Do(func() { close(r.stopped) })
}

func (r *serverRuntime) launchOutput() map[string]interface{} {
	return map[string]interface{}{
		"server":         r.name,
		"address":        r.listener.Addr().String(),
		"route_count":    len(r.def.Server.Endpoints),
		"bindings":       r.bindingKinds(),
		"owned":          r.owned,
		"active_streams": r.activeStreams,
	}
}

func (r *serverRuntime) stopOutput() map[string]interface{} {
	drained := r.drainQueue()
	r.mu.Lock()
	dropped := r.droppedEvents
	r.mu.Unlock()
	return map[string]interface{}{
		"server": r.name, "address": r.listener.Addr().String(),
		"drained_events": drained, "dropped_events": dropped, "status": "stopped",
		"drain_policy": shutdownDrainPolicy(r.def.Server.Shutdown), "queue_outcome": "drained",
	}
}

func (r *serverRuntime) drainQueue() int {
	drained := r.drainPending()
	for {
		select {
		case <-r.queue:
			drained++
		default:
			return drained
		}
	}
}

func (r *serverRuntime) drainPending() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	drained := len(r.pending)
	r.pending = nil
	return drained
}

func (r *serverRuntime) bindingKinds() []string {
	seen := map[string]bool{}
	var bindings []string
	for _, endpoint := range r.def.Server.Endpoints {
		if !seen[endpoint.Binding] {
			seen[endpoint.Binding] = true
			bindings = append(bindings, endpoint.Binding)
		}
	}
	return bindings
}

func (r *serverRuntime) incrementStreams() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeStreams++
}

func (r *serverRuntime) decrementStreams() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activeStreams--
}

func queueCapacity(queue QueueConfig) int {
	if queue.Capacity > 0 {
		return queue.Capacity
	}
	return defaultQueueCapacity
}

func parseDuration(value string, fallback time.Duration) time.Duration {
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return duration
}

type awaitFilter struct {
	server  string
	routes  []string
	signals []string
}

type resolvedAwaitSource struct {
	runtime *serverRuntime
	filter  awaitFilter
	stopped StoppedSourceBehavior
}

type awaitResult struct {
	event  InboundEvent
	signal string
	err    error
	done   bool
}

func waitAnySource(
	ctx context.Context,
	cancel context.CancelFunc,
	sources []resolvedAwaitSource,
) awaitResult {
	results := make(chan awaitResult, len(sources))
	var wg sync.WaitGroup
	for _, source := range sources {
		wg.Add(1)
		go waitAwaitSource(ctx, &wg, source, results)
	}
	select {
	case result := <-results:
		cancel()
		wg.Wait()
		return result
	case <-ctx.Done():
		wg.Wait()
		return awaitResult{event: InboundEvent{}, signal: "AwaitTimedOut"}
	}
}

func waitAwaitSource(
	ctx context.Context,
	wg *sync.WaitGroup,
	source resolvedAwaitSource,
	results chan<- awaitResult,
) {
	defer wg.Done()
	result := source.runtime.awaitMatching(ctx, source.filter, source.stopped)
	if result.signal == "" {
		return
	}
	select {
	case results <- result:
	case <-ctx.Done():
	}
}

func (r *serverRuntime) awaitMatching(
	ctx context.Context,
	filter awaitFilter,
	stopped StoppedSourceBehavior,
) awaitResult {
	for {
		if event, ok := r.takePending(filter); ok {
			return awaitResult{event: event, signal: event.Signal}
		}
		result := r.awaitNext(ctx, filter, stopped)
		if result.done || result.signal != "" || result.err != nil {
			return result
		}
	}
}

func (r *serverRuntime) awaitNext(
	ctx context.Context,
	filter awaitFilter,
	stopped StoppedSourceBehavior,
) awaitResult {
	select {
	case event := <-r.queue:
		if filter.matches(event) {
			return awaitResult{event: event, signal: event.Signal}
		}
		r.storePending(event)
		return awaitResult{}
	case <-r.stopped:
		return stoppedResult(r.name, stopped)
	case <-ctx.Done():
		return awaitResult{done: true}
	}
}

func (r *serverRuntime) takePending(filter awaitFilter) (InboundEvent, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for index, event := range r.pending {
		if filter.matches(event) {
			r.pending = append(r.pending[:index], r.pending[index+1:]...)
			return event, true
		}
	}
	return InboundEvent{}, false
}

func (r *serverRuntime) storePending(event InboundEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending = append(r.pending, event)
}

func (f awaitFilter) matches(event InboundEvent) bool {
	return matchesValue(f.server, event.Source) &&
		matchesList(f.routes, event.Route) &&
		matchesList(f.signals, event.Signal)
}

func matchesValue(want, got string) bool {
	return want == "" || want == got
}

func matchesList(allowed []string, got string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, candidate := range allowed {
		if candidate == got {
			return true
		}
	}
	return false
}

func stoppedResult(name string, behavior StoppedSourceBehavior) awaitResult {
	switch behavior {
	case StoppedSourceIgnore:
		return awaitResult{done: true}
	case StoppedSourceCommandError:
		return awaitResult{
			event: InboundEvent{Source: name}, signal: "CommandError",
			err: fmt.Errorf("REST server %q stopped while awaiting events", name),
		}
	default:
		return awaitResult{event: InboundEvent{Source: name}, signal: "ServerStopped"}
	}
}

func stoppedBehavior(source, fallback StoppedSourceBehavior) StoppedSourceBehavior {
	if source != "" {
		return source
	}
	if fallback != "" {
		return fallback
	}
	return StoppedSourceEmitServerStopped
}

func awaitAnyTimeout(options AwaitAnyOptions) time.Duration {
	if options.Timeout > 0 {
		return options.Timeout
	}
	return defaultAwaitTimeout
}

func jsonOutput(value map[string]interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(data)
}

func (r *serverRuntime) awaitTimeout() time.Duration {
	return parseDuration(r.def.Server.Queue.Timeout, defaultAwaitTimeout)
}

func (r *serverRuntime) stopTimeout() time.Duration {
	return parseDuration(r.def.Server.Shutdown.Timeout, defaultStopTimeout)
}
