// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const integrationHTTPRequestTimeout = 2 * time.Second

var integrationHTTPClient = &http.Client{Timeout: integrationHTTPRequestTimeout}

func buildIntegrationAgent(coreRoot string) (string, error) {
	binary := filepath.Join(os.TempDir(), "agent-profiles-integration-agent")
	cmd := exec.Command("go", "build", "-tags", "production", "-o", binary, "./cmd/agent")
	cmd.Dir = coreRoot
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Printf("building agent binary from %s...\n", coreRoot)
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build agent: %w", err)
	}
	return binary, nil
}

func freeLoopbackAddr() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()
	return listener.Addr().String(), nil
}

func waitHTTPStatus(url string, want int, timeout time.Duration) error {
	return waitHTTPStatusWithClient(integrationHTTPClient, url, want, timeout)
}

func waitHTTPStatusWithClient(client *http.Client, url string, want int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		ctx, cancel := context.WithTimeout(context.Background(), min(integrationHTTPRequestTimeout, remaining))
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err == nil {
			var resp *http.Response
			resp, err = client.Do(req)
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode == want {
					cancel()
					return nil
				}
				lastErr = fmt.Errorf("status %d", resp.StatusCode)
			}
		}
		cancel()
		if err == nil {
			remaining = time.Until(deadline)
			if remaining > 0 {
				time.Sleep(min(100*time.Millisecond, remaining))
			}
			continue
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = context.DeadlineExceeded
	}
	return fmt.Errorf("wait for %s status %d: %w", url, want, lastErr)
}

func requestHTTP(method, url, body string) ([]byte, int, error) {
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := integrationHTTPClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func waitTCPFree(addr string) error {
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		listener, err := net.Listen("tcp", addr)
		if err == nil {
			return listener.Close()
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("%s was not released after lifecycle exit: %w", addr, lastErr)
}

func stopIntegrationProcess(cmd *exec.Cmd, cancel context.CancelFunc) {
	cancel()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}

func commandWithOutput(cmd *exec.Cmd) (*bytes.Buffer, error) {
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	return &output, cmd.Run()
}

func writeExecutable(path, script, label string) error {
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		return fmt.Errorf("write %s: %w", label, err)
	}
	return nil
}

func readIntegrationYAML(path, label string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", label, err)
	}
	if err := yaml.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parse %s: %w", label, err)
	}
	return nil
}

func writeIntegrationYAML(path, label string, value any) error {
	data, err := yaml.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal %s: %w", label, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", label, err)
	}
	return nil
}

func replaceAll(content string, replacements map[string]string) string {
	for old, replacement := range replacements {
		content = strings.ReplaceAll(content, old, replacement)
	}
	return content
}

func localPort(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return port
}
