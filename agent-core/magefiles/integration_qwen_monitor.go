// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	monitoredQwenPrompt = "List the local Ollama models available on this machine."
	tokenMetricPollWait = 180 * time.Second
)

// OllamaMonitor proves live LLM token metrics through the embedded monitor.
func (Integration) OllamaMonitor() error {
	beginUC("ollamaMonitor")
	model, err := configuredOllamaModel()
	if err != nil {
		return fmt.Errorf("ollamaMonitor: resolve configured model: %w", err)
	}
	if _, err := requireOllamaModels(model); err != nil {
		return skipUC("ollamaMonitor", err.Error())
	}
	binary, err := buildFreshAgentFor("ollamaMonitor")
	if err != nil {
		return err
	}
	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}
	run, cleanup, err := prepareMonitoredQwenRun(rootDir, model)
	if err != nil {
		return fmt.Errorf("ollamaMonitor: prepare monitored profile: %w", err)
	}
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, output, resultCh := startMonitoredQwen(ctx, binary, rootDir, run.profilePath, run.requestPath)
	defer func() {
		cancel()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	if err := waitMonitoredQwenHTTP(run.baseURL+"/monitor/state", resultCh, output); err != nil {
		return fmt.Errorf("ollamaMonitor: monitor did not become ready: %w", err)
	}
	if err := waitForTokenMetricIncrease(run.baseURL+"/monitor/metrics", resultCh, output); err != nil {
		return err
	}
	if err := postMonitorExit(run.baseURL + "/monitor/control/exit"); err != nil {
		return err
	}
	if err := waitMonitoredQwenExit(resultCh, output); err != nil {
		return err
	}
	fmt.Printf("ollamaMonitor: PASS - %s monitor token metrics increased while Ollama ran\n", model)
	return nil
}

type monitoredQwenRun struct {
	profilePath string
	requestPath string
	baseURL     string
}

func prepareMonitoredQwenRun(rootDir, model string) (monitoredQwenRun, func(), error) {
	tmpDir, err := os.MkdirTemp("", "ollama-monitor-*")
	if err != nil {
		return monitoredQwenRun{}, nil, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }
	addr, err := freeLoopbackAddress()
	if err != nil {
		cleanup()
		return monitoredQwenRun{}, nil, err
	}
	if err := writeMonitoredQwenFiles(rootDir, tmpDir, model, addr); err != nil {
		cleanup()
		return monitoredQwenRun{}, nil, err
	}
	requestPath := filepath.Join(tmpDir, "request.txt")
	if err := os.WriteFile(requestPath, []byte(monitoredQwenPrompt), 0o644); err != nil {
		cleanup()
		return monitoredQwenRun{}, nil, err
	}
	return monitoredQwenRun{
		profilePath: filepath.Join(tmpDir, "profile.yaml"),
		requestPath: requestPath,
		baseURL:     "http://" + addr,
	}, cleanup, nil
}

