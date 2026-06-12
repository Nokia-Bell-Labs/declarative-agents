// Copyright (c) 2026 Nokia. All rights reserved.

package stl

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcGroupCmd_SetsFields(t *testing.T) {
	cmd := exec.Command("echo", "hello")
	ProcGroupCmd(cmd)
	assert.True(t, cmd.SysProcAttr.Setpgid)
	assert.NotNil(t, cmd.Cancel)
	assert.Equal(t, defaultWaitDelay, cmd.WaitDelay)
}

func TestRunProcGroup_Success(t *testing.T) {
	r := RunProcGroup(context.Background(), 5*time.Second, "", "echo", "hello")
	require.NoError(t, r.Err)
	assert.True(t, r.Success())
	assert.Equal(t, 0, r.ExitCode)
	assert.Contains(t, r.Stdout, "hello")
	assert.True(t, r.Duration > 0)
}

func TestRunProcGroup_NonZeroExit(t *testing.T) {
	r := RunProcGroup(context.Background(), 5*time.Second, "", "sh", "-c", "echo err >&2; exit 2")
	assert.False(t, r.Success())
	assert.Equal(t, 2, r.ExitCode)
	assert.Contains(t, r.Stderr, "err")
	assert.NoError(t, r.Err)
}

func TestRunProcGroup_Timeout(t *testing.T) {
	r := RunProcGroup(context.Background(), 200*time.Millisecond, "", "sleep", "30")
	assert.False(t, r.Success())
	assert.NotEqual(t, 0, r.ExitCode)
}

func TestRunProcGroup_BinaryNotFound(t *testing.T) {
	r := RunProcGroup(context.Background(), 5*time.Second, "", "nonexistent-binary-xyz")
	assert.False(t, r.Success())
	assert.Equal(t, -1, r.ExitCode)
	assert.Error(t, r.Err)
}

func TestRunProcGroup_WorkingDir(t *testing.T) {
	dir := t.TempDir()
	r := RunProcGroup(context.Background(), 5*time.Second, dir, "pwd")
	require.NoError(t, r.Err)
	assert.Contains(t, r.Stdout, dir)
}

func TestRunResult_Success(t *testing.T) {
	assert.True(t, (&RunResult{ExitCode: 0}).Success())
	assert.False(t, (&RunResult{ExitCode: 1}).Success())
	assert.False(t, (&RunResult{Err: assert.AnError}).Success())
}
