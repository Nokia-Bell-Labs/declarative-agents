// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartDoltDisablesDetachedMetrics(t *testing.T) {
	server := StartDolt(t)
	got := strings.TrimSpace(runDolt(t, server.dataDir, server.env,
		"config", "--global", "--get", "metrics.disabled"))
	if got != "true" {
		t.Fatalf("metrics.disabled = %q, want true", got)
	}
}

func TestDoltServerStopJoinsProcessBeforeDirectoryRemoval(t *testing.T) {
	if os.Getenv("DOLT_STOP_HELPER") == "1" {
		root := os.Getenv("DOLT_STOP_ROOT")
		events := filepath.Join(root, ".dolt")
		if err := os.MkdirAll(events, 0o755); err != nil {
			panic(err)
		}
		ready := os.NewFile(3, "ready")
		if _, err := ready.Write([]byte{1}); err != nil {
			panic(err)
		}
		_ = ready.Close()
		for i := 0; ; i++ {
			path := filepath.Join(events, fmt.Sprintf("event-%d", i%4))
			if err := os.WriteFile(path, []byte("metric"), 0o644); err != nil {
				panic(err)
			}
			if err := os.Remove(path); err != nil {
				panic(err)
			}
		}
	}

	for range 10 {
		root, err := os.MkdirTemp("", "dolt-stop-*")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(root) })
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDoltServerStopJoinsProcessBeforeDirectoryRemoval$")
		cmd.Env = append(os.Environ(), "DOLT_STOP_HELPER=1", "DOLT_STOP_ROOT="+root)
		readyReader, readyWriter, err := os.Pipe()
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_ = readyReader.Close()
			_ = readyWriter.Close()
		})
		cmd.ExtraFiles = []*os.File{readyWriter}
		var out bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &out
		if err := cmd.Start(); err != nil {
			cancel()
			t.Fatal(err)
		}
		_ = readyWriter.Close()

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
		t.Cleanup(func() { _ = server.Stop() })

		if err := readyReader.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatal(err)
		}
		var ready [1]byte
		if _, err := readyReader.Read(ready[:]); err != nil {
			t.Fatalf("wait for helper writer: %v\noutput:\n%s", err, out.String())
		}
		_ = readyReader.Close()

		if err := server.Stop(); err != nil {
			t.Fatalf("stop helper process: %v", err)
		}
		if err := server.Stop(); err != nil {
			t.Fatalf("stop helper process again: %v", err)
		}
		if cmd.ProcessState == nil {
			t.Fatal("Stop returned before helper process was reaped")
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
