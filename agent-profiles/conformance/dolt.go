// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
// way the whole suite is gated on AGENT_CORE_ROOT.
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
	out     *bytes.Buffer
	done    chan struct{}
}

// StartDolt initializes an isolated dolt database and starts a sql-server bound
// to a free loopback port, returning a handle whose DSN feeds --dolt-dsn. The
// server and its config live entirely under t.TempDir() (via DOLT_ROOT_PATH),
// so it neither reads nor mutates the developer's global dolt identity, and it
// is killed on test cleanup.
func StartDolt(t *testing.T) *DoltServer {
	t.Helper()
	RequireDolt(t)

	root := t.TempDir()
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
	out := &bytes.Buffer{}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatalf("start dolt sql-server: %v", err)
	}
	s := &DoltServer{t: t, dataDir: dataDir, env: env, cancel: cancel, out: out, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(s.done)
	}()
	t.Cleanup(s.Stop)

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
	out := runDolt(t, s.dataDir, s.env,
		"sql", "-q",
		"SELECT name FROM dolt_branches WHERE name LIKE 'run-%' ORDER BY name DESC LIMIT 1",
		"-r", "csv")
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "run-") {
			return line
		}
	}
	return ""
}

// Stop cancels the server process and waits briefly for it to exit.
func (s *DoltServer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	select {
	case <-s.done:
	case <-time.After(3 * time.Second):
	}
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
