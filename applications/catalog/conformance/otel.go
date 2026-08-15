// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

// Package conformance runs each profile family through the agent CLI with
// OpenTelemetry file export enabled and asserts on the emitted trace.
//
// applications/catalog and agent-core are separate Go modules, so this package
// carries a small standard-library projection of the stdouttrace JSON fields
// its assertions use. Span-shaped objects are decoded strictly; metric objects
// interleaved in the same stream are skipped by their distinct top-level shape.
package conformance

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

// Span is the projection of a stdouttrace SpanStub that the conformance
// assertions need: identity, parentage, attributes, events, and status.
type Span struct {
	Name        string
	SpanContext SpanContext
	Parent      SpanContext
	Attributes  []Attribute
	Events      []Event
	Status      Status
}

// SpanContext carries the identifiers the parser uses to tell spans apart from
// metric objects and to find the root span (the one with no parent).
type SpanContext struct {
	TraceID string
	SpanID  string
}

// Attribute mirrors the JSON shape of an OTel attribute.KeyValue.
type Attribute struct {
	Key   string
	Value AttrValue
}

// AttrValue mirrors the JSON shape of an OTel attribute value: a Type tag and
// the underlying value (string, int64, bool, ...).
type AttrValue struct {
	Type  string
	Value any
}

// Event mirrors the JSON shape of an OTel span event.
type Event struct {
	Name       string
	Attributes []Attribute
}

// Status mirrors the JSON shape of an OTel span status. Code is one of
// "Unset", "Ok", or "Error".
type Status struct {
	Code        string
	Description string
}

// StatusError is the status code the OTel SDK marshals for a span whose status
// was set to error.
const StatusError = "Error"

// TerminalEventName is the event agent-core's loop runner records on the agent
// span when the state machine reaches a terminal state
// (internal/runtime/core/loop_runner.go). Its attributes carry final_state and
// status.
const TerminalEventName = "run.terminal"

// hasID reports whether id is a real (non-empty, non-zero) identifier.
func hasID(id string) bool { return id != "" && !allZero(id) }

func allZero(id string) bool {
	for _, c := range id {
		if c != '0' {
			return false
		}
	}
	return true
}

// Spans is a queryable collection of parsed spans.
type Spans []Span

// ParseSpansFile reads path and returns the spans it contains.
func ParseSpansFile(path string) (Spans, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open trace file %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	spans, err := ParseSpans(f)
	if err != nil {
		return nil, fmt.Errorf("parse trace file %s: %w", path, err)
	}
	return spans, nil
}

// ParseSpans decodes the stream of stdouttrace and metric JSON objects. Metric
// objects are skipped, while span-shaped objects are decoded and validated
// strictly so a field rename or malformed identifier fails at the parser
// boundary rather than degrading into a confusing "zero spans" assertion.
func ParseSpans(r io.Reader) (Spans, error) {
	dec := json.NewDecoder(r)
	var spans Spans
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		span, ok, err := decodeSpanObject(raw)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		spans = append(spans, span)
	}
	return spans, nil
}

func decodeSpanObject(raw json.RawMessage) (Span, bool, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return Span{}, false, fmt.Errorf("decode telemetry object: %w", err)
	}
	_, hasContext := fields["SpanContext"]
	if !hasContext {
		if spanLike(fields) {
			return Span{}, false, fmt.Errorf("span-shaped object is missing SpanContext")
		}
		return Span{}, false, nil
	}
	var span Span
	if err := json.Unmarshal(raw, &span); err != nil {
		return Span{}, false, fmt.Errorf("decode span object: %w", err)
	}
	if err := validateSpan(span); err != nil {
		return Span{}, false, err
	}
	return span, true, nil
}

func spanLike(fields map[string]json.RawMessage) bool {
	for _, key := range []string{"Name", "Parent", "Events", "Status"} {
		if _, ok := fields[key]; ok {
			return true
		}
	}
	return false
}

