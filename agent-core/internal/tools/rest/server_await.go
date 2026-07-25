// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import (
	"context"
	"fmt"
)

// Await waits for one inbound event, timeout, or shutdown.
func (s *ServerState) Await(name string) (InboundEvent, string, error) {
	return s.AwaitContext(context.Background(), name)
}

// AwaitContext is Await with caller-controlled cancellation.
func (s *ServerState) AwaitContext(parent context.Context, name string) (InboundEvent, string, error) {
	runtime, err := s.awaitRuntime(name)
	if err != nil {
		return InboundEvent{}, "CommandError", err
	}
	ctx, cancel := context.WithTimeout(parent, runtime.awaitTimeout())
	defer cancel()
	result := runtime.awaitMatching(ctx, awaitFilter{server: name}, StoppedSourceEmitServerStopped)
	if err := parent.Err(); err != nil {
		return InboundEvent{Source: name}, "CommandError", err
	}
	if result.signal == "" && result.err == nil {
		return InboundEvent{Source: name}, "AwaitTimedOut", nil
	}
	return result.event, result.signal, result.err
}

// AwaitAny waits across multiple launched REST server queues.
func (s *ServerState) AwaitAny(options AwaitAnyOptions) (InboundEvent, string, error) {
	return s.AwaitAnyContext(context.Background(), options)
}

// AwaitAnyContext is AwaitAny with caller-controlled cancellation.
func (s *ServerState) AwaitAnyContext(parent context.Context, options AwaitAnyOptions) (InboundEvent, string, error) {
	sources, err := s.resolveAwaitSources(options)
	if err != nil {
		return InboundEvent{}, "CommandError", err
	}
	ctx, cancel := context.WithTimeout(parent, awaitAnyTimeout(options))
	defer cancel()
	result := waitAnySource(ctx, cancel, sources)
	if err := parent.Err(); err != nil {
		return InboundEvent{}, "CommandError", err
	}
	return result.event, result.signal, result.err
}

func (s *ServerState) awaitRuntime(name string) (*serverRuntime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if runtime, ok := s.servers[name]; ok {
		return runtime, nil
	}
	if runtime, ok := s.stopped[name]; ok {
		return runtime, nil
	}
	return nil, fmt.Errorf("REST server %q is not launched", name)
}

func (s *ServerState) resolveAwaitSources(options AwaitAnyOptions) ([]resolvedAwaitSource, error) {
	if len(options.Sources) == 0 {
		return nil, fmt.Errorf("at least one REST await source is required")
	}
	sources := make([]resolvedAwaitSource, 0, len(options.Sources))
	for _, source := range options.Sources {
		runtime, err := s.awaitRuntime(source.Server)
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
