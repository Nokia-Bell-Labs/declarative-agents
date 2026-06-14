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

type serverRuntime struct {
	name          string
	def           ServerDefinition
	httpServer    *http.Server
	listener      net.Listener
	queue         chan InboundEvent
	stopped       chan struct{}
	stopOnce      sync.Once
	activeStreams int
	droppedEvents int
	owned         bool
	mu            sync.Mutex
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
	timer := time.NewTimer(runtime.awaitTimeout())
	defer timer.Stop()
	select {
	case event := <-runtime.queue:
		return event, event.Signal, nil
	case <-runtime.stopped:
		return InboundEvent{Source: name}, "ServerStopped", nil
	case <-timer.C:
		return InboundEvent{Source: name}, "AwaitTimedOut", nil
	}
}

// Stop shuts down a configured REST server and drains queued events.
func (s *ServerState) Stop(name string) (map[string]interface{}, error) {
	runtime, err := s.runtime(name)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), runtime.stopTimeout())
	defer cancel()
	shutdownErr := runtime.httpServer.Shutdown(ctx)
	runtime.closeStopped()
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

func newServerRuntime(def ServerDefinition) (*serverRuntime, error) {
	if err := validateRouteConflicts(def.Server.Endpoints); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", def.Server.Address)
	if err != nil {
		return nil, fmt.Errorf("bind REST server %q: %w", def.Name, err)
	}
	runtime := &serverRuntime{
		name: def.Name, def: def, listener: listener, stopped: make(chan struct{}),
		queue: make(chan InboundEvent, queueCapacity(def.Server.Queue)), owned: true,
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
	}
}

func (r *serverRuntime) drainQueue() int {
	drained := 0
	for {
		select {
		case <-r.queue:
			drained++
		default:
			return drained
		}
	}
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
