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

const (
	monitoredQwenPrompt = "List the local Ollama models available on this machine."
	tokenMetricPollWait = 180 * time.Second
)

// Uc008 runs rel05.0-uc001: Qwen exposes live token metrics through the embedded monitor.
func (Integration) Uc008() error {
	model := configuredOllamaModel()
	if _, err := requireOllamaModels(model); err != nil {
		return skipUC("uc008", err.Error())
	}
	binary, err := buildFreshAgentFor("uc008")
	if err != nil {
		return err
	}
	rootDir, err := os.Getwd()
	if err != nil {
		return err
	}
	run, cleanup, err := prepareMonitoredQwenRun(rootDir, model)
	if err != nil {
		return fmt.Errorf("uc008: prepare monitored profile: %w", err)
	}
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, output, resultCh := startMonitoredQwen(ctx, binary, rootDir, run.profilePath)
	defer stopProcess(cmd, cancel)
	if err := waitMonitorHTTP(run.baseURL + "/monitor/state"); err != nil {
		return fmt.Errorf("uc008: monitor did not become ready: %w\n%s", err, output.String())
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
	fmt.Printf("uc008: PASS - %s monitor token metrics increased while Qwen ran\n", model)
	return nil
}

type monitoredQwenRun struct {
	profilePath string
	baseURL     string
}

func prepareMonitoredQwenRun(rootDir, model string) (monitoredQwenRun, func(), error) {
	tmpDir, err := os.MkdirTemp("", "uc008-qwen-monitor-*")
	if err != nil {
		return monitoredQwenRun{}, nil, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }
	profileRoot, err := resolveAgentProfilesRoot(rootDir)
	if err != nil {
		cleanup()
		return monitoredQwenRun{}, nil, err
	}
	addr, err := freeLoopbackAddress()
	if err != nil {
		cleanup()
		return monitoredQwenRun{}, nil, err
	}
	if err := writeMonitoredQwenFiles(rootDir, profileRoot, tmpDir, model, addr); err != nil {
		cleanup()
		return monitoredQwenRun{}, nil, err
	}
	return monitoredQwenRun{
		profilePath: filepath.Join(tmpDir, "profile.yaml"),
		baseURL:     "http://" + addr,
	}, cleanup, nil
}

func writeMonitoredQwenFiles(rootDir, profileRoot, tmpDir, model, addr string) error {
	files := []monitoredQwenFixture{
		{name: "machine.yaml", fixture: "machine.yaml"},
		{name: "tools.yaml", fixture: "tools.yaml"},
		{name: "llm.yaml", fixture: "llm.yaml.tmpl", values: map[string]string{"MODEL": fmt.Sprintf("%q", model)}},
		{name: "monitor-rest.yaml", fixture: "monitor-rest.yaml.tmpl", values: map[string]string{"ADDRESS": addr}},
		{name: "profile.yaml", fixture: "profile.yaml.tmpl", values: monitoredQwenProfileValues(rootDir, profileRoot, tmpDir)},
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

func monitoredQwenProfileValues(rootDir, profileRoot, tmpDir string) map[string]string {
	return map[string]string{
		"MACHINE_PATH":              filepath.Join(tmpDir, "machine.yaml"),
		"TOOLS_PATH":                filepath.Join(tmpDir, "tools.yaml"),
		"LLM_DECLARATIONS_PATH":     abs(rootDir, "tools/builtin/llm/all.yaml"),
		"LLM_OVERRIDE_PATH":         filepath.Join(tmpDir, "llm.yaml"),
		"OLLAMA_DECLARATIONS_PATH":  profileAbs(profileRoot, "rest/ollama-declarations.yaml"),
		"MONITOR_DECLARATIONS_PATH": profileAbs(profileRoot, "monitor/declarations.yaml"),
		"OLLAMA_REST_PATH":          profileAbs(profileRoot, "rest/ollama-rest.yaml"),
		"MONITOR_REST_PATH":         filepath.Join(tmpDir, "monitor-rest.yaml"),
	}
}

func renderMonitoredQwenFixture(rootDir, name string, values map[string]string) (string, error) {
	path := filepath.Join(rootDir, "magefiles", "fixtures", "uc008", name)
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

func startMonitoredQwen(
	ctx context.Context,
	binary string,
	rootDir string,
	profilePath string,
) (*exec.Cmd, *bytes.Buffer, <-chan error) {
	args := []string{"--profile", profilePath, "--request", monitoredQwenPrompt}
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

func waitMonitorHTTP(url string) error {
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
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

func waitForTokenMetricIncrease(url string, resultCh <-chan error, output *bytes.Buffer) error {
	client := http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(tokenMetricPollWait)
	first := 0.0
	for time.Now().Before(deadline) {
		select {
		case err := <-resultCh:
			return fmt.Errorf("uc008: agent exited before token metrics increased: %w\n%s", err, output.String())
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
	return fmt.Errorf("uc008: token metrics did not increase within %s\n%s", tokenMetricPollWait, output.String())
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
	resp, err := http.Post(url, "application/json", strings.NewReader(`{"reason":"uc008 complete"}`))
	if err != nil {
		return fmt.Errorf("uc008: post monitor exit: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return fmt.Errorf("uc008: monitor exit returned status %d", resp.StatusCode)
	}
	return nil
}

func waitMonitoredQwenExit(resultCh <-chan error, output *bytes.Buffer) error {
	select {
	case err := <-resultCh:
		if err != nil {
			return fmt.Errorf("uc008: monitored Qwen run failed: %w\n%s", err, output.String())
		}
		if !strings.Contains(output.String(), "terminal state: succeeded") {
			return fmt.Errorf("uc008: monitored Qwen run did not report succeeded\n%s", output.String())
		}
		return nil
	case <-time.After(10 * time.Second):
		return fmt.Errorf("uc008: monitored Qwen run did not exit after monitor control request\n%s", output.String())
	}
}

func stopProcess(cmd *exec.Cmd, cancel context.CancelFunc) {
	cancel()
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	_ = cmd.Wait()
}
