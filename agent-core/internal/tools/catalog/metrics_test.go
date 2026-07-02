// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"strings"
	"testing"
)

func TestParseToolDefs_MetricsValidAndOmitted(t *testing.T) {
	t.Parallel()
	yaml := `
tools:
  - name: plain
    type: builtin
    init: plain
  - name: rest_call
    type: builtin
    init: rest_call
    metrics:
      instruments:
        - name: rest.request_duration
          kind: histogram
          unit: ms
          description: Duration of one REST request.
          value_source: dispatch_duration
          attributes: [operation]
      attributes:
        - name: operation
          source: configured_operation
          cardinality: bounded
          allowed_values: [documentation_curator_requests.documents]
          redaction: none
`
	defs, err := ParseToolDefs([]byte(yaml))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("defs = %d, want 2", len(defs))
	}
	if got := defs[1].Metrics.Instruments[0].Kind; got != "histogram" {
		t.Fatalf("metric kind = %q", got)
	}
}

func TestParseToolDefs_MetricsRejectInvalidConfig(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "invalid metric name",
			body: "name: 9bad\nkind: counter\ndescription: Bad.\nvalue_source: dispatch_count\n",
			want: "not a valid metric name",
		},
		{
			name: "invalid kind",
			body: "name: requests\nkind: summary\ndescription: Bad.\nvalue_source: dispatch_count\n",
			want: "kind \"summary\" is unsupported",
		},
		{
			name: "unsafe source",
			body: "name: requests\nkind: counter\ndescription: Bad.\nvalue_source: raw_prompt\n",
			want: "not a safe source selector",
		},
		{
			name: "high cardinality label",
			body: "name: requests\nkind: counter\ndescription: Bad.\nvalue_source: dispatch_count\n",
			want: "cardinality \"high\" is not low or bounded",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseToolDefs([]byte(metricToolYAML(tc.body, tc.name == "high cardinality label")))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error %q missing %q", err.Error(), tc.want)
			}
		})
	}
}

func metricToolYAML(instrument string, highCardinality bool) string {
	attrs := ""
	if highCardinality {
		attrs = `
      attributes:
        - name: request
          source: configured_operation
          cardinality: high
`
	}
	return `
tools:
  - name: bad_tool
    type: builtin
    init: bad_tool
    metrics:
      instruments:
        - ` + strings.ReplaceAll(instrument, "\n", "\n          ") + attrs
}
