// Copyright (c) 2026 Nokia. All rights reserved.

// Package conformance runs each profile family through the agent CLI with
// OpenTelemetry file export enabled and asserts on the emitted trace.
//
// agent-core writes the stdouttrace file format for --otel-log-file: a stream
// of JSON objects, one span per object (a marshaled
// go.opentelemetry.io/otel/sdk/trace/tracetest.SpanStub), interleaved with the
// metric exporter's own objects. That SpanStub is a public, versioned SDK type,
// so this package decodes into it rather than mirroring the model — an upstream
// field rename then breaks the build here instead of silently degrading to
// "zero spans parsed". The one shim: trace.SpanContext and attribute.Value
// marshal to JSON but define no UnmarshalJSON (their fields are unexported), so
// spanStubFromWire reads the marshaled shape and rebuilds those fields with the
// SDK constructors. Span-shaped objects that fail to decode are reported, not
// skipped; only objects with no span ID (the metric exporter's) are ignored.
// The parsed SpanStubs are projected onto the small Span view the assertions
// use, keeping the query surface stable across SDK versions.
package conformance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"
)

// Span is the projection of a tracetest.SpanStub that the conformance
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

// hasID reports whether id is a real identifier. The OTel SDK marshals an unset
// trace or span id as all hex zeros (32 or 16 respectively) rather than an empty
// string, and its *FromHex constructors reject an all-zero id, so both the empty
// and all-zero forms count as absent.
func hasID(id string) bool { return !isZeroHex(id) }

// isZeroHex reports whether id is empty or all hex zeros, of any length.
func isZeroHex(id string) bool {
	if id == "" {
		return true
	}
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

// ParseSpans decodes the stream of JSON objects in r into tracetest.SpanStubs
// and returns them projected onto the Span view. Objects with no span ID (the
// metric exporter's output written to the same file) are skipped; a span-shaped
// object that fails to decode is reported rather than silently dropped.
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
		stub, ok, err := spanStubFromWire(raw)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		spans = append(spans, projectSpan(stub))
	}
	return spans, nil
}

// wireSpan mirrors the JSON shape stdouttrace marshals for the SpanStub fields
// the assertions read. It exists only because trace.SpanContext and
// attribute.Value have MarshalJSON but no UnmarshalJSON; the values are rebuilt
// into the SDK type in spanStubFromWire.
type wireSpan struct {
	Name        string          `json:"Name"`
	SpanContext wireSpanContext `json:"SpanContext"`
	Parent      wireSpanContext `json:"Parent"`
	Attributes  []wireAttribute `json:"Attributes"`
	Events      []wireEvent     `json:"Events"`
	Status      struct {
		Code        string `json:"Code"`
		Description string `json:"Description"`
	} `json:"Status"`
}

type wireSpanContext struct {
	TraceID string `json:"TraceID"`
	SpanID  string `json:"SpanID"`
}

type wireAttribute struct {
	Key   string `json:"Key"`
	Value struct {
		Type  string `json:"Type"`
		Value any    `json:"Value"`
	} `json:"Value"`
}

type wireEvent struct {
	Name       string          `json:"Name"`
	Attributes []wireAttribute `json:"Attributes"`
}

// spanStubFromWire decodes one JSON object into a tracetest.SpanStub. ok is
// false for objects with no span ID (metrics); err is non-nil for a span-shaped
// object whose identifiers do not decode, so a format drift fails loudly.
func spanStubFromWire(raw json.RawMessage) (sdktrace.SpanStub, bool, error) {
	var w wireSpan
	if err := json.Unmarshal(raw, &w); err != nil {
		// A non-span object (e.g. metrics) need not fit the span shape.
		return sdktrace.SpanStub{}, false, nil
	}
	if !hasID(w.SpanContext.SpanID) {
		return sdktrace.SpanStub{}, false, nil
	}
	spanCtx, err := wireSpanContextTo(w.SpanContext)
	if err != nil {
		return sdktrace.SpanStub{}, false, fmt.Errorf("span %q context: %w", w.Name, err)
	}
	parent, err := wireSpanContextTo(w.Parent)
	if err != nil {
		return sdktrace.SpanStub{}, false, fmt.Errorf("span %q parent: %w", w.Name, err)
	}
	return sdktrace.SpanStub{
		Name:        w.Name,
		SpanContext: spanCtx,
		Parent:      parent,
		Attributes:  wireAttributesTo(w.Attributes),
		Events:      wireEventsTo(w.Events),
		Status:      tracesdk.Status{Code: statusCodeFromWire(w.Status.Code), Description: w.Status.Description},
	}, true, nil
}

