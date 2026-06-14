// Copyright (c) 2026 Petar Djukic. All rights reserved.
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const docsCuratorAddr = "127.0.0.1:18081"

// Uc006 runs rel03.0-uc006: documentation-curator serves docs UX and REST tools.
func (Integration) Uc006() error {
	if err := requireTCPFree(docsCuratorAddr); err != nil {
		return skipUC("uc006", err.Error())
	}
	binary, err := buildFreshAgentFor("uc006")
	if err != nil {
		return err
	}
	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, output := startDocsCurator(ctx, binary, rootDir)
	defer stopProcess(cmd, cancel)
	if err := waitDocsAPI(); err != nil {
		return fmt.Errorf("uc006: documentation-curator did not become ready: %w\n%s", err, output.String())
	}
	if err := assertDocsCuratorHTTP(); err != nil {
		return err
	}
	if err := assertBenchDocsRoutesAbsent(); err != nil {
		return err
	}
	fmt.Println("uc006: PASS — documentation-curator served docs UX/API and bench docs routes stayed absent")
	return nil
}

func buildFreshAgentFor(name string) (string, error) {
	fmt.Printf("building fresh agent binary for %s...\n", name)
	if err := Build(); err != nil {
		return "", fmt.Errorf("build agent: %w", err)
	}
	return filepath.Abs(filepath.Join(binDir, "agent"))
}

func requireTCPFree(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("%s is already in use: %w", addr, err)
	}
	return listener.Close()
}

func startDocsCurator(ctx context.Context, binary, rootDir string) (*exec.Cmd, *bytes.Buffer) {
	args := []string{"--profile", filepath.Join(rootDir, "agents/knowledge-manager/documentation-curator/profile.yaml")}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = rootDir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	fmt.Printf("running: %s %s\n", binary, strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		output.WriteString(err.Error())
	}
	return cmd, &output
}

func waitDocsAPI() error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + docsCuratorAddr + "/api/v1/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for /api/v1/health")
}

func assertDocsCuratorHTTP() error {
	if err := requireJSONField("/api/v1/docs", "data"); err != nil {
		return err
	}
	if err := requirePOSTStatus("/api/v1/docs/validate", `{"paths":["SPECIFICATIONS.yaml"]}`, http.StatusOK); err != nil {
		return err
	}
	if err := requirePOSTStatus("/api/v1/docs/suggestions", `{"path":"SPECIFICATIONS.yaml","instruction":"Smoke check proposal."}`, http.StatusAccepted); err != nil {
		return err
	}
	return requireHTML("/docs/SPECIFICATIONS.yaml")
}

func requireJSONField(path, field string) error {
	data, status, err := requestDocs(http.MethodGet, path, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("%s returned status %d", path, status)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("%s returned invalid JSON: %w", path, err)
	}
	if _, ok := payload[field]; !ok {
		return fmt.Errorf("%s response missing %q", path, field)
	}
	return nil
}

func requirePOSTStatus(path, body string, want int) error {
	data, status, err := requestDocs(http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if status != want {
		return fmt.Errorf("%s returned status %d, want %d: %s", path, status, want, string(data))
	}
	return nil
}

func requireHTML(path string) error {
	data, status, err := requestDocs(http.MethodGet, path, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK || !bytes.Contains(data, []byte("<html")) {
		return fmt.Errorf("%s did not return docs SPA HTML", path)
	}
	return nil
}

func requestDocs(method, path, body string) ([]byte, int, error) {
	req, err := http.NewRequest(method, "http://"+docsCuratorAddr+path, strings.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

func assertBenchDocsRoutesAbsent() error {
	cmd := exec.Command("go", "test", "./internal/evaluation/bench", "-run", "TestDocsAPIRoutesAreNotMounted")
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("bench docs route absence test failed: %w\n%s", err, output.String())
	}
	return nil
}

func stopProcess(cmd *exec.Cmd, cancel context.CancelFunc) {
	cancel()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}