func writeMonitoredQwenFiles(rootDir, tmpDir, model, addr string) error {
	files := []monitoredQwenFixture{
		{name: "machine.yaml", fixture: "machine.yaml"},
		{name: "tools.yaml", fixture: "tools.yaml"},
		{name: "llm.yaml", fixture: "llm.yaml.tmpl", values: map[string]string{"MODEL": fmt.Sprintf("%q", model)}},
		{name: "monitor-rest.yaml", fixture: "monitor-rest.yaml.tmpl", values: map[string]string{"ADDRESS": addr}},
		{name: "profile.yaml", fixture: "profile.yaml.tmpl", values: monitoredQwenProfileValues(rootDir, tmpDir)},
	}
	for _, file := range files {
		content, err := renderMonitoredQwenFixture(rootDir, file.fixture, file.values)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(tmpDir, file.name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

type monitoredQwenFixture struct {
	name    string
	fixture string
	values  map[string]string
}

func monitoredQwenProfileValues(rootDir, tmpDir string) map[string]string {
	return map[string]string{
		"MACHINE_PATH":              filepath.Join(tmpDir, "machine.yaml"),
		"TOOLS_PATH":                filepath.Join(tmpDir, "tools.yaml"),
		"LLM_DECLARATIONS_PATH":     abs(rootDir, "tools/builtin/llm/all.yaml"),
		"LLM_OVERRIDE_PATH":         filepath.Join(tmpDir, "llm.yaml"),
		"OLLAMA_DECLARATIONS_PATH":  coreIntegrationProfilePath(rootDir, "ollama-rest/declarations.yaml"),
		"MONITOR_DECLARATIONS_PATH": coreIntegrationProfilePath(rootDir, "monitor/declarations.yaml"),
		"OLLAMA_REST_PATH":          coreIntegrationProfilePath(rootDir, "ollama-rest/rest.yaml"),
		"MONITOR_REST_PATH":         filepath.Join(tmpDir, "monitor-rest.yaml"),
	}
}

func renderMonitoredQwenFixture(rootDir, name string, values map[string]string) (string, error) {
	path := coreIntegrationProfilePath(rootDir, filepath.Join("ollama-monitor", name))
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read Ollama monitor fixture %s: %w", path, err)
	}
	content := string(data)
	for key, value := range values {
		content = strings.ReplaceAll(content, "{{"+key+"}}", value)
	}
	if strings.Contains(content, "{{") {
		return "", fmt.Errorf("Ollama monitor fixture %s has unresolved template values", path)
	}
	return content, nil
}

func startMonitoredQwen(
	ctx context.Context,
	binary string,
	rootDir string,
	profilePath string,
	requestPath string,
) (*exec.Cmd, *bytes.Buffer, <-chan error) {
	args := monitoredQwenArgs(profilePath, requestPath)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = rootDir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	resultCh := make(chan error, 1)
	fmt.Printf("running: %s %s\n", binary, strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		output.WriteString(err.Error())
		resultCh <- err
		return cmd, &output, resultCh
	}
	go func() { resultCh <- cmd.Wait() }()
	return cmd, &output, resultCh
}

func monitoredQwenArgs(profilePath, requestPath string) []string {
	return []string{"--profile", profilePath, "--request", requestPath}
}

func waitMonitorHTTP(url string) error {
	return waitHTTPStatus(url, http.StatusOK, 10*time.Second)
}

func waitMonitoredQwenHTTP(url string, resultCh <-chan error, output *bytes.Buffer) error {
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-resultCh:
			if err == nil {
				err = errors.New("agent exited without an error")
			}
			return fmt.Errorf("agent exited before monitor readiness: %w\n%s", err, output.String())
		default:
		}
		remaining := time.Until(deadline)
		if err := waitHTTPStatus(url, http.StatusOK, min(250*time.Millisecond, remaining)); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	return fmt.Errorf("%w\n%s", lastErr, output.String())
}

func waitForTokenMetricIncrease(url string, resultCh <-chan error, output *bytes.Buffer) error {
	client := http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(tokenMetricPollWait)
	first := 0.0
	for time.Now().Before(deadline) {
		select {
		case err := <-resultCh:
			return fmt.Errorf("ollamaMonitor: agent exited before token metrics increased: %w\n%s", err, output.String())
		default:
		}
		total, ok, err := readTokenMetricTotal(client, url)
		if err == nil && ok {
			if total > first {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("ollamaMonitor: token metrics did not increase within %s\n%s", tokenMetricPollWait, output.String())
}

func readTokenMetricTotal(client http.Client, url string) (float64, bool, error) {
	resp, err := client.Get(url)
	if err != nil {
		return 0, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 0, false, fmt.Errorf("%s returned status %d", url, resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, false, err
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, false, err
	}
	return metricAggregateSum(payload, "llm.prompt_tokens") + metricAggregateSum(payload, "llm.completion_tokens"),
		metricAggregatePresent(payload, "llm.prompt_tokens") || metricAggregatePresent(payload, "llm.completion_tokens"), nil
}

func metricAggregateSum(payload map[string]interface{}, name string) float64 {
	metrics, _ := payload["metrics"].(map[string]interface{})
	metric, _ := metrics[name].(map[string]interface{})
	for _, key := range []string{"sum", "Sum", "last_value", "LastValue"} {
		if value, ok := numeric(metric[key]); ok {
			return value
		}
	}
	return 0
}

func metricAggregatePresent(payload map[string]interface{}, name string) bool {
	metrics, _ := payload["metrics"].(map[string]interface{})
	_, ok := metrics[name]
	return ok
}

func numeric(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	default:
		return 0, false
	}
}

func postMonitorExit(url string) error {
	if err := postJSONStatus(url, `{"reason":"ollamaMonitor complete"}`, http.StatusAccepted); err != nil {
		return fmt.Errorf("ollamaMonitor: post monitor exit: %w", err)
	}
	return nil
}

func waitMonitoredQwenExit(resultCh <-chan error, output *bytes.Buffer) error {
	select {
	case err := <-resultCh:
		if err != nil {
			return fmt.Errorf("ollamaMonitor: monitored Ollama run failed: %w\n%s", err, output.String())
		}
		if !strings.Contains(output.String(), "terminal state: succeeded") {
			return fmt.Errorf("ollamaMonitor: monitored Ollama run did not report succeeded\n%s", output.String())
		}
		return nil
	case <-time.After(tokenMetricPollWait):
		return fmt.Errorf("ollamaMonitor: monitored Ollama run did not exit after monitor control request\n%s", output.String())
	}
}
