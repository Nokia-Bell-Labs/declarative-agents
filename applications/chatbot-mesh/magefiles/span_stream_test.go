// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The exporter's output shape is the thing these tests pin. A reader that
// handles only one of them turns a real trace into an empty slice, and an empty
// span list reads exactly like a turn that never ran the word being asserted
// (GH-85).

const prettyPrintedSpan = `{
	"Name": "execute_tool invoke_command",
	"StartTime": "2026-08-25T12:00:01Z",
	"SpanContext": {
		"TraceID": "trace-a",
		"SpanID": "span-2"
	},
	"Parent": {
		"TraceID": "trace-a",
		"SpanID": "span-1"
	},
	"Attributes": [
		{
			"Key": "command.name",
			"Value": {
				"Value": "invoke_command"
			}
		}
	]
}`

const lineDelimitedSpan = `{"Name":"machine_request chat","StartTime":"2026-08-25T12:00:00Z","SpanContext":{"TraceID":"trace-a","SpanID":"span-1"},"Attributes":[{"Key":"command.name","Value":{"Value":"parse_tier"}}]}`

func writeTrace(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "trace.ndjson")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadChromaSpansReadsPrettyPrintedTraces(t *testing.T) {
	spans, err := readChromaSpans(writeTrace(t, prettyPrintedSpan+"\n"))
	if err != nil {
		t.Fatalf("pretty-printed trace rejected: %v", err)
	}
	if len(spans) != 1 || spans[0].commandName() != "invoke_command" {
		t.Fatalf("spans = %+v, want one invoke_command span", spans)
	}
}

func TestReadChromaSpansReadsLineDelimitedTraces(t *testing.T) {
	content := lineDelimitedSpan + "\n" + lineDelimitedSpan + "\n"
	spans, err := readChromaSpans(writeTrace(t, content))
	if err != nil {
		t.Fatalf("line-delimited trace rejected: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2", len(spans))
	}
}

// The exporter writes one shape per run, but a reader that assumes a shape is
// the defect being fixed, so neither shape may depend on the other's absence.
func TestReadChromaSpansReadsBothShapesInOneTrace(t *testing.T) {
	content := lineDelimitedSpan + "\n" + prettyPrintedSpan + "\n"
	spans, err := readChromaSpans(writeTrace(t, content))
	if err != nil {
		t.Fatalf("mixed trace rejected: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("spans = %d, want 2", len(spans))
	}
}

// readChromaSpans orders by start time; assertConnectedTrace and the tier
// assertion both walk spans expecting the turn to precede what it dispatched.
func TestReadChromaSpansOrdersByStartTime(t *testing.T) {
	content := prettyPrintedSpan + "\n" + lineDelimitedSpan + "\n"
	spans, err := readChromaSpans(writeTrace(t, content))
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 2 || spans[0].SpanContext.SpanID != "span-1" {
		t.Fatalf("spans out of start order: %+v", spans)
	}
}

// The defect this issue records: a trace that decodes to nothing must not read
// as a successful empty result.
func TestDecodeSpanStreamRejectsATraceThatDecodesToNothing(t *testing.T) {
	for _, test := range []struct{ name, content string }{
		{"empty file", ""},
		{"whitespace only", "\n\n   \n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			spans, err := readChromaSpans(writeTrace(t, test.content))
			if err == nil {
				t.Fatalf("a trace with no spans was accepted, spans = %+v", spans)
			}
			if !strings.Contains(err.Error(), "decoded no spans") {
				t.Errorf("error does not say the trace decoded to nothing: %v", err)
			}
		})
	}
}

// A corrupt log ends the read with an error rather than a silent prefix. These
// traces are read after the agent exits, so a value that does not decode is a
// damaged file, not one still being written.
func TestDecodeSpanStreamRejectsAMalformedTail(t *testing.T) {
	content := lineDelimitedSpan + "\n{\"Name\": \"truncated\"" + "\n"
	spans, err := readChromaSpans(writeTrace(t, content))
	if err == nil {
		t.Fatalf("a malformed trace was accepted, spans = %+v", spans)
	}
	if !strings.Contains(err.Error(), "after 1 span(s)") {
		t.Errorf("error does not say how far the read got: %v", err)
	}
}

func TestDecodeSpanStreamReportsAMissingFile(t *testing.T) {
	_, err := readChromaSpans(filepath.Join(t.TempDir(), "absent.ndjson"))
	if err == nil {
		t.Fatal("a missing trace file was accepted")
	}
	if !strings.Contains(err.Error(), "read trace") {
		t.Errorf("error does not name the read failure: %v", err)
	}
}

// The caller audit this issue asks for, as behaviour rather than as a reading
// of the code. Every assertion over a span log must fail when the log yields
// nothing; the reader now errors first, and these pin that none of them draws
// a conclusion from an unreadable trace.
func TestSpanAssertionsFailOnAnUnreadableTrace(t *testing.T) {
	empty := writeTrace(t, "")
	populated := writeTrace(t, lineDelimitedSpan+"\n")

	if err := assertChromaIngestTrace(empty); err == nil {
		t.Error("assertChromaIngestTrace accepted a trace with no spans")
	}
	if err := assertChatbotTierSelectionTrace(empty, "fast-model", "deep-model"); err == nil {
		t.Error("assertChatbotTierSelectionTrace accepted a trace with no spans")
	}
	if err := assertConnectedTrace(empty, populated); err == nil {
		t.Error("assertConnectedTrace accepted an unreadable chatbot trace")
	}
	if err := assertConnectedTrace(populated, empty); err == nil {
		t.Error("assertConnectedTrace accepted an unreadable rag-server trace")
	}
}
