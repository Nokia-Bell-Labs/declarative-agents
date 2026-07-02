// Copyright (c) 2026 Nokia. All rights reserved.

package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeSpan(name string, attrs map[string]interface{}) *Span {
	var kvs []KeyValue
	for k, v := range attrs {
		kv := KeyValue{Key: k}
		switch val := v.(type) {
		case string:
			kv.Value = AttrValue{Type: "STRING", Value: val}
		case int:
			kv.Value = AttrValue{Type: "INT64", Value: float64(val)}
		case float64:
			kv.Value = AttrValue{Type: "INT64", Value: val}
		}
		kvs = append(kvs, kv)
	}
	return &Span{
		Name:       name,
		Attributes: kvs,
		StartTime:  time.Now(),
		EndTime:    time.Now().Add(time.Second),
	}
}

func TestExtractToolSnapshots_WithMetrics(t *testing.T) {
	spans := []*Span{
		makeSpan("execute_tool test", map[string]interface{}{
			"command.name":        "test",
			"command.signal":      "ToolFailed",
			"tool.metrics.total":  4,
			"tool.metrics.passed": 1,
			"tool.metrics.failed": 3,
		}),
		makeSpan("execute_tool test", map[string]interface{}{
			"command.name":        "test",
			"command.signal":      "ToolFailed",
			"tool.metrics.total":  4,
			"tool.metrics.passed": 3,
			"tool.metrics.failed": 1,
		}),
		makeSpan("execute_tool test", map[string]interface{}{
			"command.name":        "test",
			"command.signal":      "ToolDone",
			"tool.metrics.total":  4,
			"tool.metrics.passed": 4,
			"tool.metrics.failed": 0,
		}),
	}
	for i, s := range spans {
		s.StartTime = time.Now().Add(time.Duration(i) * time.Minute)
		s.EndTime = s.StartTime.Add(time.Second)
	}

	snaps := ExtractToolSnapshots(spans)
	require.Len(t, snaps, 3)

	assert.Equal(t, "test", snaps[0].Tool)
	assert.Equal(t, 4, snaps[0].Total)
	assert.Equal(t, 1, snaps[0].Passed)
	assert.Equal(t, 3, snaps[0].Failed)

	assert.Equal(t, 3, snaps[1].Passed)
	assert.Equal(t, 1, snaps[1].Failed)

	assert.Equal(t, 4, snaps[2].Passed)
	assert.Equal(t, 0, snaps[2].Failed)
}

func TestExtractToolSnapshots_SkipsNonMetricTools(t *testing.T) {
	spans := []*Span{
		makeSpan("execute_tool read", map[string]interface{}{
			"command.name":   "read",
			"command.signal": "ToolDone",
		}),
		makeSpan("execute_tool write", map[string]interface{}{
			"command.name":   "write",
			"command.signal": "ToolDone",
		}),
		makeSpan("execute_tool build", map[string]interface{}{
			"command.name":        "build",
			"command.signal":      "ToolDone",
			"tool.metrics.total":  0,
			"tool.metrics.passed": 0,
			"tool.metrics.failed": 0,
		}),
	}
	for i, s := range spans {
		s.StartTime = time.Now().Add(time.Duration(i) * time.Minute)
	}

	snaps := ExtractToolSnapshots(spans)
	require.Len(t, snaps, 1)
	assert.Equal(t, "build", snaps[0].Tool)
}
