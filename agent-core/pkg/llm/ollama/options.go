// Copyright (c) 2026 Nokia. All rights reserved.

package ollama

import (
	"net/http"

	"gitlabe1.ext.net.nokia.com/proof-of-concepts/agent-core/pkg/tracing"
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

// WithSkipModelCheck disables the startup model-availability check.
// Useful when the adapter is created only for ListModels or other
// operations that don't require a specific model.
func WithSkipModelCheck() Option {
	return func(a *Adapter) { a.skipModelCheck = true }
}
