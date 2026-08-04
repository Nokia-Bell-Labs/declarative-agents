// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestCleanupDoltStopsBeforeRemoval(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	var events []string

	err := cleanupDolt(
		root,
		func() error {
			events = append(events, "stop")
			return nil
		},
		func(path string) error {
			events = append(events, "remove "+path)
			return os.RemoveAll(path)
		},
		func(time.Duration) {
			events = append(events, "wait")
		},
	)
	if err != nil {
		t.Fatalf("cleanup dolt: %v", err)
	}
	want := strings.Join([]string{"stop", "remove " + root, "wait"}, "\n")
	if got := strings.Join(events, "\n"); got != want {
		t.Fatalf("cleanup order:\n%s\nwant:\n%s", got, want)
	}
}

func TestCleanupDoltReportsStopAndRemovalErrors(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stopErr := errors.New("server process remains live")
	removeErr := &os.PathError{
		Op:   "remove",
		Path: filepath.Join(root, "protected"),
		Err:  syscall.EACCES,
	}

	err := cleanupDolt(
		root,
		func() error { return stopErr },
		func(string) error { return removeErr },
		func(time.Duration) {},
	)
	for _, want := range []error{stopErr, removeErr} {
		if !errors.Is(err, want) {
			t.Errorf("cleanup error %v does not include %v", err, want)
		}
	}
}

func TestRemoveDoltTempDirRetriesTransientNotEmpty(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	blocker := filepath.Join(root, "late-entry")
	removeCalls := 0
	waitCalls := 0

	err := removeDoltTempDir(
		root,
		func(path string) error {
			removeCalls++
			if removeCalls == 1 {
				return &os.PathError{Op: "remove", Path: blocker, Err: syscall.ENOTEMPTY}
			}
			return os.RemoveAll(path)
		},
		func(time.Duration) { waitCalls++ },
	)
	if err != nil {
		t.Fatalf("remove dolt temp directory: %v", err)
	}
	if removeCalls != 2 {
		t.Fatalf("remove calls = %d, want 2", removeCalls)
	}
	if waitCalls != 2 {
		t.Fatalf("wait calls = %d, want 2", waitCalls)
	}
}

func TestRemoveDoltTempDirRetriesRecreatedDirectory(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	lateEntry := filepath.Join(root, "late-entry")
	removeCalls := 0

	err := removeDoltTempDir(
		root,
		func(path string) error {
			removeCalls++
			if err := os.RemoveAll(path); err != nil {
				return err
			}
			if removeCalls == 1 {
				if err := os.MkdirAll(path, 0o755); err != nil {
					return err
				}
				return os.WriteFile(lateEntry, []byte("late"), 0o600)
			}
			return nil
		},
		func(time.Duration) {},
	)
	if err != nil {
		t.Fatalf("remove recreated dolt temp directory: %v", err)
	}
	if removeCalls != 2 {
		t.Fatalf("remove calls = %d, want 2", removeCalls)
	}
}

func TestRemoveDoltTempDirReportsPersistentFailurePaths(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	blocker := filepath.Join(root, "late-entry")
	removeCalls := 0

	err := removeDoltTempDir(
		root,
		func(string) error {
			removeCalls++
			return &os.PathError{Op: "remove", Path: blocker, Err: syscall.ENOTEMPTY}
		},
		func(time.Duration) {},
	)
	if err == nil {
		t.Fatal("remove dolt temp directory succeeded, want persistent failure")
	}
	if removeCalls != doltCleanupAttempts {
		t.Fatalf("remove calls = %d, want %d", removeCalls, doltCleanupAttempts)
	}
	for _, path := range []string{root, blocker} {
		if !strings.Contains(err.Error(), path) {
			t.Errorf("error %q does not report path %q", err, path)
		}
	}
}

func TestRemoveDoltTempDirDoesNotRetryPermissionError(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	blocker := filepath.Join(root, "protected")
	removeCalls := 0
	waitCalls := 0

	err := removeDoltTempDir(
		root,
		func(string) error {
			removeCalls++
			return &os.PathError{Op: "remove", Path: blocker, Err: syscall.EACCES}
		},
		func(time.Duration) { waitCalls++ },
	)
	if !errors.Is(err, syscall.EACCES) {
		t.Fatalf("remove error = %v, want permission error", err)
	}
	if removeCalls != 1 || waitCalls != 0 {
		t.Fatalf("remove calls = %d, wait calls = %d; want 1, 0", removeCalls, waitCalls)
	}
}

func TestDoltServerStopJoinsProcessBeforeDirectoryRemoval(t *testing.T) {
	t.Parallel()
	if os.Getenv("DOLT_STOP_HELPER") == "1" {
		for {
			time.Sleep(time.Second)
		}
	}

	for range 10 {
		root, err := os.MkdirTemp("", "dolt-stop-*")
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDoltServerStopJoinsProcessBeforeDirectoryRemoval$")
		cmd.Env = append(os.Environ(), "DOLT_STOP_HELPER=1")
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}

		server := &DoltServer{
			cancel: cancel,
			cmd:    cmd,
			out:    &out,
			done:   make(chan struct{}),
		}
		go func() {
			server.waitErr = cmd.Wait()
			close(server.done)
		}()

		if err := server.Stop(); err != nil {
			t.Fatalf("stop helper process: %v", err)
		}
		if err := os.RemoveAll(filepath.Clean(root)); err != nil {
			t.Fatalf("remove released directory: %v", err)
		}
		_, err = os.Stat(root)
		if !os.IsNotExist(err) {
			t.Fatalf("released directory still exists: %v", err)
		}
	}
}
