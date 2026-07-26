// Copyright (c) 2026 Nokia. All rights reserved.

// Package conformance runs each profile family through the agent CLI with
// OpenTelemetry file export enabled and asserts on the emitted trace.
//
// agent-profiles and agent-core are separate Go modules, so this package
// cannot import agent-core's telemetry types. It carries its own small parser
// for the stdouttrace file format agent-core writes for --otel-log-file: a
// stream of JSON objects, one span (SpanStub) per object, interleaved with the
// metric exporter's own objects. The parser keeps span-shaped objects (those
// with a span ID) and ignores everything else, using only the standard library.
package conformance

import (
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

// zeroSpanID is what the OTel SDK marshals for an unset span or parent: 16 hex
// zeros rather than an empty string.
const zeroSpanID = "0000000000000000"

// hasID reports whether id is a real (non-empty, non-zero) span identifier.
func hasID(id string) bool { return id != "" && id != zeroSpanID }

// Spans is a queryable collection of parsed spans.
type Spans []Span

// ParseSpansFile reads path and returns the spans it contains.
func ParseSpansFile(path string) (Spans, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open trace file %s: %w", path, err)
	}
	defer f.Close()
	spans, err := ParseSpans(f)
	if err != nil {
		return nil, fmt.Errorf("parse trace file %s: %w", path, err)
	}
	return spans, nil
}

// ParseSpans decodes the stream of JSON objects in r and returns those that are
// spans (identified by a non-empty span ID). Non-span objects, such as the
// metric exporter's output written to the same file, are skipped.
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
		var span Span
		if err := json.Unmarshal(raw, &span); err != nil {
			// A non-span object (e.g. metrics) may not fit the span shape;
			// ignore it rather than failing the whole parse.
			continue
		}
		if !hasID(span.SpanContext.SpanID) {
			continue
		}
		spans = append(spans, span)
	}
	return spans, nil
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
