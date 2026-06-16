// Copyright (c) 2026 Nokia. All rights reserved.

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

type docsCuratorIntegrationConfig struct {
	profilePath string
	docsAddr    string
	controlAddr string
	requestAddr string
}

// Uc006 runs rel03.0-uc006: documentation-curator serves docs UX and REST tools.
func (Integration) Uc006() error {
	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, cleanup, err := prepareDocsCuratorIntegration(rootDir)
	if err != nil {
		return err
	}
	defer cleanup()
	binary, err := buildFreshAgentFor("uc006")
	if err != nil {
		return err
	}
	cmd, output, cancel := launchDocsCuratorProcess(binary, rootDir, cfg)
	defer cancel()
	defer stopProcess(cmd, cancel)
	if err := waitDocsAPI(cfg.docsAddr); err != nil {
		return fmt.Errorf("uc006: documentation-curator did not become ready: %w\n%s", err, output.String())
	}
	if err := assertDocsCuratorHTTP(cfg.docsAddr); err != nil {
		return err
	}
	if err := assertBenchDocsRoutesAbsent(); err != nil {
		return err
	}
	if err := requestDocsCuratorExit(cfg.controlAddr); err != nil {
		return err
	}
	if err := waitDocsCuratorExit(cmd, output); err != nil {
		return err
	}
	if err := assertDocsCuratorLifecycleExit(output.String()); err != nil {
		return err
	}
	if err := waitDocsCuratorPortsFree("uc006", cfg); err != nil {
		return err
	}
	fmt.Println("uc006: PASS - documentation-curator served docs UX/API and exited through lifecycle control")
	return nil
}

// Uc007 runs rel03.0-uc007: Puppeteer verifies machine_request documentation UX.
func (Integration) Uc007() error {
	browser, err := findPuppeteerBrowser()
	if err != nil {
		return skipUC("uc007", err.Error())
	}
	if _, err := exec.LookPath("npm"); err != nil {
		return skipUC("uc007", "npm is required to run the Puppeteer harness")
	}
	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, cleanup, err := prepareDocsCuratorIntegration(rootDir)
	if err != nil {
		return err
	}
	defer cleanup()
	binary, err := buildFreshAgentFor("uc007")
	if err != nil {
		return err
	}
	cmd, output, cancel := launchDocsCuratorProcess(binary, rootDir, cfg)
	defer cancel()
	defer stopProcess(cmd, cancel)
	if err := waitDocsAPI(cfg.docsAddr); err != nil {
		return fmt.Errorf("uc007: documentation-curator did not become ready: %w\n%s", err, output.String())
	}
	if err := runDocsPuppeteer(rootDir, browser, cfg.docsAddr); err != nil {
		return err
	}
	if err := requestDocsCuratorExit(cfg.controlAddr); err != nil {
		return err
	}
	if err := waitDocsCuratorExit(cmd, output); err != nil {
		return err
	}
	if err := waitDocsCuratorPortsFree("uc007", cfg); err != nil {
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

func launchDocsCuratorProcess(
	binary string,
	rootDir string,
	cfg docsCuratorIntegrationConfig,
) (*exec.Cmd, *bytes.Buffer, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd, output := startDocsCurator(ctx, binary, rootDir, cfg.profilePath)
	return cmd, output, cancel
}

func freeLocalAddr() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer listener.Close()
	return listener.Addr().String(), nil
}

func requireTCPFree(addr string) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("%s is already in use: %w", addr, err)
	}
	return listener.Close()
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

func waitDocsCuratorPortsFree(id string, cfg docsCuratorIntegrationConfig) error {
	for _, addr := range []string{cfg.docsAddr, cfg.controlAddr, cfg.requestAddr} {
		if err := waitTCPFree(addr); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
	}
	return nil
}

