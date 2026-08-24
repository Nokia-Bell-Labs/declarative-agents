// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRootCommandExposesEveryRuntimeFlag(t *testing.T) {
	t.Parallel()
	flags := []string{
		"profile",
		"core-root",
		"directory",
		"request",
		"output",
		"child-agent-binary",
		"validate-config",
		"otel-log-file",
		"otel-otlp-endpoint",
		"otel-metric-otlp-endpoint",
		"otel-service-name",
		"otel-parent-span",
		"telemetry-capture",
		"verbose-trace",
		"dolt-dsn",
		"resume-checkpoint",
		"resume-signal",
	}
	require.Len(t, flags, 17)
	for _, name := range flags {
		require.NotNil(t, rootCmd.PersistentFlags().Lookup(name), name)
	}
}