// wireSpanContextTo rebuilds a trace.SpanContext from its marshaled hex ids. A
// zero id (unset parent) stays the zero value of the corresponding SDK type.
func wireSpanContextTo(w wireSpanContext) (oteltrace.SpanContext, error) {
	cfg := oteltrace.SpanContextConfig{}
	if hasID(w.TraceID) {
		traceID, err := oteltrace.TraceIDFromHex(w.TraceID)
		if err != nil {
			return oteltrace.SpanContext{}, fmt.Errorf("trace id %q: %w", w.TraceID, err)
		}
		cfg.TraceID = traceID
	}
	if hasID(w.SpanID) {
		spanID, err := oteltrace.SpanIDFromHex(w.SpanID)
		if err != nil {
			return oteltrace.SpanContext{}, fmt.Errorf("span id %q: %w", w.SpanID, err)
		}
		cfg.SpanID = spanID
	}
	return oteltrace.NewSpanContext(cfg), nil
}

func wireAttributesTo(in []wireAttribute) []attribute.KeyValue {
	if len(in) == 0 {
		return nil
	}
	out := make([]attribute.KeyValue, 0, len(in))
	for _, a := range in {
		out = append(out, attributeFromWire(a))
	}
	return out
}

// attributeFromWire rebuilds a typed attribute.KeyValue from the marshaled
// {Type, Value} shape. Unknown types fall back to a string so a new OTel value
// kind is still surfaced (as text) rather than dropped.
func attributeFromWire(a wireAttribute) attribute.KeyValue {
	switch a.Value.Type {
	case "BOOL":
		b, _ := a.Value.Value.(bool)
		return attribute.Bool(a.Key, b)
	case "INT64":
		f, _ := a.Value.Value.(float64)
		return attribute.Int64(a.Key, int64(f))
	case "FLOAT64":
		f, _ := a.Value.Value.(float64)
		return attribute.Float64(a.Key, f)
	case "STRING":
		s, _ := a.Value.Value.(string)
		return attribute.String(a.Key, s)
	default:
		return attribute.String(a.Key, fmt.Sprint(a.Value.Value))
	}
}

func wireEventsTo(in []wireEvent) []tracesdk.Event {
	if len(in) == 0 {
		return nil
	}
	out := make([]tracesdk.Event, 0, len(in))
	for _, e := range in {
		out = append(out, tracesdk.Event{Name: e.Name, Attributes: wireAttributesTo(e.Attributes)})
	}
	return out
}

// statusCodeFromWire maps the marshaled status string to the SDK code. stdout
// trace marshals codes.Code as its String() form.
func statusCodeFromWire(code string) codes.Code {
	switch code {
	case "Ok":
		return codes.Ok
	case "Error":
		return codes.Error
	default:
		return codes.Unset
	}
}

// projectSpan reduces a tracetest.SpanStub to the Span view the assertions read.
func projectSpan(stub sdktrace.SpanStub) Span {
	return Span{
		Name:        stub.Name,
		SpanContext: SpanContext{TraceID: stub.SpanContext.TraceID().String(), SpanID: stub.SpanContext.SpanID().String()},
		Parent:      SpanContext{TraceID: stub.Parent.TraceID().String(), SpanID: stub.Parent.SpanID().String()},
		Attributes:  projectAttributes(stub.Attributes),
		Events:      projectEvents(stub.Events),
		Status:      Status{Code: stub.Status.Code.String(), Description: stub.Status.Description},
	}
}

func projectAttributes(in []attribute.KeyValue) []Attribute {
	if len(in) == 0 {
		return nil
	}
	out := make([]Attribute, 0, len(in))
	for _, kv := range in {
		out = append(out, Attribute{
			Key:   string(kv.Key),
			Value: AttrValue{Type: kv.Value.Type().String(), Value: kv.Value.AsInterface()},
		})
	}
	return out
}

func projectEvents(in []tracesdk.Event) []Event {
	if len(in) == 0 {
		return nil
	}
	out := make([]Event, 0, len(in))
	for _, e := range in {
		out = append(out, Event{Name: e.Name, Attributes: projectAttributes(e.Attributes)})
	}
	return out
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
