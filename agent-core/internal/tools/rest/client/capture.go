// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package client

import (
	"encoding/json"
	"io"
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/observability/tracing"
)

// Content capture for REST client dispatch spans (srd028 R9.5-R9.7). An LLM
// invoked through a declared REST operation carries its prompt in a request
// body, which byte counts alone leave unrecoverable.
const (
	// captureBodyLimit bounds each recorded body: large enough for any prompt,
	// small enough that an embedding batch does not bloat the span.
	captureBodyLimit = 16384

	attrRequestBody           = "http.request.body"
	attrRequestBodyTruncated  = "http.request.body.truncated"
	attrResponseBody          = "http.response.body"
	attrResponseBodyTruncated = "http.response.body.truncated"
)

// bodyCapture records redacted request and response bodies on the dispatch
// span. It is inert unless the composition root enabled telemetry capture
// full, so off and delta carry neither attribute (R9.5).
type bodyCapture struct {
	tracer  tracing.Tracer
	enabled bool
}

func newBodyCapture(enabled bool) bodyCapture {
	return bodyCapture{tracer: tracing.NoopTracer{}, enabled: enabled}
}

// setTracer receives the dispatch child tracer. A nil tracer keeps the noop so
// a capture-enabled command never dereferences one that was never injected.
func (b *bodyCapture) setTracer(tracer tracing.Tracer) {
	if tracer == nil {
		b.tracer = tracing.NoopTracer{}
		return
	}
	b.tracer = tracer
}

// recordRequest records the rendered request body. Selectors come from the
// operation's success mapping: the status is not known at dispatch, and a
// request whose call then fails is the one most worth having on the span.
func (b bodyCapture) recordRequest(request *http.Request, selectors []string) {
	if !b.enabled || request == nil || request.GetBody == nil {
		return
	}
	reader, err := request.GetBody()
	if err != nil {
		return
	}
	defer func() { _ = reader.Close() }()
	raw, err := io.ReadAll(io.LimitReader(reader, captureBodyLimit+1))
	if err != nil {
		return
	}
	b.record(raw, selectors, attrRequestBody, attrRequestBodyTruncated)
}

// recordResponse records the mapped response body from the bytes the mapping
// already read, using that status mapping's own selectors.
func (b bodyCapture) recordResponse(raw []byte, selectors []string) {
	if !b.enabled {
		return
	}
	b.record(raw, selectors, attrResponseBody, attrResponseBodyTruncated)
}

func (b bodyCapture) record(raw []byte, selectors []string, bodyKey, truncatedKey string) {
	encoded, ok := redactCapturedBody(raw, selectors)
	if !ok {
		return
	}
	if len(encoded) > captureBodyLimit {
		encoded = encoded[:captureBodyLimit]
		b.tracer.SetAttributes(attribute.Bool(truncatedKey, true))
	}
	b.tracer.SetAttributes(attribute.String(bodyKey, string(encoded)))
}

// redactCapturedBody applies the operation's declared redaction selectors to a
// body before it reaches a span, so R9.3 holds for captured content. A body
// that is not a JSON object is not captured at all: field-wise redaction
// cannot apply to it, and recording it raw would defeat the rule (R9.6).
func redactCapturedBody(raw []byte, selectors []string) ([]byte, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	decoded := map[string]interface{}{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false
	}
	redactClientOutput(map[string]interface{}{"body": decoded}, selectors)
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return nil, false
	}
	return encoded, true
}
