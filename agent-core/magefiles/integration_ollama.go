// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	ollamaLLMRel         = "rest/ollama-llm.yaml"
	ollamaPrompt         = "List the local Ollama models available on this machine."
	ollamaListModelsTool = "ollama_list_models"
	// ollamaTestModel is the default chat model for the Ollama uc005 tracer.
	// It is separate from qwen35b, which the generator/evaluator gates require
	// for the qwen35b-specific generator profiles.
	ollamaTestModel = "ornith:9b"
)

// Uc005 runs rel03.0-uc005: Qwen lists Ollama models through an OpenAPI REST tool.
func (Integration) Uc005() error {
	model := configuredOllamaModel()
	names, err := requireOllamaModels(model)
	if err != nil {
		return skipUC("uc005", err.Error())
	}
	binary, err := buildFreshAgentFor("uc005")
	if err != nil {
		return err
	}
	run, cleanup, err := prepareOllamaIntegrationRun(model)
	if err != nil {
		return fmt.Errorf("uc005: prepare profile: %w", err)
	}
	defer cleanup()
	if err := runAgentCapture(binary, run.args()); err != nil {
		return classifyOllamaRunFailure(run.tracePath, err)
	}
	if err := assertOllamaTrace(run.tracePath, names); err != nil {
		return err
	}
	fmt.Printf("uc005: PASS — %s used %s and answered with live Ollama models\n", model, ollamaListModelsTool)
	return nil
}

type ollamaIntegrationRun struct {
	profilePath string
	tracePath   string
}

func (r ollamaIntegrationRun) args() []string {
	return []string{
		"--profile", r.profilePath,
		"--verbose-trace",
		"--otel-log-file", r.tracePath,
	}
}

func configuredOllamaModel() string {
	return ollamaTestModel
}

func requireOllamaModels(model string) ([]string, error) {
	if err := requireOllama(); err != nil {
		return nil, fmt.Errorf("missing Ollama: %w", err)
	}
	names, err := fetchOllamaModelNames()
	if err != nil {
		return nil, fmt.Errorf("REST preflight failed: %w", err)
	}
	if !containsString(names, model) {
		return nil, fmt.Errorf("missing Qwen model %q; available: %s", model, strings.Join(names, ", "))
	}
	return names, nil
}

func fetchOllamaModelNames() ([]string, error) {
	resp, err := http.Get("http://127.0.0.1:11434/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("/api/tags returned status %d", resp.StatusCode)
	}
	var result struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return modelNames(result.Models), nil
}

func modelNames(models []struct {
	Name string `json:"name"`
}) []string {
	names := make([]string, 0, len(models))
	for _, model := range models {
		if model.Name != "" {
			names = append(names, model.Name)
		}
	}
	return names
}

func prepareOllamaIntegrationRun(model string) (ollamaIntegrationRun, func(), error) {
	rootDir, err := os.Getwd()
	if err != nil {
		return ollamaIntegrationRun{}, nil, err
	}
	profileRoot, err := resolveAgentProfilesRoot(rootDir)
	if err != nil {
		return ollamaIntegrationRun{}, nil, err
	}
	tmpDir, err := os.MkdirTemp("", "uc005-ollama-*")
	if err != nil {
		return ollamaIntegrationRun{}, nil, err
	}
	cleanup := func() { os.RemoveAll(tmpDir) }
	run := ollamaIntegrationRun{
		profilePath: filepath.Join(tmpDir, "profile.yaml"),
		tracePath:   filepath.Join(tmpDir, "trace.json"),
	}
	err = writeOllamaTempProfile(rootDir, profileRoot, tmpDir, model, run.profilePath)
	return run, cleanup, err
}

func writeOllamaTempProfile(rootDir, profileRoot, tmpDir, model, profilePath string) error {
	llmPath := filepath.Join(tmpDir, "ollama-llm.yaml")
	if err := writeOllamaLLMOverride(profileRoot, llmPath, model); err != nil {
		return err
	}
	profile := fmt.Sprintf(`name: ollama-rest
machine: %s
tools:
  - %s
tool_declarations:
  - %s
  - %s
  - %s
rest_definitions:
  - %s
`, profileAbs(profileRoot, "rest/ollama-machine.yaml"), profileAbs(profileRoot, "rest/ollama-tools.yaml"),
		abs(rootDir, "tools/builtin/llm/all.yaml"), llmPath,
		profileAbs(profileRoot, "rest/ollama-declarations.yaml"), profileAbs(profileRoot, "rest/ollama-rest.yaml"))
	return os.WriteFile(profilePath, []byte(profile), 0o644)
}

