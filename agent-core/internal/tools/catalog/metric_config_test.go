// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/runtime/core"
)

func TestMonitorMetricConfig_RejectsUnsafeLabels(t *testing.T) {
	t.Parallel()
	unsafeSources := []string{
		"raw_prompt",
		"full_model_response",
		"full_tool_output",
		"secret",
		"arbitrary_url",
		"request_id",
		"timestamp",
		"stack_trace",
		"command_output",
		"user_free_text",
	}
	for _, source := range unsafeSources {
		t.Run("instrument source "+source, func(t *testing.T) {
			t.Parallel()
			_, err := ParseToolDefs([]byte(metricToolDefYAML(source, "low", "")))
			require.ErrorContains(t, err, `sample.metrics.instruments[0].value_source`)
			require.ErrorContains(t, err, source)
		})
	}

	tests := []struct {
		name        string
		cardinality string
		redaction   string
		wantField   string
	}{
		{name: "unbounded cardinality", cardinality: "unbounded", wantField: ".cardinality"},
		{name: "bounded without allowlist", cardinality: "bounded", wantField: ".allowed_values"},
		{name: "unsupported redaction", cardinality: "low", redaction: "plaintext", wantField: ".redaction"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseToolDefs([]byte(metricToolDefYAML("status", tt.cardinality, tt.redaction)))
			require.ErrorContains(t, err, "sample.metrics.attributes[0]"+tt.wantField)
		})
	}

	for _, value := range []string{"raw_prompt", "request_id", "timestamp", strings.Repeat("x", 129), ""} {
		t.Run("machine label "+value, func(t *testing.T) {
			t.Parallel()
			machine := fmt.Sprintf(`
name: unsafe-label
initial_state: Start
states: [Start, Done]
terminal_states: [Done]
signals: [Seed]
metric_labels: {workflow: %q}
transitions:
  - {state: Start, signal: Seed, next: Done}
`, value)
			_, err := core.ParseMachineSpec([]byte(machine))
			require.ErrorContains(t, err, "metric_labels.workflow")
			require.ErrorContains(t, err, "not a safe metric label")
		})
	}
}

func metricToolDefYAML(source, cardinality, redaction string) string {
	return fmt.Sprintf(`
tools:
  - name: sample
    binary: echo
    metrics:
      instruments:
        - name: sample.duration
          kind: histogram
          description: Sample duration.
          value_source: %q
      attributes:
        - name: status
          source: status
          cardinality: %q
          redaction: %q
`, source, cardinality, redaction)
}
