// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"testing"
	"time"
)

const (
	defaultLiveConformanceTimeout = 5 * time.Minute
	liveConformanceFlag           = "live"
	liveConformanceTimeoutFlag    = "live-timeout"
)

var (
	liveConformance = flag.Bool(liveConformanceFlag, false,
		"enable conformance tests that perform live model inference")
	liveConformanceTimeout = flag.Duration(liveConformanceTimeoutFlag, defaultLiveConformanceTimeout,
		"per-run timeout for live model inference")
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
		*liveConformance,
		*liveConformanceTimeout,
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

func liveModelGate(optIn bool, timeout time.Duration, model string, probe func(string) error) (time.Duration, string, error) {
	if !optIn {
		return 0, "live model conformance disabled; run `mage liveConformance`", nil
	}

	if timeout <= 0 {
		return 0, "", fmt.Errorf("-%s must be a positive Go duration (for example 5m)", liveConformanceTimeoutFlag)
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
