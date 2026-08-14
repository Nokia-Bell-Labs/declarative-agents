// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var doltBin = flag.String("dolt-bin", "", "path or command name for the Dolt integration-test executable")

func TestDoltBinaryPrefersExplicitFlag(t *testing.T) {
	previous := *doltBin
	t.Cleanup(func() { *doltBin = previous })
	*doltBin = " /declared/bin/dolt "

	require.Equal(t, "/declared/bin/dolt", doltBinary())
}

// doltBinary resolves the dolt executable: an explicit test flag wins,
// otherwise the binary is looked up on PATH. An empty result means no dolt is
// available and the gated tests skip.
func doltBinary() string {
	if configured := strings.TrimSpace(*doltBin); configured != "" {
		return configured
	}
	if path, err := exec.LookPath("dolt"); err == nil {
		return path
	}
	return ""
}

// startDoltServer launches a `dolt sql-server` from a prebuilt dolt binary on an
// ephemeral port against a throwaway data directory, waits until it accepts
// connections, and returns a database-less DSN base ("root@tcp(127.0.0.1:PORT)/").
// The server is torn down when the test ends. It skips when no dolt binary is
// installed so `go test ./...` stays green on machines without dolt, while the
// two Dolt Mage targets run it for real.
func startDoltServer(t *testing.T) string {
	t.Helper()
	bin := doltBinary()
	if bin == "" {
		t.Skip("no dolt binary found (install dolt or pass -dolt-bin); skipping Dolt integration test")
	}

	port := freeTCPPort(t)
	rootDir := t.TempDir()
	dataDir := filepath.Join(rootDir, "data")
	homeDir := filepath.Join(rootDir, "home")
	doltRootDir := filepath.Join(rootDir, "dolt-root")
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	require.NoError(t, os.MkdirAll(homeDir, 0o700))
	require.NoError(t, os.MkdirAll(doltRootDir, 0o700))
	env := doltTestEnvironment(homeDir, doltRootDir)
	configureDoltTestIdentity(t, bin, dataDir, env)

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, bin, "sql-server",
		"--host=127.0.0.1",
		fmt.Sprintf("--port=%d", port),
		"--data-dir="+dataDir,
	)
	// DOLT_ROOT_HOST=% lets root connect over TCP from 127.0.0.1; the server
	// otherwise only provisions root@localhost. Identity and global config are
	// rooted under this test's temporary directory, never the developer's home.
	cmd.Env = env
	var log bytes.Buffer
	cmd.Stdout = &log
	cmd.Stderr = &log
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	base := fmt.Sprintf("root@tcp(127.0.0.1:%d)/", port)
	waitForDolt(t, base, &log)
	return base
}

func doltTestEnvironment(homeDir, doltRootDir string) []string {
	replacements := map[string]string{
		"HOME":               homeDir,
		"DOLT_ROOT_PATH":     doltRootDir,
		"DOLT_ROOT_HOST":     "%",
		"DOLT_ROOT_PASSWORD": "",
	}
	env := make([]string, 0, len(os.Environ())+len(replacements))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, replaced := replacements[key]; !replaced {
			env = append(env, entry)
		}
	}
	for key, value := range replacements {
		env = append(env, key+"="+value)
	}
	return env
}

func configureDoltTestIdentity(
	t *testing.T,
	bin string,
	dataDir string,
	env []string,
) {
	t.Helper()
	settings := []struct {
		name  string
		value string
	}{
		{name: "user.name", value: "Agent Core Integration"},
		{name: "user.email", value: "agent-core-integration@example.invalid"},
		{name: "metrics.disabled", value: "true"},
	}
	for _, setting := range settings {
		cmd := exec.Command(bin, "config", "--global", "--add", setting.name, setting.value)
		cmd.Dir = dataDir
		cmd.Env = env
		output, err := cmd.CombinedOutput()
		require.NoErrorf(
			t,
			err,
			"configure isolated Dolt %s:\n%s",
			setting.name,
			output,
		)
	}
}

// freeTCPPort reserves an ephemeral port and releases it for the server to claim.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	require.NoError(t, listener.Close())
	return port
}

// waitForDolt polls the freshly started server until it answers a ping or the
// deadline passes, surfacing the captured server log on timeout.
func waitForDolt(t *testing.T, base string, log *bytes.Buffer) {
	t.Helper()
	db, err := sql.Open("dolt", base)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	deadline := time.Now().Add(60 * time.Second)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err := db.PingContext(ctx)
		cancel()
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("dolt sql-server did not become ready within 60s: %v\nserver log:\n%s", err, log.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// requireDoltTestDB creates the shared test database on a database-less DSN base
// so subsequent adapters can select it.
func requireDoltTestDB(t *testing.T, base string) {
	t.Helper()
	db, err := sql.Open("dolt", base)
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = db.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS "+doltTestDB)
	require.NoError(t, err)
}
