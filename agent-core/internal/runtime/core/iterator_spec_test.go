// Copyright (c) 2026 Nokia. All rights reserved.

package core

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestMachineSpecForEachRoundTrips(t *testing.T) {
	t.Parallel()
	spec, err := ParseMachineSpec([]byte(iteratorMachineYAML))
	require.NoError(t, err)
	require.NotNil(t, spec.Transitions[1].ForEach)
	require.Equal(t, "$from(items).items", spec.Transitions[1].ForEach.Items)
	require.Equal(t, "items_joined", spec.Transitions[1].ForEach.Join.Label)

	data, err := yaml.Marshal(spec)
	require.NoError(t, err)
	roundTrip, err := ParseMachineSpec(data)
	require.NoError(t, err)
	require.Equal(t, spec.Transitions[1].ForEach, roundTrip.Transitions[1].ForEach)
}

func TestMachineSpecForEachValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		old     string
		repl    string
		wantErr string
	}{
		{
			name: "selector", old: "$from(items).items", repl: "$.items",
			wantErr: "is not a $from(label).path selector",
		},
		{
			name: "binding", old: "as: item", repl: "as: bad.item",
			wantErr: "is not a valid command-state label",
		},
		{
			name: "parallel bound", old: "mode: sequential", repl: "mode: parallel",
			wantErr: "max_concurrency: must be positive for parallel mode",
		},
		{
			name: "sequential bound", old: "mode: sequential", repl: "mode: sequential\n      max_concurrency: 2",
			wantErr: "max_concurrency: valid only for parallel mode",
		},
		{
			name: "invalid mode", old: "mode: sequential", repl: "mode: unbounded",
			wantErr: `mode: "unbounded" must be sequential or parallel`,
		},
		{
			name: "dynamic action", old: "action: item", repl: "action: $tool",
			wantErr: "requires a named action",
		},
		{
			name: "join state", old: "next: Joined\n        label: items_joined", repl: "next: Missing\n        label: items_joined",
			wantErr: `join.next: state "Missing" not in states list`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			input := strings.Replace(iteratorMachineYAML, test.old, test.repl, 1)
			_, err := ParseMachineSpec([]byte(input))
			require.ErrorContains(t, err, test.wantErr)
		})
	}
}

func TestMachineDiagnosticsTreatForEachSignalsAndJoinAsUsed(t *testing.T) {
	t.Parallel()
	spec, err := ParseMachineSpec([]byte(iteratorMachineYAML))
	require.NoError(t, err)
	for _, diagnostic := range DiagnoseMachineSpec(spec) {
		require.NotEqual(t, "unused_signal", diagnostic.Code, diagnostic.Message)
		require.NotEqual(t, "unreachable_state", diagnostic.Code, diagnostic.Message)
	}
}
