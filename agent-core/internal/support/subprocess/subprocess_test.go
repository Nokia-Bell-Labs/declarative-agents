// Copyright (c) 2026 Nokia. All rights reserved.

package subprocess

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunSeparatesStdoutAndStderr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		script     string
		wantStdout string
		wantStderr string
		wantExit   int
	}{
		{name: "stdout only", script: `printf stdout`, wantStdout: "stdout"},
		{name: "stderr only", script: `printf stderr >&2`, wantStderr: "stderr"},
		{
			name: "interleaved nonzero", script: `printf out1; printf err1 >&2; printf out2; printf err2 >&2; exit 7`,
			wantStdout: "out1out2", wantStderr: "err1err2", wantExit: 7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := Run(context.Background(), Spec{
				Binary: "sh", Args: []string{"-c", tt.script}, Timeout: time.Second,
			})

			assert.Equal(t, tt.wantStdout, result.Stdout)
			assert.Equal(t, tt.wantStderr, result.Stderr)
			assert.Equal(t, tt.wantExit, result.ExitCode)
			assert.NoError(t, result.Err)
			assert.Equal(t, tt.wantExit == 0, result.Success())
			assert.Positive(t, result.Duration)
		})
	}
}

func TestRunTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		context   func() (context.Context, context.CancelFunc)
		timeout   time.Duration
		timedOut  bool
		wantError error
	}{
		{
			name: "timeout",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			timeout:  20 * time.Millisecond,
			timedOut: true,
		},
		{
			name: "active cancellation",
			context: func() (context.Context, context.CancelFunc) {
				return context.WithCancel(context.Background())
			},
			timeout:   time.Second,
			wantError: context.Canceled,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := tt.context()
			defer cancel()
			if tt.wantError != nil {
				go func() {
					time.Sleep(20 * time.Millisecond)
					cancel()
				}()
			}
			result := Run(ctx, Spec{Binary: "sh", Args: []string{"-c", "sleep 2"}, Timeout: tt.timeout})

			assert.Equal(t, tt.timedOut, result.TimedOut)
			assert.Equal(t, -1, result.ExitCode)
			if tt.wantError != nil {
				assert.ErrorIs(t, result.Err, tt.wantError)
			} else {
				assert.NoError(t, result.Err)
			}
			assert.False(t, result.Success())
		})
	}
}

func TestRunCapturesLargeOutputWithoutImplicitCap(t *testing.T) {
	t.Parallel()
	result := Run(context.Background(), Spec{
		Binary: "sh", Args: []string{"-c", `printf '%05000d' 0; printf '%05000d' 0 >&2`},
		Timeout: time.Second,
	})

	require.True(t, result.Success())
	assert.Len(t, result.Stdout, 5000)
	assert.Len(t, result.Stderr, 5000)
}

func TestRunSpawnFailure(t *testing.T) {
	t.Parallel()
	result := Run(context.Background(), Spec{Binary: "/definitely/missing/subprocess", Timeout: time.Second})

	assert.Equal(t, -1, result.ExitCode)
	assert.Error(t, result.Err)
	assert.False(t, result.TimedOut)
	assert.False(t, result.Success())
}

func TestRunCLIOutputUsesStderrForFailure(t *testing.T) {
	t.Parallel()
	output, err := RunCLIOutput(context.Background(), "", "sh", "-c", `printf stdout; printf diagnostic >&2; exit 2`)
	assert.Empty(t, output)
	require.Error(t, err)
	assert.Equal(t, "diagnostic", err.Error())

	output, err = RunCLIOutput(context.Background(), "", "sh", "-c", `printf success`)
	require.NoError(t, err)
	assert.Equal(t, "success", output)
}

func TestEnvVarFormatting(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "KEY=value", EnvVar("KEY", "value"))
	assert.Equal(t, "COUNT=42", EnvVarInt("COUNT", 42))
}
