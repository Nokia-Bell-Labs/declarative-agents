// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"net/http"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

// Option configures an Adapter during construction.
type Option func(*Adapter)

// WithTracer sets the tracing.Tracer used for inference spans.
func WithTracer(tr tracing.Tracer) Option {
	return func(a *Adapter) { a.tracer = tr }
}

// WithHTTPClient replaces the default http.Client used for all Ollama
// API calls. Useful for testing or custom timeouts.
func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) { a.client = c }
}