func validateSpan(span Span) error {
	if span.Name == "" {
		return fmt.Errorf("span object has an empty Name")
	}
	if err := validateSpanContext("SpanContext", span.SpanContext, false); err != nil {
		return fmt.Errorf("span %q: %w", span.Name, err)
	}
	if err := validateSpanContext("Parent", span.Parent, true); err != nil {
		return fmt.Errorf("span %q: %w", span.Name, err)
	}
	switch span.Status.Code {
	case "", "Unset", "Ok", "Error":
		return nil
	default:
		return fmt.Errorf("span %q: unsupported status code %q", span.Name, span.Status.Code)
	}
}

func validateSpanContext(field string, ctx SpanContext, allowZero bool) error {
	if allowZero && !hasID(ctx.TraceID) && !hasID(ctx.SpanID) {
		return nil
	}
	if err := validateHexID(field+".TraceID", ctx.TraceID, 16); err != nil {
		return err
	}
	return validateHexID(field+".SpanID", ctx.SpanID, 8)
}

func validateHexID(field, id string, byteLen int) error {
	decoded, err := hex.DecodeString(id)
	if err != nil || len(decoded) != byteLen || allZero(id) {
		return fmt.Errorf("%s %q is not a non-zero %d-byte hex ID", field, id, byteLen)
	}
	return nil
}

// Named returns the spans whose name equals name.
func (s Spans) Named(name string) Spans {
	var out Spans
	for _, span := range s {
		if span.Name == name {
			out = append(out, span)
		}
	}
	return out
}

// NamePrefixed returns the spans whose name starts with prefix. The genai span
// vocabulary is "<operation> <subject>" (e.g. "execute_tool load_corpus"), so
// callers match a family of spans by their operation prefix.
func (s Spans) NamePrefixed(prefix string) Spans {
	var out Spans
	for _, span := range s {
		if len(span.Name) >= len(prefix) && span.Name[:len(prefix)] == prefix {
			out = append(out, span)
		}
	}
	return out
}

// Errored returns the spans whose status code is "Error".
func (s Spans) Errored() Spans {
	var out Spans
	for _, span := range s {
		if span.Status.Code == StatusError {
			out = append(out, span)
		}
	}
	return out
}

// Root returns the span with no parent span ID, if exactly one is present.
func (s Spans) Root() (Span, bool) {
	var root Span
	found := false
	for _, span := range s {
		if !hasID(span.Parent.SpanID) {
			if found {
				return Span{}, false
			}
			root = span
			found = true
		}
	}
	return root, found
}

// Names returns the span names in order, for diagnostic messages.
func (s Spans) Names() []string {
	names := make([]string, 0, len(s))
	for _, span := range s {
		names = append(names, span.Name)
	}
	return names
}

// Attribute returns the attribute with the given key.
func (span Span) Attribute(key string) (AttrValue, bool) {
	for _, attr := range span.Attributes {
		if attr.Key == key {
			return attr.Value, true
		}
	}
	return AttrValue{}, false
}

// StringAttr returns the string form of the span attribute with the given key.
func (span Span) StringAttr(key string) (string, bool) {
	return attrString(span.Attributes, key)
}

// HasEvent reports whether the span carries an event with the given name.
func (span Span) HasEvent(name string) bool {
	for _, event := range span.Events {
		if event.Name == name {
			return true
		}
	}
	return false
}

// StringAttr returns the string form of the event attribute with the given key.
func (e Event) StringAttr(key string) (string, bool) {
	return attrString(e.Attributes, key)
}

// FindEvent returns the first event with the given name across all spans, along
// with the span that carries it.
func (s Spans) FindEvent(name string) (Event, Span, bool) {
	for _, span := range s {
		for _, event := range span.Events {
			if event.Name == name {
				return event, span, true
			}
		}
	}
	return Event{}, Span{}, false
}

func attrString(attrs []Attribute, key string) (string, bool) {
	for _, attr := range attrs {
		if attr.Key == key {
			return fmt.Sprint(attr.Value.Value), true
		}
	}
	return "", false
}
