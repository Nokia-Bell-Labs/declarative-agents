// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// The lifecycle approval profile persists a checkpoint through the typed
// Checkpoint port only when --dolt-dsn names a running dolt sql-server
// (rel02.0-uc001, srd036-dolt-state-persistence). Suspend/resume therefore
// needs a live backend, unlike the model-free families. This file starts a
// throwaway dolt sql-server for the test process and tears it down on cleanup,
// so the suite stays self-contained where dolt is installed and skips cleanly
// where it is not.

// RequireDolt returns the dolt binary path or skips the test when dolt is not on
// PATH. Lifecycle persistence tests are gated on a local dolt install the same
// way the whole suite is gated on the sibling agent-core checkout being present.
func RequireDolt(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("dolt")
	if err != nil {
		t.Skipf("dolt not on PATH; skipping Dolt-backed lifecycle conformance: %v", err)
	}
	return path
}

// DoltServer is a throwaway dolt sql-server serving a single "agent" database.
type DoltServer struct {
	t       *testing.T
	dsn     string
	dataDir string // the "agent" database directory (a dolt repo)
	env     []string
	cancel  context.CancelFunc
	cmd     *exec.Cmd
	out     *bytes.Buffer
	done    chan struct{}
	waitErr error
	stop    sync.Once
	stopErr error
}

// StartDolt initializes an isolated dolt database and starts a sql-server bound
// to a free loopback port, returning a handle whose DSN feeds --dolt-dsn. The
// server and its config live entirely under an explicitly owned temporary
// directory (via DOLT_ROOT_PATH), so it neither reads nor mutates the
// developer's global dolt identity. One test cleanup stops and joins the server,
// then removes that directory after a bounded filesystem-quiescence check.
func StartDolt(t *testing.T) *DoltServer {
	t.Helper()
	RequireDolt(t)

	root, err := os.MkdirTemp("", "catalog-conformance-dolt-*")
	if err != nil {
		t.Fatalf("create dolt temp directory: %v", err)
	}
	s := &DoltServer{t: t, out: &bytes.Buffer{}}
	t.Cleanup(func() {
		if err := cleanupDolt(root, s.Stop, os.RemoveAll, time.Sleep); err != nil {
			t.Errorf("clean up dolt sql-server: %v\noutput:\n%s", err, s.out.String())
		}
	})

	// Isolate dolt's global config/identity under the temp tree so DOLT_COMMIT
	// has an author without touching the developer's ~/.dolt config.
	doltHome := filepath.Join(root, "dolthome")
	if err := os.MkdirAll(doltHome, 0o755); err != nil {
		t.Fatalf("create dolt home: %v", err)
	}
	env := append(os.Environ(), "DOLT_ROOT_PATH="+doltHome)
	runDolt(t, root, env, "config", "--global", "--add", "user.name", "conformance")
	runDolt(t, root, env, "config", "--global", "--add", "user.email", "conformance@example.com")

	dataDir := filepath.Join(root, "agent")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create dolt data dir: %v", err)
	}
	runDolt(t, dataDir, env, "init")

	addr := FreeAddr(t)
	port := PortOf(t, addr)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, "dolt", "sql-server", "--host", "127.0.0.1", "--port", port)
	cmd.Dir = root
	cmd.Env = env
	cmd.Stdout = s.out
	cmd.Stderr = s.out
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start dolt sql-server: %v", err)
	}
	s.dataDir = dataDir
	s.env = env
	s.cancel = cancel
	s.cmd = cmd
	s.done = make(chan struct{})
	go func() {
		s.waitErr = cmd.Wait()
		close(s.done)
	}()

	s.waitListen(addr, 30*time.Second)
	s.dsn = fmt.Sprintf("root@tcp(127.0.0.1:%s)/agent", port)
	return s
}

// DSN is the MySQL-wire DSN passed to the agent via --dolt-dsn.
func (s *DoltServer) DSN() string { return s.dsn }

