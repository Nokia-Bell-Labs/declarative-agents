// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
)

const documentationCuratorProfile = "agents/knowledge-manager/documentation-curator"

// Integration contains profile-owned integration tracer bullets.
type Integration mg.Namespace

type documentationCuratorConfig struct {
	profilePath string
	docsAddr    string
	controlAddr string
	requestAddr string
}

// DocumentationCurator proves the Knowledge Manager profile UX and lifecycle tracer.
func (Integration) DocumentationCurator() error {
	profilesRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	coreRoot := envOrDefault(agentCoreRootEnv, filepath.Join(filepath.Dir(profilesRoot), "agent-core"))
	cfg, cleanup, err := prepareDocumentationCuratorIntegration(profilesRoot, coreRoot)
	if err != nil {
		return err
	}
	defer cleanup()
	binary, err := buildIntegrationAgent(coreRoot)
	if err != nil {
		return err
	}
	cmd, output, cancel := launchDocumentationCurator(binary, profilesRoot, coreRoot, cfg.profilePath)
	defer cancel()
	defer stopIntegrationProcess(cmd, cancel)
	if err := waitDocumentationAPI(cfg.docsAddr); err != nil {
		return fmt.Errorf("documentation-curator did not become ready: %w\n%s", err, output.String())
	}
	if err := assertDocumentationCuratorHTTP(cfg.docsAddr); err != nil {
		return err
	}
	if err := requestDocumentationCuratorExit(cfg.controlAddr); err != nil {
		return err
	}
	if err := waitDocumentationCuratorExit(cmd, output); err != nil {
		return err
	}
	if err := assertDocumentationCuratorLifecycleExit(output.String()); err != nil {
		return err
	}
	if err := waitDocumentationCuratorPortsFree(cfg); err != nil {
		return err
	}
	fmt.Println("integration:documentationCurator PASS - profile served docs, emitted trace metadata, and exited cleanly")
	return nil
}

