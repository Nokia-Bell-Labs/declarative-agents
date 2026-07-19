// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestOllamaTierRendersWithDefaults locks srd015 R6: with the defaults
// (ollama.enabled=true) the chart renders the in-cluster LLM tier -- an Ollama
// StatefulSet, its Service, the model-preload Job pulling every declared model,
// and the chatbot's wait-for-llm-models readiness init container.
func TestOllamaTierRendersWithDefaults(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	out, err := exec.Command("helm", "template", "t", chart).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)
	for _, want := range []string{
		"name: t-chatbot-mesh-ollama",       // StatefulSet + Service
		"name: t-chatbot-mesh-ollama-preload", // preload Job
		"name: wait-for-llm-models",          // chatbot readiness init
		"ollama pull",                        // the preload pulls models
	} {
		if !strings.Contains(render, want) {
			t.Errorf("default render missing %q (the LLM tier must render with defaults)", want)
		}
	}
	// The embedding base_url points at the in-cluster Ollama Service.
	if !strings.Contains(render, "base_url: http://t-chatbot-mesh-ollama:11434") {
		t.Error("default render does not point the embedding client at the in-cluster Ollama Service")
	}
	// Every declared model reaches the preload (named once in values).
	for _, model := range []string{"qwen3-embedding:8b", "qwen2.5:3b", "ornith:9b"} {
		if !strings.Contains(render, model) {
			t.Errorf("preload/config missing declared model %q", model)
		}
	}
}

// TestOllamaDisabledReproducesExternalEndpoint locks R6.1: with ollama.enabled=false
// and an endpoint override, no Ollama tier renders and the co-generated embedding
// client points at the operator-supplied service (the pre-tier external behavior),
// so the co-generation contract (R2) is unchanged.
func TestOllamaDisabledReproducesExternalEndpoint(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm not on PATH")
	}
	chart := findChartDir(t)
	out, err := exec.Command("helm", "template", "t", chart,
		"--set", "ollama.enabled=false",
		"--set", "llm.externalURL=http://ollama.example:11434",
	).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template: %v\n%s", err, out)
	}
	render := string(out)
	for _, absent := range []string{
		"app.kubernetes.io/component: ollama",
		"ollama-preload",
		"wait-for-llm-models",
	} {
		if strings.Contains(render, absent) {
			t.Errorf("disabled render still contains %q (no LLM tier must render when disabled)", absent)
		}
	}
	if !strings.Contains(render, "base_url: http://ollama.example:11434") {
		t.Error("disabled render does not point the embedding client at the external endpoint override")
	}
}
