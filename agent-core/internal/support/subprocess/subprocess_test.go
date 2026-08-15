// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package subprocess

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
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

func TestStartCancellationKillsAndReapsProcess(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	handle, err := Start(ctx, StartSpec{Binary: "sh", Args: []string{"-c", "sleep 3600"}})
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- handle.Wait() }()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("detached subprocess was not reaped after cancellation")
	}
	assert.Error(t, syscall.Kill(handle.PID(), 0))
}

func TestProductionProcGroupSetupIsOwnedBySubprocess(t *testing.T) {
	t.Parallel()

	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	internalRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	var owners []string
	require.NoError(t, filepath.WalkDir(internalRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr == nil && strings.Contains(string(data), "SysProcAttr") {
			owners = append(owners, path)
		}
		return readErr
	}))
	require.Equal(t, []string{filepath.Join(filepath.Dir(file), "subprocess.go")}, owners)
}

func TestEnvVarFormatting(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "KEY=value", EnvVar("KEY", "value"))
	assert.Equal(t, "COUNT=42", EnvVarInt("COUNT", 42))
}
