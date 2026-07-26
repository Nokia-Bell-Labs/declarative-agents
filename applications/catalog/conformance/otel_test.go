// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"strings"
	"testing"
)

// synthetic mirrors the stdouttrace file format: a stream of JSON objects, one
// span per object, interleaved with a metric object the parser must skip.
const synthetic = `
{"Name":"agent.run","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331"},"Parent":{"TraceID":"00000000000000000000000000000000","SpanID":"0000000000000000"},"Attributes":[{"Key":"exporter.file_enabled","Value":{"Type":"BOOL","Value":true}}],"Events":[{"Name":"run complete","Attributes":[]}],"Status":{"Code":"Unset","Description":""}}
{"Resource":[{"Key":"service.name","Value":{"Type":"STRING","Value":"agent"}}],"ScopeMetrics":[{"Metrics":[{"Name":"dispatch_count"}]}]}
{"Name":"execute_tool load_corpus","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"c1c2c3c4c5c6c7c8"},"Parent":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331"},"Attributes":[{"Key":"gen_ai.tool.name","Value":{"Type":"STRING","Value":"load_corpus"}}],"Status":{"Code":"Unset","Description":""}}
{"Name":"execute_tool validate_specs","SpanContext":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"d1d2d3d4d5d6d7d8"},"Parent":{"TraceID":"0af7651916cd43dd8448eb211c80319c","SpanID":"b7ad6b7169203331"},"Attributes":[{"Key":"gen_ai.tool.name","Value":{"Type":"STRING","Value":"validate_specs"}}],"Status":{"Code":"Error","Description":"boom"}}
`

func TestParseSpansSkipsNonSpanObjects(t *testing.T) {
	spans, err := ParseSpans(strings.NewReader(synthetic))
	if err != nil {
		t.Fatalf("ParseSpans: %v", err)
	}
	if len(spans) != 3 {
		t.Fatalf("parsed %d spans, want 3 (metric object must be skipped): %v", len(spans), spans.Names())
	}
}

func TestSpansRootAndQueries(t *testing.T) {
	spans, err := ParseSpans(strings.NewReader(synthetic))
	if err != nil {
		t.Fatalf("ParseSpans: %v", err)
	}

	root, ok := spans.Root()
	if !ok {
		t.Fatal("expected a single root span")
	}
	if root.Name != "agent.run" {
		t.Fatalf("root span = %q, want agent.run", root.Name)
	}
	if !root.HasEvent("run complete") {
		t.Fatal("root span missing 'run complete' event")
	}

	if got := len(spans.Named("agent.run")); got != 1 {
		t.Fatalf("Named(agent.run) = %d, want 1", got)
	}
	if got := len(spans.NamePrefixed("execute_tool")); got != 2 {
		t.Fatalf("NamePrefixed(execute_tool) = %d, want 2", got)
	}

	errored := spans.Errored()
	if len(errored) != 1 {
		t.Fatalf("Errored() = %d, want 1: %v", len(errored), errored.Names())
	}
	if errored[0].Name != "execute_tool validate_specs" {
		t.Fatalf("errored span = %q, want execute_tool validate_specs", errored[0].Name)
	}
}

func TestSpanStringAttr(t *testing.T) {
	spans, err := ParseSpans(strings.NewReader(synthetic))
	if err != nil {
		t.Fatalf("ParseSpans: %v", err)
	}
	tool := spans.Named("execute_tool load_corpus")
	if len(tool) != 1 {
		t.Fatalf("expected one load_corpus span, got %d", len(tool))
	}
	name, ok := tool[0].StringAttr("gen_ai.tool.name")
	if !ok || name != "load_corpus" {
		t.Fatalf("StringAttr(gen_ai.tool.name) = %q,%v, want load_corpus,true", name, ok)
	}
	if _, ok := tool[0].StringAttr("missing"); ok {
		t.Fatal("StringAttr(missing) returned ok=true")
	}
}
