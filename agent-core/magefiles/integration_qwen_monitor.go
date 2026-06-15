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
	addr, err := freeLoopbackAddress()
	if err != nil {
		cleanup()
		return monitoredQwenRun{}, nil, err
	}
	if err := writeMonitoredQwenFiles(rootDir, tmpDir, model, addr); err != nil {
		cleanup()
		return monitoredQwenRun{}, nil, err
	}
	return monitoredQwenRun{
		profilePath: filepath.Join(tmpDir, "profile.yaml"),
		baseURL:     "http://" + addr,
	}, cleanup, nil
}

func writeMonitoredQwenFiles(rootDir, tmpDir, model, addr string) error {
	files := map[string]string{
		"machine.yaml":      monitoredQwenMachine(),
		"tools.yaml":        monitoredQwenTools(),
		"llm.yaml":          monitoredQwenLLM(model),
		"monitor-rest.yaml": monitoredQwenREST(addr),
		"profile.yaml":      monitoredQwenProfile(rootDir, tmpDir),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func monitoredQwenProfile(rootDir, tmpDir string) string {
	return fmt.Sprintf(`name: monitored-qwen-ollama-rest
machine: %s
tools:
  - %s
tool_declarations:
  - %s
  - %s
  - %s
  - %s
rest_definitions:
  - %s
  - %s
`, filepath.Join(tmpDir, "machine.yaml"), filepath.Join(tmpDir, "tools.yaml"),
		abs(rootDir, "tools/builtin/llm/all.yaml"), filepath.Join(tmpDir, "llm.yaml"),
		abs(rootDir, "agents/rest/ollama-declarations.yaml"), abs(rootDir, "agents/monitor/declarations.yaml"),
		abs(rootDir, "agents/rest/ollama-rest.yaml"), filepath.Join(tmpDir, "monitor-rest.yaml"))
}

func monitoredQwenTools() string {
	return `tools:
  - launch_monitor_rest
  - invoke_llm
  - parse_response
  - report_parse_error
  - done
  - ollama_list_models
  - await_monitor_control
  - stop_monitor_rest
`
}

func monitoredQwenMachine() string {
	return `name: monitored-qwen-ollama-rest
purpose: Start the embedded monitor without blocking Qwen, then prove token metrics while the run remains queryable.
initial_state: Idle
metric_labels:
  profile: monitored_qwen
budget:
  max_iterations: 12
  max_consecutive_parse_errors: 2
states:
  - name: Idle
  - name: StartingMonitor
  - name: Composing
  - name: Parsing
  - name: AwaitingMonitorExit
  - name: StoppingMonitor
  - name: Succeeded
  - name: Failed
  - name: BudgetExceeded
terminal_states:
  - Succeeded
  - Failed
  - BudgetExceeded
signals:
  - name: Seed
  - name: ServerLaunched
  - name: LLMResponded
  - name: ToolDone
  - name: ParseFailed
  - name: TaskCompleted
  - name: RESTResponded
  - name: RESTDomainFailed
  - name: ExitRequested
  - name: AwaitTimedOut
  - name: ServerStopped
  - name: CommandError
  - name: BudgetExhausted
transitions:
  - state: Idle
    signal: Seed
    next: StartingMonitor
    action: launch_monitor_rest
    metric_labels:
      route_group: monitor
  - state: StartingMonitor
    signal: ServerLaunched
    next: Composing
    action: invoke_llm
  - state: StartingMonitor
    signal: CommandError
    next: Failed
  - state: Composing
    signal: LLMResponded
    next: Parsing
    action: parse_response
  - state: Parsing
    signal: ToolDone
    next: Composing
    action: $tool
  - state: Parsing
    signal: TaskCompleted
    next: AwaitingMonitorExit
    action: await_monitor_control
  - state: Parsing
    signal: ParseFailed
    next: Composing
    action: report_parse_error
  - state: Composing
    signal: ToolDone
    next: Composing
    action: invoke_llm
  - state: Composing
    signal: RESTResponded
    next: Composing
    action: invoke_llm
  - state: Composing
    signal: RESTDomainFailed
    next: Composing
    action: invoke_llm
  - state: AwaitingMonitorExit
    signal: ExitRequested
    next: StoppingMonitor
    action: stop_monitor_rest
  - state: AwaitingMonitorExit
    signal: AwaitTimedOut
    next: Failed
  - state: AwaitingMonitorExit
    signal: ServerStopped
    next: Failed
  - state: StoppingMonitor
    signal: ServerStopped
    next: Succeeded
  - state: Composing
    signal: CommandError
    next: Failed
  - state: Parsing
    signal: CommandError
    next: Failed
  - state: AwaitingMonitorExit
    signal: CommandError
    next: Failed
  - state: StoppingMonitor
    signal: CommandError
    next: Failed
  - state: Composing
    signal: BudgetExhausted
    next: BudgetExceeded
`
}

func monitoredQwenLLM(model string) string {
	return fmt.Sprintf(`tools:
  - name: invoke_llm
    type: builtin
    init: invoke_llm
    visibility: internal
    emits: [LLMResponded, CommandError]
    description: Invoke the configured Qwen Ollama model while emitting monitor token metrics.
    category: boundary
    output:
      description: Raw Qwen response and token usage.
      schema:
        type: object
        properties:
          response: {type: string}
          provider: {type: string}
          model: {type: string}
          prompt_tokens: {type: integer}
          completion_tokens: {type: integer}
    metrics:
      instruments:
        - name: llm.prompt_tokens
          kind: histogram
          unit: "1"
          description: Prompt tokens returned by the provider.
          value_source: prompt_tokens
          attributes: [provider, model]
        - name: llm.completion_tokens
          kind: histogram
          unit: "1"
          description: Completion tokens returned by the provider.
          value_source: completion_tokens
          attributes: [provider, model]
      attributes:
        - name: provider
          source: config_literal
          cardinality: bounded
          allowed_values: [ollama]
        - name: model
          source: config_literal
          cardinality: bounded
          allowed_values: [%q]
    config:
      model: %q
      provider: ollama
      provider_url: "http://localhost:11434"
      manifest_state: Composing
      response_profile: qwen
      max_time: 120
      llm_timeout: 120
      system_prompt: |
        You list local Ollama models by using the provided REST tool.

        Rules:
        - First call ollama_list_models. Do not answer from memory.
        - After the REST result returns, call done with a concise answer.
        - The final answer must include at least one exact model name from the REST body models array.
        - Use only these tools: ollama_list_models, done.

        Tool call format:
        [tool_call]
        {"tool":"ollama_list_models","parameters":{}}
        [/tool_call]

        Final answer format:
        [tool_call]
        {"tool":"done","parameters":{"summary":"Local Ollama models include: <names from REST result>."}}
        [/tool_call]
`, model, model)
}

func monitoredQwenREST(addr string) string {
	return fmt.Sprintf(`rest:
  version: v1
  limits:
    local_monitor:
      timeout: 2s
      read_timeout: 2s
      max_request_bytes: 4096
      max_response_bytes: 1048576
      network:
        hosts: [127.0.0.1, localhost]
  servers:
    monitor:
      address: %s
      limits_ref: local_monitor
      queue:
        name: monitor
        capacity: 8
        overflow: reject
        timeout: 30s
      shutdown:
        timeout: 2s
        drain_policy: drain
        stop_listeners: true
        unblock_await_signal: ServerStopped
      endpoints:
        current_state:
          method: GET
          path: /monitor/state
          binding: read_state
          monitor_view: current_state
        metrics:
          method: GET
          path: /monitor/metrics
          binding: read_state
          monitor_view: metrics
        control_exit:
          method: POST
          path: /monitor/control/exit
          binding: emit_signal
          signal: ExitRequested
          request:
            body_schema:
              type: object
              properties:
                reason:
                  type: string
          response:
            output:
              accepted: "true"
`, addr)
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
