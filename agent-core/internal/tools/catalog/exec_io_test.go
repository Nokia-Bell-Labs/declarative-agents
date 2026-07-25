// Copyright (c) 2026 Nokia. All rights reserved.

package catalog

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseToolDefsExecIOContract(t *testing.T) {
	defs, err := ParseToolDefs([]byte(`
tools:
  - name: consume
    type: exec
    binary: wc
    stdin_source: $from(search).output
    stdin_max_bytes: 2048
    output:
      mode: structured
`))
	require.NoError(t, err)
	require.Len(t, defs, 1)
	require.Equal(t, "$from(search).output", defs[0].StdinSource)
	require.Equal(t, 2048, defs[0].StdinMaxBytes)
	require.Equal(t, "structured", defs[0].Output.Mode)
}

func TestParseToolDefsRejectsInvalidExecIO(t *testing.T) {
	tests := []struct {
		name   string
		fields string
		want   string
	}{
		{name: "empty source", fields: "    stdin_source: \"\"\n", want: "must not be empty"},
		{name: "non selector scalar", fields: "    stdin_source: 42\n", want: "must be a $from(label).path selector"},
		{name: "malformed source", fields: "    stdin_source: $from(search\n", want: "must be a $from(label).path selector"},
		{name: "current value source", fields: "    stdin_source: $.output\n", want: "must be a $from(label).path selector"},
		{name: "limit without source", fields: "    stdin_max_bytes: 10\n", want: "requires stdin_source"},
		{name: "zero limit", fields: "    stdin_source: $from(search).output\n    stdin_max_bytes: 0\n", want: "must be positive"},
		{name: "negative limit", fields: "    stdin_source: $from(search).output\n    stdin_max_bytes: -1\n", want: "must be positive"},
		{name: "invalid output mode", fields: "    output:\n      mode: json\n", want: "must be raw or structured"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseToolDefs([]byte("tools:\n  - name: consume\n    type: exec\n    binary: wc\n" + tc.fields))
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestParseToolDefsRejectsExecIOWithoutBinary(t *testing.T) {
	_, err := ParseToolDefs([]byte(`
tools:
  - name: consume
    type: boundary
    stdin_source: $from(search).output
`))
	require.ErrorContains(t, err, "exec input or output mode requires binary")
}
