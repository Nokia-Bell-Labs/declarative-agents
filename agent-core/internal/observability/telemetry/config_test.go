// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package telemetry

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/require"
)

func TestRegisterFlagsDefaultsAndHelp(t *testing.T) {
	t.Parallel()
	var cfg Config
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.RegisterFlags(fs)

	for _, tc := range []struct {
		name  string
		def   string
		usage string
	}{
		{name: "otel-log-file", def: "", usage: "path to OTel trace output file"},
		{name: "otel-otlp-endpoint", def: "", usage: "OTLP gRPC endpoint for OTel spans (host:port); enables the OTLP exporter (srd008)"},
		{name: "otel-metric-otlp-endpoint", def: "", usage: "optional OTLP gRPC endpoint for OTel metrics; defaults to --otel-otlp-endpoint (srd008)"},
		{name: "otel-service-name", def: "agent", usage: "OTel resource service.name for this agent, so a cross-agent trace distinguishes agents"},
		{name: "otel-parent-span", def: "", usage: "W3C traceparent for parent span"},
		{name: "telemetry-capture", def: captureOff, usage: "telemetry content capture level: off, delta, or full"},
		{name: "verbose-trace", def: "false", usage: "record full LLM input/output in traces (alias for --telemetry-capture=full)"},
	} {
		flag := fs.Lookup(tc.name)
		require.NotNil(t, flag, tc.name)
		require.Equal(t, tc.def, flag.DefValue, tc.name)
		require.Equal(t, tc.usage, flag.Usage, tc.name)
	}
	require.Equal(t, captureOff, cfg.Capture)
	require.Equal(t, "agent", cfg.ServiceName)
}

func TestRegisterFlagsDoesNotTouchCommandLine(t *testing.T) {
	t.Parallel()
	require.Nil(t, pflag.CommandLine.Lookup("otel-log-file"))
	var cfg Config
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.RegisterFlags(fs)
	require.Nil(t, pflag.CommandLine.Lookup("otel-log-file"))
	require.NotNil(t, fs.Lookup("otel-log-file"))
}

func TestResolveCapture(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		args    []string
		want    string
		errText string
	}{
		{name: "default off", want: captureOff},
		{name: "explicit delta", args: []string{"--telemetry-capture=delta"}, want: captureDelta},
		{name: "explicit full", args: []string{"--telemetry-capture=full"}, want: captureFull},
		{name: "verbose alias", args: []string{"--verbose-trace"}, want: captureFull},
		{name: "alias with explicit full", args: []string{"--telemetry-capture=full", "--verbose-trace"}, want: captureFull},
		{name: "alias conflicts with off", args: []string{"--telemetry-capture=off", "--verbose-trace"}, errText: "conflicts"},
		{name: "alias conflicts with delta", args: []string{"--telemetry-capture=delta", "--verbose-trace"}, errText: "conflicts"},
		{name: "invalid", args: []string{"--telemetry-capture=everything"}, errText: "want off, delta, or full"},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cfg Config
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			cfg.RegisterFlags(fs)
			require.NoError(t, fs.Parse(tc.args))
			got, err := cfg.ResolveCapture(fs)
			if tc.errText != "" {
				require.ErrorContains(t, err, tc.errText)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
