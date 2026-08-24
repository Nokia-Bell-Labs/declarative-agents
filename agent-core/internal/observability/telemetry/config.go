// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package telemetry

import (
	"fmt"
	"strings"

	"github.com/spf13/pflag"
)

// Capture values match internal/tools/llm.CaptureLevel. This package cannot
// import that type: observability must not import tools (boundaries.yaml).
const (
	captureOff   = "off"
	captureDelta = "delta"
	captureFull  = "full"
)

// Config holds the agent binary's telemetry flags. RegisterFlags binds them
// onto a caller-supplied flag set; the package never touches a global flagset.
type Config struct {
	LogFile        string // --otel-log-file
	OTLPEndpoint   string // --otel-otlp-endpoint
	MetricEndpoint string // --otel-metric-otlp-endpoint
	ServiceName    string // --otel-service-name
	ParentSpan     string // --otel-parent-span
	Capture        string // --telemetry-capture
	VerboseTrace   bool   // --verbose-trace
}

// RegisterFlags defines the telemetry flags on fs. Callers must invoke this
// from cmd/agent; nothing registers at import time.
func (c *Config) RegisterFlags(fs *pflag.FlagSet) {
	fs.StringVar(&c.LogFile, "otel-log-file", "", "path to OTel trace output file")
	fs.StringVar(&c.OTLPEndpoint, "otel-otlp-endpoint", "", "OTLP gRPC endpoint for OTel spans (host:port); enables the OTLP exporter (srd008)")
	fs.StringVar(&c.MetricEndpoint, "otel-metric-otlp-endpoint", "", "optional OTLP gRPC endpoint for OTel metrics; defaults to --otel-otlp-endpoint (srd008)")
	fs.StringVar(&c.ServiceName, "otel-service-name", "agent", "OTel resource service.name for this agent, so a cross-agent trace distinguishes agents")
	fs.StringVar(&c.ParentSpan, "otel-parent-span", "", "W3C traceparent for parent span")
	fs.StringVar(&c.Capture, "telemetry-capture", captureOff, "telemetry content capture level: off, delta, or full")
	fs.BoolVar(&c.VerboseTrace, "verbose-trace", false, "record full LLM input/output in traces (alias for --telemetry-capture=full)")
}

// ResolveCapture validates Capture and applies --verbose-trace precedence.
// An explicit --telemetry-capture wins; otherwise --verbose-trace aliases full.
func (c *Config) ResolveCapture(fs *pflag.FlagSet) (string, error) {
	level, err := parseCaptureLevel(c.Capture)
	if err != nil {
		return "", fmt.Errorf("parse --telemetry-capture: %w", err)
	}
	if !c.VerboseTrace {
		return level, nil
	}
	if captureFlagChanged(fs) && level != captureFull {
		return "", fmt.Errorf(
			"--verbose-trace conflicts with --telemetry-capture=%s; use --telemetry-capture=full or omit one flag",
			level,
		)
	}
	return captureFull, nil
}

func captureFlagChanged(fs *pflag.FlagSet) bool {
	return fs != nil && fs.Changed("telemetry-capture")
}

func parseCaptureLevel(value string) (string, error) {
	level := strings.TrimSpace(value)
	switch level {
	case captureOff, captureDelta, captureFull:
		return level, nil
	default:
		return "", fmt.Errorf("invalid telemetry capture level %q (want off, delta, or full)", value)
	}
}
