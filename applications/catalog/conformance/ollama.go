// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

const (
	// LiveConformanceEnv is the explicit opt-in for tests that perform live model
	// inference. Model installation alone must never enable those tests.
	LiveConformanceEnv = "AGENT_PROFILES_LIVE_CONFORMANCE"
	// LiveConformanceTimeoutEnv optionally overrides the per-run live inference
	// timeout using Go duration syntax.
	LiveConformanceTimeoutEnv = "AGENT_PROFILES_LIVE_TIMEOUT"

	defaultLiveConformanceTimeout = 5 * time.Minute
)

// ollamaBaseURL is the default local Ollama endpoint the generator/planner LLM
// declarations point at (agents/*/llm/default.yaml provider_url).
const ollamaBaseURL = "http://localhost:11434"

// RequireLiveModel first enforces the explicit live-conformance opt-in, then
// checks that the exact model required by the test is available. It returns the
// configured timeout for the live request or agent run.
func RequireLiveModel(t *testing.T, model string) time.Duration {
	t.Helper()
	timeout, skip, err := liveModelGate(
		os.Getenv(LiveConformanceEnv),
		os.Getenv(LiveConformanceTimeoutEnv),
		model,
		func(model string) error {
			return probeOllama(&http.Client{Timeout: 3 * time.Second}, ollamaBaseURL, model)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if skip != "" {
		t.Skip(skip)
	}
	return timeout
}

func liveModelGate(optIn, timeoutValue, model string, probe func(string) error) (time.Duration, string, error) {
	if strings.TrimSpace(optIn) != "1" {
		return 0, fmt.Sprintf(
			"live model conformance disabled; run `mage liveConformance` or set %s=1",
			LiveConformanceEnv,
		), nil
	}

	timeout := defaultLiveConformanceTimeout
	if value := strings.TrimSpace(timeoutValue); value != "" {
		parsed, err := time.ParseDuration(value)
		if err != nil || parsed <= 0 {
			return 0, "", fmt.Errorf("%s=%q must be a positive Go duration (for example 5m)", LiveConformanceTimeoutEnv, timeoutValue)
		}
		timeout = parsed
	}
	if err := probe(model); err != nil {
		return 0, fmt.Sprintf(
			"live model conformance enabled but dependency unavailable: %v; install/start the exact dependency and rerun `mage liveConformance`",
			err,
		), nil
	}
	return timeout, "", nil
}

func probeOllama(client *http.Client, baseURL, model string) error {
	resp, err := client.Get(baseURL + "/api/tags")
	if err != nil {
		return fmt.Errorf("cannot reach Ollama at %s: %w", baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the Ollama tags endpoint returned %d", resp.StatusCode)
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return fmt.Errorf("decode Ollama tags: %w", err)
	}
	for _, m := range payload.Models {
		if m.Name == model {
			return nil
		}
	}
	return fmt.Errorf("the Ollama model %q is not pulled", model)
}