func writeOllamaLLMOverride(profileRoot, outPath, model string) error {
	data, err := os.ReadFile(profileAbs(profileRoot, ollamaLLMRel))
	if err != nil {
		return err
	}
	replacement := fmt.Sprintf("model: %q", model)
	updated := strings.Replace(string(data), `model: "qwen3.6:35b-mlx"`, replacement, 1)
	return os.WriteFile(outPath, []byte(updated), 0o644)
}

func abs(rootDir, rel string) string {
	return filepath.Join(rootDir, rel)
}

func profileAbs(profileRoot, rel string) string {
	return agentProfileAsset(profileRoot, rel)
}

func runAgentCapture(binary string, args []string) error {
	cmd := exec.Command(binary, args...)
	fmt.Printf("running: %s %s\n", binary, strings.Join(args, " "))
	out, err := runCommandCapture(cmd)
	if err != nil {
		return fmt.Errorf("%w\n%s", err, out.String())
	}
	return nil
}

func classifyOllamaRunFailure(tracePath string, runErr error) error {
	trace, _ := os.ReadFile(tracePath)
	switch {
	case bytes.Contains(trace, []byte(ollamaListModelsTool)):
		return fmt.Errorf("REST tool failure or answer mismatch: %w", runErr)
	case bytes.Contains(trace, []byte("ollama chat request failed")):
		return fmt.Errorf("missing Ollama during model call: %w", runErr)
	default:
		return fmt.Errorf("LLM answer mismatch before REST tool use: %w", runErr)
	}
}

func assertOllamaTrace(tracePath string, modelNames []string) error {
	data, err := os.ReadFile(tracePath)
	if err != nil {
		return fmt.Errorf("uc005: read trace: %w", err)
	}
	if !bytes.Contains(data, []byte(ollamaListModelsTool)) || !bytes.Contains(data, []byte("RESTResponded")) {
		return fmt.Errorf("uc005: trace does not show %s usage", ollamaListModelsTool)
	}
	summary, err := traceDoneSummary(data)
	if err != nil {
		return err
	}
	if !containsAnyModel(summary, modelNames) {
		return fmt.Errorf("uc005: final answer does not include any /api/tags model name")
	}
	if err := requireSummarySubset(summary, modelNames); err != nil {
		return err
	}
	return nil
}

type traceSpan struct {
	Attributes []traceAttr `json:"Attributes"`
}

type traceAttr struct {
	Key   string         `json:"Key"`
	Value traceAttrValue `json:"Value"`
}

type traceAttrValue struct {
	Type  string      `json:"Type"`
	Value interface{} `json:"Value"`
}

func traceDoneSummary(trace []byte) (string, error) {
	for _, line := range strings.Split(string(trace), "\n") {
		var span traceSpan
		if err := json.Unmarshal([]byte(line), &span); err != nil {
			continue
		}
		for _, attr := range span.Attributes {
			if attr.Key == "done.summary" {
				if summary, ok := attr.Value.Value.(string); ok {
					return summary, nil
				}
			}
		}
	}
	return "", fmt.Errorf("uc005: trace does not show final done answer")
}

func containsAnyModel(summary string, modelNames []string) bool {
	for _, name := range modelNames {
		if name != "" && strings.Contains(summary, name) {
			return true
		}
	}
	return false
}

func requireSummarySubset(summary string, modelNames []string) error {
	listed := listedSummaryModels(summary)
	if len(listed) == 0 {
		return fmt.Errorf("uc005: trace does not show final done answer")
	}
	allowed := map[string]bool{}
	for _, name := range modelNames {
		allowed[name] = true
	}
	for _, name := range listed {
		if !allowed[name] {
			return fmt.Errorf("uc005: final answer named absent model %q", name)
		}
	}
	return nil
}

func listedSummaryModels(summary string) []string {
	idx := strings.Index(summary, "include:")
	if idx < 0 {
		return nil
	}
	raw := strings.TrimSpace(strings.TrimSuffix(summary[idx+len("include:"):], "."))
	parts := strings.Split(raw, ",")
	models := make([]string, 0, len(parts))
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name != "" {
			models = append(models, name)
		}
	}
	return models
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
