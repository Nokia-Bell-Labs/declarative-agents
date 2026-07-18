// Copyright (c) 2026 Nokia. All rights reserved.

package conformance

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// The generator and planner default machines dispatch invoke_llm, which pings
// Ollama at tool registration and calls the model during the run. Those
// families are therefore gated on a reachable Ollama serving the configured
// model, the same way the whole suite is gated on the sibling agent-core checkout being present: with no
// model the profile cannot even start.

// ollamaBaseURL is the default local Ollama endpoint the generator/planner LLM
// declarations point at (agents/*/llm/default.yaml provider_url).
const ollamaBaseURL = "http://localhost:11434"

// RequireOllama skips the test unless a local Ollama server is reachable and
// serving the named model. It keeps model-dependent conformance runs opt-in on
// machines without a model, matching agent-core's Ollama integration gating.
func RequireOllama(t *testing.T, model string) {
	t.Helper()
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(ollamaBaseURL + "/api/tags")
	if err != nil {
		t.Skipf("Ollama not reachable at %s; skipping model-gated conformance: %v", ollamaBaseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Skipf("Ollama tags endpoint returned %d; skipping model-gated conformance", resp.StatusCode)
	}
	var payload struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Skipf("decode Ollama tags: %v; skipping model-gated conformance", err)
	}
	for _, m := range payload.Models {
		if m.Name == model {
			return
		}
	}
	t.Skipf("Ollama model %q not pulled; skipping model-gated conformance", model)
}
