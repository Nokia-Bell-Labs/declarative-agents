// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package execute

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildArgs_ProfileOnly(t *testing.T) {
	cfg := Config{
		Profile: "agents/executor/profile.yaml", CoreRoot: "/core",
		Directory: "/workspace", Request: "request.yaml", Output: "out",
		OTelLogFile: "trace.json", OTelServiceName: "executor", OTLPEndpoint: "localhost:4317",
	}
	require.Equal(t, []string{
		"--profile", "agents/executor/profile.yaml",
		"--core-root", "/core",
		"--directory", "/workspace",
		"--request", "request.yaml",
		"--output", "out",
		"--otel-log-file", "trace.json",
		"--otel-service-name", "executor",
		"--otel-otlp-endpoint", "localhost:4317",
	}, cfg.BuildArgs())
}

func TestRunAgentMapsSuccessAndFailure(t *testing.T) {
	success := RunAgent(context.Background(), Config{Binary: "printf", Timeout: time.Second}, "done")
	require.True(t, success.Success())
	assert.True(t, success.Started)
	assert.Equal(t, "done", success.Stdout)

	failure := RunAgent(context.Background(), Config{Binary: "false", Timeout: time.Second})
	require.False(t, failure.Success())
	assert.True(t, failure.Started)
	assert.NotEqual(t, 0, failure.ExitCode)
}

func TestRunAgentReportsMissingBinary(t *testing.T) {
	result := RunAgent(context.Background(), Config{Binary: "/nonexistent/agent", Timeout: time.Second})
	require.False(t, result.Success())
	assert.False(t, result.Started)
	require.Error(t, result.Err)
}
