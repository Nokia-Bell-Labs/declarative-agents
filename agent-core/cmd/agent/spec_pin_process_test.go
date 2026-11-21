// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// specPinMainArgsEnv carries the argument list a re-executed test binary
// hands to main(). TestMain reads it before any test runs, so the child is the
// agent process itself. These tests pin process behaviour that only main()
// has: the exit-code mapping and the signal disposition before the run binds.
const specPinMainArgsEnv = "AGENT_CORE_SPEC_PIN_MAIN_ARGS"

func specPinMainCommand(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^$")
	command.Env = append(os.Environ(), specPinMainArgsEnv+"="+strings.Join(args, "\x1f"))
	return command
}

// TestSpecPinRunErrorExitsOne pins R-RUN-008's run-error code: a run the
// binary cannot complete (here, an unreadable profile) exits with code 1,
// distinct from a failed terminal (2) and success (0).
func TestSpecPinRunErrorExitsOne(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent", "profile.yaml")
	err := specPinMainCommand(t, "--profile", missing).Run()

	var exit *exec.ExitError
	require.True(t, errors.As(err, &exit), "want an exit error, got %v", err)
	require.Equal(t, ExitRunError, exit.ExitCode())
}

// TestSpecPinPreRunSignalKeepsOSDefault pins R-RUN-010's pre-run half: until
// the run is bound, the process installs no SIGTERM handler, so a SIGTERM
// sent while startup is blocked (here, reading a profile from a FIFO no one
// writes) ends the process by the signal rather than being swallowed.
func TestSpecPinPreRunSignalKeepsOSDefault(t *testing.T) {
	fifo := filepath.Join(t.TempDir(), "profile.yaml")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))

	command := specPinMainCommand(t, "--profile", fifo)
	require.NoError(t, command.Start())
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()

	// Give the child time to reach the blocking read; a signal that lands
	// earlier still exercises the pre-run disposition.
	time.Sleep(500 * time.Millisecond)
	require.NoError(t, command.Process.Signal(syscall.SIGTERM))

	select {
	case err := <-done:
		var exit *exec.ExitError
		require.True(t, errors.As(err, &exit), "want the process to end by signal, got %v", err)
		status, ok := exit.Sys().(syscall.WaitStatus)
		require.True(t, ok)
		require.True(t, status.Signaled(), "process exited with code %d instead of ending by SIGTERM", status.ExitStatus())
		require.Equal(t, syscall.SIGTERM, status.Signal())
	case <-time.After(10 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("SIGTERM before the run was bound did not end the process")
	}
}
