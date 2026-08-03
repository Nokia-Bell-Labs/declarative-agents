// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
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

// ollamaBaseURL is the fallback used by declarations whose provider_url is
// ${OLLAMA_URL:-http://localhost:11434}.
const ollamaBaseURL = "http://localhost:11434"

// RequireLiveModel first enforces the explicit live-conformance opt-in, then
// checks the exact provider URL and model required by the test. Passing the URL
// at the call site keeps the gate aligned with the profile under test.
func RequireLiveModel(t *testing.T, baseURL, model string) time.Duration {
	t.Helper()
	timeout, skip, err := liveModelGate(
		*liveConformance,
		*liveConformanceTimeout,
		model,
		func(model string) error {
			return probeOllama(&http.Client{Timeout: 3 * time.Second}, baseURL, model)
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

func ollamaURLFromEnvironment() string {
	if configured := strings.TrimSpace(os.Getenv("OLLAMA_URL")); configured != "" {
		return strings.TrimRight(configured, "/")
	}
	return ollamaBaseURL
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
	baseURL = strings.TrimRight(baseURL, "/")
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
		if ollamaModelMatches(m.Name, model) {
			return nil
		}
	}
	return fmt.Errorf("the Ollama model %q is not pulled", model)
}

func ollamaModelMatches(available, required string) bool {
	if strings.EqualFold(available, required) {
		return true
	}
	if !strings.Contains(required, ":") {
		return strings.EqualFold(available, required+":latest")
	}
	return false
}
