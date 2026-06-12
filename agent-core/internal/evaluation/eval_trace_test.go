// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"testing"
	"time"
)

var sampleNDJSON = []byte(`{"Name":"llm.call","SpanContext":{"SpanID":"abc123"},"Parent":{"SpanID":""},"StartTime":"2026-01-01T00:00:00Z","EndTime":"2026-01-01T00:00:01Z","Attributes":[{"Key":"gen_ai.usage.input_tokens","Value":{"Type":"INT64","Value":100}},{"Key":"gen_ai.usage.output_tokens","Value":{"Type":"INT64","Value":50}},{"Key":"gen_ai.agent.version","Value":{"Type":"STRING","Value":"v0.1.0"}}],"Events":[]}
{"Name":"execute_tool","SpanContext":{"SpanID":"def456"},"Parent":{"SpanID":"abc123"},"StartTime":"2026-01-01T00:00:02Z","EndTime":"2026-01-01T00:00:03Z","Attributes":[{"Key":"command.name","Value":{"Type":"STRING","Value":"run_tests"}},{"Key":"command.signal","Value":{"Type":"STRING","Value":"ToolDone"}}],"Events":[]}
`)

func TestParseNDJSON(t *testing.T) {
	spans, err := ParseNDJSON(sampleNDJSON)
	if err != nil {
		t.Fatalf("ParseNDJSON: %v", err)
	}
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	if spans[0].Name != "llm.call" {
		t.Errorf("span[0].Name = %q, want %q", spans[0].Name, "llm.call")
	}
	if spans[0].SpanContext.SpanID != "abc123" {
		t.Errorf("span[0].SpanID = %q, want %q", spans[0].SpanContext.SpanID, "abc123")
	}
}

func TestParseNDJSON_SkipsInvalid(t *testing.T) {
	data := []byte(`not json
{"Name":"valid","SpanContext":{"SpanID":"x1"},"StartTime":"2026-01-01T00:00:00Z","EndTime":"2026-01-01T00:00:01Z","Attributes":[],"Events":[]}
`)
	spans, err := ParseNDJSON(data)
	if err != nil {
		t.Fatalf("ParseNDJSON: %v", err)
	}
	if len(spans) != 1 {
		t.Fatalf("expected 1 span (skipped invalid), got %d", len(spans))
	}
}

func TestParseNDJSON_Empty(t *testing.T) {
	spans, err := ParseNDJSON([]byte(""))
	if err != nil {
		t.Fatalf("ParseNDJSON: %v", err)
	}
	if len(spans) != 0 {
		t.Fatalf("expected 0 spans, got %d", len(spans))
	}
}

func TestIntAttr(t *testing.T) {
	spans, _ := ParseNDJSON(sampleNDJSON)
	if v := IntAttr(spans[0], "gen_ai.usage.input_tokens"); v != 100 {
		t.Errorf("IntAttr(input_tokens) = %d, want 100", v)
	}
	if v := IntAttr(spans[0], "gen_ai.usage.output_tokens"); v != 50 {
		t.Errorf("IntAttr(output_tokens) = %d, want 50", v)
	}
	if v := IntAttr(spans[0], "nonexistent"); v != 0 {
		t.Errorf("IntAttr(nonexistent) = %d, want 0", v)
	}
}

func TestStrAttr(t *testing.T) {
	spans, _ := ParseNDJSON(sampleNDJSON)
	if v := StrAttr(spans[0], "gen_ai.agent.version"); v != "v0.1.0" {
		t.Errorf("StrAttr(agent.version) = %q, want %q", v, "v0.1.0")
	}
	if v := StrAttr(spans[0], "nonexistent"); v != "" {
		t.Errorf("StrAttr(nonexistent) = %q, want empty", v)
	}
}

func TestHasAttr(t *testing.T) {
	spans, _ := ParseNDJSON(sampleNDJSON)
	if !HasAttr(spans[1], "command.name") {
		t.Error("expected HasAttr(command.name) = true")
	}
	if HasAttr(spans[1], "nonexistent") {
		t.Error("expected HasAttr(nonexistent) = false")
	}
}

func TestAgentVersion(t *testing.T) {
	spans, _ := ParseNDJSON(sampleNDJSON)
	if v := AgentVersion(spans); v != "v0.1.0" {
		t.Errorf("AgentVersion = %q, want %q", v, "v0.1.0")
	}
}

func TestToolSpans(t *testing.T) {
	spans, _ := ParseNDJSON(sampleNDJSON)
	tools := ToolSpans(spans)
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool span, got %d", len(tools))
	}
	if tools[0].Name != "execute_tool" {
		t.Errorf("tool span name = %q, want %q", tools[0].Name, "execute_tool")
	}
}

func TestToolSpans_Sorted(t *testing.T) {
	data := []byte(`{"Name":"tool2","SpanContext":{"SpanID":"b"},"StartTime":"2026-01-01T00:00:05Z","EndTime":"2026-01-01T00:00:06Z","Attributes":[{"Key":"command.name","Value":{"Type":"STRING","Value":"b"}},{"Key":"command.signal","Value":{"Type":"STRING","Value":"done"}}],"Events":[]}
{"Name":"tool1","SpanContext":{"SpanID":"a"},"StartTime":"2026-01-01T00:00:01Z","EndTime":"2026-01-01T00:00:02Z","Attributes":[{"Key":"command.name","Value":{"Type":"STRING","Value":"a"}},{"Key":"command.signal","Value":{"Type":"STRING","Value":"done"}}],"Events":[]}
`)
	spans, _ := ParseNDJSON(data)
	tools := ToolSpans(spans)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tool spans, got %d", len(tools))
	}
	if !tools[0].StartTime.Before(tools[1].StartTime) {
		t.Error("tool spans not sorted by start time")
	}
}

func TestFormatGridPoint(t *testing.T) {
	tests := []struct {
		name string
		gp   GridPoint
		want string
	}{
		{"empty", GridPoint{}, ""},
		{"single", GridPoint{"temp": 0.7}, "temp=0.7"},
		{"multi", GridPoint{"a": 1, "b": "x"}, "a=1_b=x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatGridPoint(tt.gp)
			if got != tt.want {
				t.Errorf("FormatGridPoint(%v) = %q, want %q", tt.gp, got, tt.want)
			}
		})
	}
}

func TestEvalPointID(t *testing.T) {
	got := EvalPointID("sample1", "harness1", "model1", GridPoint{"t": 0.5}, 2)
	want := "sample1--harness1--model1--t=0.5--rep2"
	if got != want {
		t.Errorf("EvalPointID = %q, want %q", got, want)
	}

	got = EvalPointID("s", "h", "m", GridPoint{}, 1)
	want = "s--h--m--rep1"
	if got != want {
		t.Errorf("EvalPointID (no grid) = %q, want %q", got, want)
	}
}

func TestSpanTimes(t *testing.T) {
	spans, _ := ParseNDJSON(sampleNDJSON)
	expected := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if !spans[0].StartTime.Equal(expected) {
		t.Errorf("span StartTime = %v, want %v", spans[0].StartTime, expected)
	}
}
