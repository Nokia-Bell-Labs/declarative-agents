// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func buildFreshAgentFor(name string) (string, error) {
	fmt.Printf("building fresh agent binary for %s...\n", name)
	if err := Build(); err != nil {
		return "", fmt.Errorf("build agent: %w", err)
	}
	return filepath.Abs(filepath.Join(binDir, "agent"))
}

func renderIntegrationFixture(rootDir, rel string, values map[string]string) (string, error) {
	path := filepath.Join(rootDir, "magefiles", "fixtures", rel)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(data)
	for key, value := range values {
		content = strings.ReplaceAll(content, "{{"+key+"}}", value)
	}
	return content, nil
}

func freeLoopbackAddress() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()
	return listener.Addr().String(), nil
}

func waitHTTPStatus(url string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == want {
				return nil
			}
			lastErr = fmt.Errorf("%s returned status %d", url, resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return lastErr
}

func postJSONStatus(url, body string, want int) error {
	resp, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != want {
		return fmt.Errorf("%s returned status %d, want %d", url, resp.StatusCode, want)
	}
	return nil
}

func runCommandCapture(cmd *exec.Cmd) (bytes.Buffer, error) {
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	return out, cmd.Run()
}

func stopProcess(cmd *exec.Cmd, cancel context.CancelFunc) {
	cancel()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}
