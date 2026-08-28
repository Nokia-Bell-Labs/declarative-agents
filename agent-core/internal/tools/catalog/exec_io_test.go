// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package catalog

import (
	"strings"
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

func TestParseToolDefsExecEnvContract(t *testing.T) {
	defs, err := ParseToolDefs([]byte(`
tools:
  - name: run_corpus_ingest
    type: exec
    binary: agent
    env:
      - CORPUS_INGEST_COLLECTION={{ params.collection }}
    parameters:
      type: object
      properties:
        collection:
          type: string
`))
	require.NoError(t, err)
	require.Len(t, defs, 1)
	require.Equal(t, []string{"CORPUS_INGEST_COLLECTION={{ params.collection }}"}, defs[0].Env)
}

func TestParseToolDefsRejectsInvalidExecEnv(t *testing.T) {
	tests := []struct {
		name   string
		fields string
		want   string
	}{
		{name: "missing equals", fields: "    env: [\"NOTKEYVALUE\"]\n", want: "must be KEY=VALUE"},
		{name: "invalid key", fields: "    env: [\"bad-key=value\"]\n", want: "must match"},
		{name: "empty value", fields: "    env: [\"FOO=\"]\n", want: "value must not be empty"},
		{name: "unknown token", fields: "    env: [\"FOO={{params.collection}}\"]\n", want: "unknown env template token"},
		{name: "undeclared param", fields: "    env: [\"FOO={{ params.collection }}\"]\n    parameters:\n      type: object\n      properties:\n        other:\n          type: string\n", want: "requires parameter collection"},
		{name: "builtin env", fields: "    type: builtin\n    init: delay\n    env: [\"FOO=bar\"]\n", want: "cannot declare env"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			yaml := "tools:\n  - name: consume\n    binary: agent\n" + tc.fields
			if strings.Contains(tc.fields, "type: builtin") {
				yaml = "tools:\n  - name: consume\n" + tc.fields
			}
			_, err := ParseToolDefs([]byte(yaml))
			require.ErrorContains(t, err, tc.want)
		})
	}
}

func TestParseToolDefsRejectsEnvWithoutBinary(t *testing.T) {
	_, err := ParseToolDefs([]byte(`
tools:
  - name: consume
    type: boundary
    env:
      - FOO=bar
`))
	require.ErrorContains(t, err, "env requires binary")
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
