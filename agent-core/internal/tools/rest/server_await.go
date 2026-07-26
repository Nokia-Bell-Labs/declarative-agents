// Copyright (c) 2026 Nokia. All rights reserved.

package rest

import "context"

// Await waits for one inbound event, timeout, or shutdown.
func (s *ServerState) Await(name string) (InboundEvent, string, error) {
	return s.AwaitContext(context.Background(), name)
}

// AwaitContext is Await with caller-controlled cancellation.
func (s *ServerState) AwaitContext(parent context.Context, name string) (InboundEvent, string, error) {
	runtime, err := s.runtime(name)
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
