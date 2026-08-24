// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

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
	require.NotNil(t, rootCmd.PersistentFlags().Lookup("otel-log-file"))
	require.NotNil(t, rootCmd.PersistentFlags().Lookup("otel-otlp-endpoint"))
	require.NotNil(t, rootCmd.PersistentFlags().Lookup("otel-metric-otlp-endpoint"))
	require.NotNil(t, rootCmd.PersistentFlags().Lookup("otel-service-name"))
	require.NotNil(t, rootCmd.PersistentFlags().Lookup("otel-parent-span"))

	usage := rootCmd.UsageString()
	require.Contains(t, usage, "--telemetry-capture")
	require.Contains(t, usage, "--verbose-trace")
}

func TestLoadRuntimeConfigRejectsInvalidTelemetryCapture(t *testing.T) {
	restore := snapshotAgentFlags()
	t.Cleanup(func() { restoreAgentFlags(restore) })
	clearAgentFlags()
	flagProfile = profilePathFromTest(t, "monitor/profile.yaml")
	telemetryCfg.Capture = "everything"
	rootCmd.PersistentFlags().Lookup("telemetry-capture").Changed = true

	_, err := loadRuntimeConfig()

	require.ErrorContains(t, err, "parse --telemetry-capture")
	require.ErrorContains(t, err, "want off, delta, or full")
}
