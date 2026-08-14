// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	toollm "github.com/Nokia-Bell-Labs/declarative-agents/agent-core/internal/tools/llm"
)

func TestRootCommandExposesTelemetryCaptureFlags(t *testing.T) {
	capture := rootCmd.PersistentFlags().Lookup("telemetry-capture")
	require.NotNil(t, capture)
	require.Equal(t, string(toollm.CaptureOff), capture.DefValue)
	require.NotNil(t, rootCmd.PersistentFlags().Lookup("verbose-trace"))

	usage := rootCmd.UsageString()
	require.Contains(t, usage, "--telemetry-capture")
	require.Contains(t, usage, "--verbose-trace")
}

func TestResolveTelemetryCapture(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		value         string
		explicitlySet bool
		verboseTrace  bool
		want          toollm.CaptureLevel
		errContains   string
	}{
		{name: "default off", value: "off", want: toollm.CaptureOff},
		{name: "explicit delta", value: "delta", explicitlySet: true, want: toollm.CaptureDelta},
		{name: "explicit full", value: "full", explicitlySet: true, want: toollm.CaptureFull},
		{name: "verbose alias", value: "off", verboseTrace: true, want: toollm.CaptureFull},
		{name: "alias with explicit full", value: "full", explicitlySet: true, verboseTrace: true, want: toollm.CaptureFull},
		{name: "alias conflicts with off", value: "off", explicitlySet: true, verboseTrace: true, errContains: "conflicts"},
		{name: "alias conflicts with delta", value: "delta", explicitlySet: true, verboseTrace: true, errContains: "conflicts"},
		{name: "invalid", value: "everything", explicitlySet: true, errContains: "want off, delta, or full"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveTelemetryCapture(tc.value, tc.explicitlySet, tc.verboseTrace)
			if tc.errContains != "" {
				require.ErrorContains(t, err, tc.errContains)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestLoadRuntimeConfigRejectsInvalidTelemetryCapture(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })
	clearAgentFlags()
	flagProfile = profilePathFromTest(t, "monitor/profile.yaml")
	flagTelemetryCapture = "everything"
	rootCmd.PersistentFlags().Lookup("telemetry-capture").Changed = true

	_, err := loadRuntimeConfig()

	require.ErrorContains(t, err, "parse --telemetry-capture")
	require.ErrorContains(t, err, "want off, delta, or full")
}
