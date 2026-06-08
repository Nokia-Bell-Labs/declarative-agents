// Copyright (c) 2026 Nokia. All rights reserved.

package eval

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// Span mirrors the Go SDK stdouttrace NDJSON format.
type Span struct {
	Name        string     `json:"Name"`
	SpanContext SpanCtx    `json:"SpanContext"`
	Parent      SpanCtx    `json:"Parent"`
	StartTime   time.Time  `json:"StartTime"`
	EndTime     time.Time  `json:"EndTime"`
	Attributes  []KeyValue `json:"Attributes"`
	Events      []Event    `json:"Events"`
}

// SpanCtx holds the span identifier.
type SpanCtx struct {
	SpanID string `json:"SpanID"`
}

// KeyValue is an attribute key-value pair.
type KeyValue struct {
	Key   string    `json:"Key"`
	Value AttrValue `json:"Value"`
}

// AttrValue holds a typed attribute value.
type AttrValue struct {
	Type  string      `json:"Type"`
	Value interface{} `json:"Value"`
}

// Event is a span event with attributes.
type Event struct {
	Name       string     `json:"Name"`
	Attributes []KeyValue `json:"Attributes"`
	Time       time.Time  `json:"Time"`
}

// ReadTraceFile parses an NDJSON trace file into spans.
func ReadTraceFile(path string) ([]*Span, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read trace: %w", err)
	}
	return ParseNDJSON(data)
}

// ParseNDJSON extracts spans from NDJSON-encoded trace data.
func ParseNDJSON(data []byte) ([]*Span, error) {
	var spans []*Span
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var s Span
		if err := json.Unmarshal(line, &s); err != nil {
			continue
		}
		if s.Name == "" && s.SpanContext.SpanID == "" {
			continue
		}
		spans = append(spans, &s)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan trace: %w", err)
	}
	return spans, nil
}

// StrAttr returns a string attribute value by key.
func StrAttr(s *Span, key string) string {
	for _, a := range s.Attributes {
		if a.Key == key {
			if sv, ok := a.Value.Value.(string); ok {
				return sv
			}
		}
	}
	return ""
}

// IntAttr returns an integer attribute value by key.
func IntAttr(s *Span, key string) int {
	for _, a := range s.Attributes {
		if a.Key == key {
			switch v := a.Value.Value.(type) {
			case float64:
				return int(v)
			case int64:
				return int(v)
			case json.Number:
				n, _ := v.Int64()
				return int(n)
			}
		}
	}
	return 0
}

// HasAttr returns true if the span has the given attribute key.
func HasAttr(s *Span, key string) bool {
	for _, a := range s.Attributes {
		if a.Key == key {
			return true
		}
	}
	return false
}

// AgentVersion extracts gen_ai.agent.version from the first span that carries it.
func AgentVersion(spans []*Span) string {
	for _, s := range spans {
		if v := StrAttr(s, "gen_ai.agent.version"); v != "" {
			return v
		}
	}
	return ""
}

// ToolSpans returns spans representing tool executions, sorted by start time.
func ToolSpans(spans []*Span) []*Span {
	var tools []*Span
	for _, s := range spans {
		if HasAttr(s, "command.name") && HasAttr(s, "command.signal") {
			tools = append(tools, s)
		}
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].StartTime.Before(tools[j].StartTime)
	})
	return tools
}