func startDocsCurator(ctx context.Context, binary, rootDir, profilePath string) (*exec.Cmd, *bytes.Buffer) {
	args := []string{"--profile", profilePath}
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

func waitDocsAPI(addr string) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get("http://" + addr + "/api/v1/health")
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

func assertDocsCuratorHTTP(addr string) error {
	if err := requireJSONField(addr, "/api/v1/docs", "data"); err != nil {
		return err
	}
	if err := requirePOSTStatus(addr, "/api/v1/docs/validate", `{"paths":["SPECIFICATIONS.yaml"]}`, http.StatusOK); err != nil {
		return err
	}
	if err := requirePOSTStatus(addr, "/api/v1/docs/suggestions", `{"path":"SPECIFICATIONS.yaml","instruction":"Smoke check proposal."}`, http.StatusAccepted); err != nil {
		return err
	}
	return requireHTML(addr, "/docs/SPECIFICATIONS.yaml")
}

func requireJSONField(addr, path, field string) error {
	data, status, err := requestDocs(addr, http.MethodGet, path, "")
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

func prepareDocsCuratorIntegration(rootDir string) (docsCuratorIntegrationConfig, func(), error) {
	tmpDir, err := os.MkdirTemp("", "docs-curator-profile-*")
	if err != nil {
		return docsCuratorIntegrationConfig{}, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	cfg, err := docsCuratorEphemeralConfig(tmpDir)
	if err != nil {
		cleanup()
		return docsCuratorIntegrationConfig{}, nil, err
	}
	if err := writeDocsCuratorProfileFiles(rootDir, tmpDir, cfg); err != nil {
		cleanup()
		return docsCuratorIntegrationConfig{}, nil, err
	}
	return cfg, cleanup, nil
}

func docsCuratorEphemeralConfig(tmpDir string) (docsCuratorIntegrationConfig, error) {
	docsAddr, err := freeLocalAddr()
	if err != nil {
		return docsCuratorIntegrationConfig{}, err
	}
	controlAddr, err := freeLocalAddr()
	if err != nil {
		return docsCuratorIntegrationConfig{}, err
	}
	requestAddr, err := freeLocalAddr()
	if err != nil {
		return docsCuratorIntegrationConfig{}, err
	}
	return docsCuratorIntegrationConfig{
		profilePath: filepath.Join(tmpDir, "profile.yaml"),
		docsAddr:    docsAddr,
		controlAddr: controlAddr,
		requestAddr: requestAddr,
	}, nil
}

func writeDocsCuratorProfileFiles(rootDir, tmpDir string, cfg docsCuratorIntegrationConfig) error {
	writers := []func(string, string, docsCuratorIntegrationConfig) error{
		writeDocsCuratorProfile,
		writeDocsCuratorBuiltin,
		writeDocsCuratorRest,
		writeDocsCuratorOpenAPI,
		copyDocsCuratorRequestMachine,
	}
	for _, write := range writers {
		if err := write(rootDir, tmpDir, cfg); err != nil {
			return err
		}
	}
	return nil
}

func writeDocsCuratorProfile(rootDir, tmpDir string, _ docsCuratorIntegrationConfig) error {
	profile := fmt.Sprintf(`name: documentation-curator
machine: %q
tools:
  - %q
tool_declarations:
  - %q
  - %q
  - %q
  - %q
rest_definitions:
  - %q
`, docsCuratorPath(rootDir, "machine.yaml"), docsCuratorPath(rootDir, "tools.yaml"),
		filepath.Join(tmpDir, "builtin.yaml"), docsCuratorPath(rootDir, "declarations.yaml"),
		docsCuratorPath(rootDir, "request-declarations.yaml"),
		filepath.Join(rootDir, "tools/builtin/lifecycle/exit-agent.yaml"),
		filepath.Join(tmpDir, "rest.yaml"))
	return os.WriteFile(filepath.Join(tmpDir, "profile.yaml"), []byte(profile), 0o644)
}

func writeDocsCuratorBuiltin(rootDir, tmpDir string, cfg docsCuratorIntegrationConfig) error {
	content, err := readDocsCuratorConfig(rootDir, "builtin.yaml")
	if err != nil {
		return err
	}
	replacements := map[string]string{
		"addr: :18081":         "addr: " + fmt.Sprintf("%q", cfg.docsAddr),
		"docs_dir: docs":       "docs_dir: " + fmt.Sprintf("%q", filepath.Join(rootDir, "docs")),
		"configs_dir: configs": "configs_dir: " + fmt.Sprintf("%q", filepath.Join(rootDir, "configs")),
		"source_dir: .":        "source_dir: " + fmt.Sprintf("%q", rootDir),
		"profile_path: agents/knowledge-manager/documentation-curator/profile.yaml": "profile_path: " + fmt.Sprintf("%q", cfg.profilePath),
	}
	return os.WriteFile(filepath.Join(tmpDir, "builtin.yaml"), []byte(replaceAll(content, replacements)), 0o644)
}

func writeDocsCuratorRest(rootDir, tmpDir string, cfg docsCuratorIntegrationConfig) error {
	content, err := readDocsCuratorConfig(rootDir, "rest.yaml")
	if err != nil {
		return err
	}
	replacements := map[string]string{
		"http://127.0.0.1:18081":   "http://" + cfg.docsAddr,
		"ports: [18081]":           "ports: [" + localPort(cfg.docsAddr) + "]",
		"ports: [18082]":           "ports: [" + localPort(cfg.controlAddr) + "]",
		"ports: [18083]":           "ports: [" + localPort(cfg.requestAddr) + "]",
		"address: 127.0.0.1:18082": "address: " + cfg.controlAddr,
		"address: 127.0.0.1:18083": "address: " + cfg.requestAddr,
	}
	return os.WriteFile(filepath.Join(tmpDir, "rest.yaml"), []byte(replaceAll(content, replacements)), 0o644)
}

func writeDocsCuratorOpenAPI(rootDir, tmpDir string, cfg docsCuratorIntegrationConfig) error {
	content, err := readDocsCuratorConfig(rootDir, "openapi.yaml")
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, "http://127.0.0.1:18081", "http://"+cfg.docsAddr)
	return os.WriteFile(filepath.Join(tmpDir, "openapi.yaml"), []byte(content), 0o644)
}

func copyDocsCuratorRequestMachine(rootDir, tmpDir string, _ docsCuratorIntegrationConfig) error {
	content, err := readDocsCuratorConfig(rootDir, "request-machine.yaml")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(tmpDir, "request-machine.yaml"), []byte(content), 0o644)
}

func readDocsCuratorConfig(rootDir, name string) (string, error) {
	data, err := os.ReadFile(docsCuratorPath(rootDir, name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func docsCuratorPath(rootDir, name string) string {
	return filepath.Join(rootDir, "agents/knowledge-manager/documentation-curator", name)
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

func runDocsPuppeteer(rootDir, browser, docsAddr string) error {
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
		"KM_DOCS_BASE_URL=http://"+docsAddr+"/",
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

func requirePOSTStatus(addr, path, body string, want int) error {
	data, status, err := requestDocs(addr, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if status != want {
		return fmt.Errorf("%s returned status %d, want %d: %s", path, status, want, string(data))
	}
	return nil
}

func requireHTML(addr, path string) error {
	data, status, err := requestDocs(addr, http.MethodGet, path, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK || !bytes.Contains(data, []byte("<html")) {
		return fmt.Errorf("%s did not return docs SPA HTML", path)
	}
	return nil
}

func requestDocs(addr, method, path, body string) ([]byte, int, error) {
	req, err := http.NewRequest(method, "http://"+addr+path, strings.NewReader(body))
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

func requestDocsCuratorExit(controlAddr string) error {
	var lastErr error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := postDocsCuratorExit(controlAddr); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("uc006: lifecycle exit was not accepted: %w", lastErr)
}

func postDocsCuratorExit(controlAddr string) error {
	url := "http://" + controlAddr + "/api/lifecycle/exit"
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
