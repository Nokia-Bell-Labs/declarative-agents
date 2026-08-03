// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"net/http"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

// Option configures an Adapter during construction.
type Option func(*Adapter)

// WithTracer sets the tracing.Tracer used for span events during
// model checks and list-models calls.
func WithTracer(tr tracing.Tracer) Option {
	return func(a *Adapter) { a.tracer = tr }
}

// WithHTTPClient replaces the default http.Client used for all Ollama
// API calls. Useful for testing or custom timeouts.
func WithHTTPClient(c *http.Client) Option {
	return func(a *Adapter) { a.client = c }
}

// WithSkipModelCheck disables the startup model-availability check. Callers
// that probe availability elsewhere (for example a declared rest_client_invoke
// word) skip the check so a dead backend surfaces as a routable machine
// transition rather than an adapter-construction failure.
func WithSkipModelCheck() Option {
	return func(a *Adapter) { a.skipModelCheck = true }
}