func prepareDocumentationCuratorIntegration(profilesRoot, coreRoot string) (documentationCuratorConfig, func(), error) {
	tmpDir, err := os.MkdirTemp("", "agent-profiles-docs-curator-*")
	if err != nil {
		return documentationCuratorConfig{}, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	cfg, err := ephemeralDocumentationCuratorConfig(tmpDir)
	if err != nil {
		cleanup()
		return documentationCuratorConfig{}, nil, err
	}
	if err := writeDocumentationCuratorProfileFiles(profilesRoot, coreRoot, tmpDir, cfg); err != nil {
		cleanup()
		return documentationCuratorConfig{}, nil, err
	}
	return cfg, cleanup, nil
}

func ephemeralDocumentationCuratorConfig(tmpDir string) (documentationCuratorConfig, error) {
	docsAddr, err := freeLoopbackAddr()
	if err != nil {
		return documentationCuratorConfig{}, err
	}
	controlAddr, err := freeLoopbackAddr()
	if err != nil {
		return documentationCuratorConfig{}, err
	}
	requestAddr, err := freeLoopbackAddr()
	if err != nil {
		return documentationCuratorConfig{}, err
	}
	return documentationCuratorConfig{
		profilePath: filepath.Join(tmpDir, "profile.yaml"),
		docsAddr:    docsAddr,
		controlAddr: controlAddr,
		requestAddr: requestAddr,
	}, nil
}

func writeDocumentationCuratorProfileFiles(profilesRoot, coreRoot, tmpDir string, cfg documentationCuratorConfig) error {
	writers := []func(string, string, string, documentationCuratorConfig) error{
		writeDocumentationCuratorProfile,
		writeDocumentationCuratorBuiltin,
		writeDocumentationCuratorRest,
		writeDocumentationCuratorOpenAPI,
		copyDocumentationCuratorRequestMachine,
		copyDocumentationCuratorUXConfig,
	}
	for _, write := range writers {
		if err := write(profilesRoot, coreRoot, tmpDir, cfg); err != nil {
			return err
		}
	}
	return nil
}

func writeDocumentationCuratorProfile(profilesRoot, coreRoot, tmpDir string, _ documentationCuratorConfig) error {
	profileDir := documentationCuratorPath(profilesRoot, "")
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
`, filepath.Join(profileDir, "machine.yaml"),
		filepath.Join(profileDir, "tools.yaml"),
		filepath.Join(tmpDir, "builtin.yaml"),
		filepath.Join(profileDir, "declarations.yaml"),
		filepath.Join(profileDir, "request-declarations.yaml"),
		filepath.Join(coreRoot, "tools", "builtin", "lifecycle", "exit-agent.yaml"),
		filepath.Join(tmpDir, "rest.yaml"))
	return os.WriteFile(filepath.Join(tmpDir, "profile.yaml"), []byte(profile), 0o644)
}

func writeDocumentationCuratorBuiltin(profilesRoot, coreRoot, tmpDir string, cfg documentationCuratorConfig) error {
	content, err := readDocumentationCuratorConfig(profilesRoot, "builtin.yaml")
	if err != nil {
		return err
	}
	replacements := map[string]string{
		"addr: :18081":         "addr: " + fmt.Sprintf("%q", cfg.docsAddr),
		"docs_dir: docs":       "docs_dir: " + fmt.Sprintf("%q", filepath.Join(coreRoot, "docs")),
		"configs_dir: configs": "configs_dir: " + fmt.Sprintf("%q", filepath.Join(coreRoot, "configs")),
		"source_dir: .":        "source_dir: " + fmt.Sprintf("%q", coreRoot),
		"profile_path: agents/knowledge-manager/documentation-curator/profile.yaml": "profile_path: " + fmt.Sprintf("%q", cfg.profilePath),
	}
	return os.WriteFile(filepath.Join(tmpDir, "builtin.yaml"), []byte(replaceAll(content, replacements)), 0o644)
}

func writeDocumentationCuratorRest(profilesRoot, _ string, tmpDir string, cfg documentationCuratorConfig) error {
	content, err := readDocumentationCuratorConfig(profilesRoot, "rest.yaml")
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

func writeDocumentationCuratorOpenAPI(profilesRoot, _ string, tmpDir string, cfg documentationCuratorConfig) error {
	content, err := readDocumentationCuratorConfig(profilesRoot, "openapi.yaml")
	if err != nil {
		return err
	}
	content = strings.ReplaceAll(content, "http://127.0.0.1:18081", "http://"+cfg.docsAddr)
	return os.WriteFile(filepath.Join(tmpDir, "openapi.yaml"), []byte(content), 0o644)
}

func copyDocumentationCuratorRequestMachine(profilesRoot, _ string, tmpDir string, _ documentationCuratorConfig) error {
	content, err := readDocumentationCuratorConfig(profilesRoot, "request-machine.yaml")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(tmpDir, "request-machine.yaml"), []byte(content), 0o644)
}

func copyDocumentationCuratorUXConfig(profilesRoot, _ string, tmpDir string, _ documentationCuratorConfig) error {
	content, err := readDocumentationCuratorConfig(profilesRoot, filepath.Join("ui", "ux.yaml"))
	if err != nil {
		return err
	}
	uiDir := filepath.Join(tmpDir, "ui")
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(uiDir, "ux.yaml"), []byte(content), 0o644)
}

func launchDocumentationCurator(binary, profilesRoot, coreRoot, profilePath string) (*exec.Cmd, *bytes.Buffer, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	args := []string{"--profile", profilePath, "--directory", coreRoot, "--core-root", coreRoot}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = profilesRoot
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	fmt.Printf("running: %s %s\n", binary, strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		output.WriteString(err.Error())
	}
	return cmd, &output, cancel
}

func assertDocumentationCuratorHTTP(addr string) error {
	if err := requireJSONField(addr, "/api/v1/docs", "data"); err != nil {
		return err
	}
	if err := requireDocumentTrace(addr, "/api/v1/docs/SPECIFICATIONS.yaml"); err != nil {
		return err
	}
	if err := requireJSONField(addr, "/api/v1/ux", "data"); err != nil {
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

func requireDocumentTrace(addr, path string) error {
	data, status, err := requestDocumentation(addr, http.MethodGet, path, "")
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
	trace, _ := payload["trace"].(map[string]interface{})
	if trace == nil {
		return fmt.Errorf("%s response missing trace metadata", path)
	}
	for _, field := range []string{"server", "route", "machine", "terminal_signal", "status"} {
		if trace[field] == nil {
			return fmt.Errorf("%s trace missing %q: %#v", path, field, trace)
		}
	}
	if trace["terminal_signal"] != "DocumentDetailReady" {
		return fmt.Errorf("%s terminal_signal = %v, want DocumentDetailReady", path, trace["terminal_signal"])
	}
	return nil
}

func waitDocumentationAPI(addr string) error {
	if err := waitHTTPStatus("http://"+addr+"/api/v1/health", http.StatusOK, 10*time.Second); err != nil {
		return fmt.Errorf("timeout waiting for /api/v1/health: %w", err)
	}
	return nil
}

func requireJSONField(addr, path, field string) error {
	data, status, err := requestDocumentation(addr, http.MethodGet, path, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("%s returned status %d: %s", path, status, string(data))
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

func requirePOSTStatus(addr, path, body string, want int) error {
	data, status, err := requestDocumentation(addr, http.MethodPost, path, body)
	if err != nil {
		return err
	}
	if status != want {
		return fmt.Errorf("%s returned status %d, want %d: %s", path, status, want, string(data))
	}
	return nil
}

func requireHTML(addr, path string) error {
	data, status, err := requestDocumentation(addr, http.MethodGet, path, "")
	if err != nil {
		return err
	}
	if status != http.StatusOK || !bytes.Contains(data, []byte("<html")) {
		return fmt.Errorf("%s did not return docs SPA HTML", path)
	}
	return nil
}

func requestDocumentation(addr, method, path, body string) ([]byte, int, error) {
	return requestHTTP(method, "http://"+addr+path, body)
}

func requestDocumentationCuratorExit(controlAddr string) error {
	var lastErr error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := postDocumentationCuratorExit(controlAddr)
		if err == nil {
			return nil
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("lifecycle exit was not accepted: %w", lastErr)
}

func postDocumentationCuratorExit(controlAddr string) error {
	url := "http://" + controlAddr + "/api/lifecycle/exit"
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(`{"reason":"profile integration complete","status":"success"}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := integrationHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("post lifecycle exit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("lifecycle exit returned status %d", resp.StatusCode)
	}
	return nil
}

func waitDocumentationCuratorExit(cmd *exec.Cmd, output *bytes.Buffer) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("documentation-curator exit failed: %w\n%s", err, output.String())
		}
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("documentation-curator did not exit after lifecycle request\n%s", output.String())
	}
}

func assertDocumentationCuratorLifecycleExit(output string) error {
	if strings.Contains(output, "terminal state: cancelled") || strings.Contains(output, "status=cancelled") {
		return fmt.Errorf("lifecycle exit cancelled the run instead of reaching Done\n%s", output)
	}
	if !strings.Contains(output, "terminal state: succeeded") {
		return fmt.Errorf("lifecycle exit did not report terminal state succeeded\n%s", output)
	}
	if !strings.Contains(output, "run complete: status=succeeded") {
		return fmt.Errorf("lifecycle exit did not record a succeeded RunResult\n%s", output)
	}
	return nil
}

func waitDocumentationCuratorPortsFree(cfg documentationCuratorConfig) error {
	for _, addr := range []string{cfg.docsAddr, cfg.controlAddr, cfg.requestAddr} {
		if err := waitTCPFree(addr); err != nil {
			return err
		}
	}
	return nil
}

func readDocumentationCuratorConfig(profilesRoot, name string) (string, error) {
	data, err := os.ReadFile(documentationCuratorPath(profilesRoot, name))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func documentationCuratorPath(profilesRoot, name string) string {
	return filepath.Join(profilesRoot, documentationCuratorProfile, name)
}
