// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestDoltServerStopJoinsProcessBeforeDirectoryRemoval(t *testing.T) {
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
