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

const (
	docsCuratorAddr        = "127.0.0.1:18081"
	docsCuratorControlAddr = "127.0.0.1:18082"
)

// Uc006 runs rel03.0-uc006: documentation-curator serves docs UX and REST tools.
func (Integration) Uc006() error {
	if err := requireDocsCuratorPortsFree("uc006"); err != nil {
		return err
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
	if err := requestDocsCuratorExit(); err != nil {
		return err
	}
	if err := waitDocsCuratorExit(cmd, output); err != nil {
		return err
	}
	if err := assertDocsCuratorLifecycleExit(output.String()); err != nil {
		return err
	}
	if err := waitTCPFree(docsCuratorAddr); err != nil {
		return err
	}
	if err := waitTCPFree(docsCuratorControlAddr); err != nil {
		return err
	}
	fmt.Println("uc006: PASS — documentation-curator served docs UX/API and exited through lifecycle control")
	return nil
}

// Uc007 runs rel03.0-uc007: Puppeteer verifies machine_request documentation UX.
func (Integration) Uc007() error {
	if err := requireDocsCuratorPortsFree("uc007"); err != nil {
		return err
	}
	browser, err := findPuppeteerBrowser()
	if err != nil {
		return skipUC("uc007", err.Error())
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return skipUC("uc007", "npm is required to run the Puppeteer harness")
	}
	binary, err := buildFreshAgentFor("uc007")
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
		return fmt.Errorf("uc007: documentation-curator did not become ready: %w\n%s", err, output.String())
	}
	if err := runDocsPuppeteer(rootDir, browser); err != nil {
		return err
	}
	if err := requestDocsCuratorExit(); err != nil {
		return err
	}
	if err := waitDocsCuratorExit(cmd, output); err != nil {
		return err
	}
	fmt.Println("uc007: PASS - Puppeteer verified machine_request documentation UX")
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

func requireDocsCuratorPortsFree(id string) error {
	if err := requireTCPFree(docsCuratorAddr); err != nil {
		return skipUC(id, err.Error())
	}
	if err := requireTCPFree(docsCuratorControlAddr); err != nil {
		return skipUC(id, err.Error())
	}
	return nil
}

func waitTCPFree(addr string) error {
	deadline := time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := requireTCPFree(addr); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("uc006: %s was not released after lifecycle exit: %w", addr, lastErr)
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

func findPuppeteerBrowser() (string, error) {
	if configured := os.Getenv("PUPPETEER_EXECUTABLE_PATH"); configured != "" {
		return configured, nil
	}
	if configured := os.Getenv("CHROME_BIN"); configured != "" {
		return configured, nil
	}
	names := []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable"}
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	paths := []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("set PUPPETEER_EXECUTABLE_PATH or CHROME_BIN to a local Chrome/Chromium binary")
}

func runDocsPuppeteer(rootDir, browser string) error {
	artifacts, err := os.MkdirTemp("", "uc007-puppeteer-*")
	if err != nil {
		return fmt.Errorf("uc007: create artifact dir: %w", err)
	}
	uiDir := filepath.Join(rootDir, "internal/knowledge/documentation/ui")
	cmd := exec.Command("npm", "run", "test:e2e:machine-request")
	cmd.Dir = uiDir
	cmd.Env = append(os.Environ(),
		"AGENT_CORE_MACHINE_REQUEST_CONFORMANCE=1",
		"PUPPETEER_EXECUTABLE_PATH="+browser,
		"KM_DOCS_BASE_URL=http://"+docsCuratorAddr+"/",
		"KM_DOCS_ARTIFACT_DIR="+artifacts,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("uc007: Puppeteer proof failed; artifacts at %s: %w", artifacts, err)
	}
	fmt.Printf("uc007: Puppeteer artifacts at %s\n", artifacts)
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

func requestDocsCuratorExit() error {
	var lastErr error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := postDocsCuratorExit(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("uc006: lifecycle exit was not accepted: %w", lastErr)
}

func postDocsCuratorExit() error {
	url := "http://" + docsCuratorControlAddr + "/api/lifecycle/exit"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"reason":"uc006 smoke exit","status":"success"}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("uc006: post lifecycle exit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("uc006: lifecycle exit returned status %d", resp.StatusCode)
	}
	return nil
}

func waitDocsCuratorExit(cmd *exec.Cmd, output *bytes.Buffer) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("uc006: documentation-curator exit failed: %w\n%s", err, output.String())
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("uc006: documentation-curator did not exit after lifecycle request\n%s", output.String())
	}
}

func assertDocsCuratorLifecycleExit(output string) error {
	if strings.Contains(output, "terminal state: cancelled") || strings.Contains(output, "status=cancelled") {
		return fmt.Errorf("uc006: lifecycle exit cancelled the run instead of reaching Done\n%s", output)
	}
	if !strings.Contains(output, "terminal state: succeeded") {
		return fmt.Errorf("uc006: lifecycle exit did not report terminal state succeeded\n%s", output)
	}
	if !strings.Contains(output, "run complete: status=succeeded") {
		return fmt.Errorf("uc006: lifecycle exit did not record a succeeded RunResult\n%s", output)
	}
	return nil
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