// LatestRunBranch returns the most recent run-* branch, which is the branch a
// suspended (non-terminal) run persists and does not merge away. A terminal run
// merges its branch to main and deletes it, so a suspended run leaves exactly
// one such branch for the resume invocation to target.
func (s *DoltServer) LatestRunBranch(t *testing.T) string {
	t.Helper()
	// Ask dolt for JSON and decode it, rather than scanning CSV lines for a
	// run- prefix. The prefix scan did double duty as header-skip and value
	// filter -- it worked only because the header column is literally 'name'
	// and silently returned an empty branch name on any format change, which
	// then flowed on as a branch (GH-1391).
	out := runDolt(t, s.dataDir, s.env,
		"sql", "-q",
		"SELECT name FROM dolt_branches WHERE name LIKE 'run-%' ORDER BY name DESC LIMIT 1",
		"-r", "json")
	var result struct {
		Rows []struct {
			Name string `json:"name"`
		} `json:"rows"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("parse dolt sql json output: %v\noutput:\n%s", err, out)
	}
	if len(result.Rows) == 0 || result.Rows[0].Name == "" {
		t.Fatalf("no run-* branch found in dolt_branches; output:\n%s", out)
	}
	return result.Rows[0].Name
}

// Stop cancels and joins the server process before its temporary directory is
// removed. If context cancellation does not terminate Dolt promptly, Stop
// explicitly kills and joins it so background filesystem activity cannot race
// the owned-directory cleanup.
func (s *DoltServer) Stop() error {
	s.stop.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		if s.cmd == nil {
			return
		}
		select {
		case <-s.done:
		case <-time.After(3 * time.Second):
			if s.cmd == nil || s.cmd.Process == nil {
				s.stopErr = fmt.Errorf("timed out waiting for server shutdown")
				return
			}
			if err := s.cmd.Process.Kill(); err != nil && !strings.Contains(err.Error(), "process already finished") {
				s.stopErr = fmt.Errorf("kill after shutdown timeout: %w", err)
				return
			}
			<-s.done
		}
		if s.waitErr != nil && !isExpectedDoltShutdown(s.waitErr) {
			s.stopErr = fmt.Errorf("server exited unexpectedly: %w", s.waitErr)
		}
	})
	return s.stopErr
}

const (
	doltCleanupAttempts    = 20
	doltCleanupQuietPeriod = 50 * time.Millisecond
)

// cleanupDolt owns the complete teardown order. It always stops and joins the
// server before attempting directory removal, and reports errors from both
// phases instead of allowing one to conceal the other.
func cleanupDolt(
	root string,
	stop func() error,
	removeAll func(string) error,
	wait func(time.Duration),
) error {
	stopErr := stop()
	removeErr := removeDoltTempDir(root, removeAll, wait)
	if stopErr != nil {
		stopErr = fmt.Errorf("stop and join dolt sql-server: %w", stopErr)
	}
	return errors.Join(stopErr, removeErr)
}

// removeDoltTempDir tolerates only the narrow race seen when a just-joined Dolt
// process has late filesystem activity: RemoveAll may report ENOTEMPTY, or may
// succeed immediately before an entry is recreated. Each successful removal
// must remain absent for one quiet period. Other errors (notably permissions)
// fail immediately, and a persistent race reports both the owned root and the
// failing entry returned by the filesystem.
func removeDoltTempDir(
	root string,
	removeAll func(string) error,
	wait func(time.Duration),
) error {
	var lastErr error
	for attempt := 1; attempt <= doltCleanupAttempts; attempt++ {
		removeErr := removeAll(root)
		switch {
		case removeErr == nil, errors.Is(removeErr, fs.ErrNotExist):
			lastErr = nil
		case errors.Is(removeErr, syscall.ENOTEMPTY):
			lastErr = removeErr
		default:
			return fmt.Errorf("remove dolt temp directory %q: %w", root, removeErr)
		}

		wait(doltCleanupQuietPeriod)
		_, statErr := os.Lstat(root)
		switch {
		case errors.Is(statErr, fs.ErrNotExist):
			return nil
		case statErr != nil:
			return fmt.Errorf("verify removal of dolt temp directory %q: %w", root, statErr)
		case lastErr == nil:
			lastErr = fmt.Errorf("path %q was recreated after removal", root)
		}
	}
	return fmt.Errorf(
		"remove dolt temp directory %q after %d attempts: %w",
		root, doltCleanupAttempts, lastErr,
	)
}

func isExpectedDoltShutdown(err error) bool {
	exitErr, ok := err.(*exec.ExitError)
	return ok && (exitErr.ExitCode() == -1 || exitErr.ExitCode() == 137)
}

// waitListen blocks until the server accepts a TCP connection or the timeout
// elapses, absorbing the start-up race the OS-assigned port introduces.
func (s *DoltServer) waitListen(addr string, timeout time.Duration) {
	s.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-s.done:
			s.t.Fatalf("dolt sql-server exited before listening:\n%s", s.out.String())
		default:
		}
		conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	s.t.Fatalf("dolt sql-server not listening on %s within %s:\n%s", addr, timeout, s.out.String())
}

// runDolt runs a one-shot dolt subcommand in dir and returns its combined
// output, failing the test on error.
func runDolt(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("dolt", args...)
	cmd.Dir = dir
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("dolt %s: %v\noutput:\n%s", strings.Join(args, " "), err, out.String())
	}
	return out.String()
}
