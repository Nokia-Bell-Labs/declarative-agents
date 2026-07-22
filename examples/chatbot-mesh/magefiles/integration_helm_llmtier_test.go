// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestOllamaTierRendersWithDefaults locks srd003 R6: with the defaults
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
		"name: t-chatbot-mesh-ollama",         // StatefulSet + Service
		"name: t-chatbot-mesh-ollama-preload", // preload Job
		"name: wait-for-llm-models",           // agent readiness init
		"ollama pull",                         // the preload pulls models
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
	// The preload runs the ollama image, which ships neither wget nor curl, so its
	// reachability probe must be the ollama CLI, never an HTTP client absent from
	// the image (a wget probe hung the preload forever).
	if !strings.Contains(render, "until ollama list") {
		t.Error("preload reachability probe must use `ollama list` (the ollama image has no wget/curl)")
	}
	if strings.Contains(render, "wget -qO- \"$OLLAMA_HOST") {
		t.Error("preload must not probe reachability with wget: the ollama image ships no wget, so the loop never exits")
	}
	// The default chart has one chatbot and one RAG workload. Both must carry the
	// positive preload-completion gate. Removing the gate from either workload,
	// or rendering it only when Ollama is disabled, changes this exact count.
	if got := strings.Count(render, "- name: wait-for-llm-models"); got != 2 {
		t.Errorf("wait-for-llm-models init containers = %d, want 2 (chatbot + RAG)", got)
	}
	if got := strings.Count(render, "until wget -qO- \"$url\""); got != 2 {
		t.Errorf("positive model-presence gates = %d, want 2", got)
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
	// External-tier rendering removes chart-owned Ollama resources and their
	// preload gates, but retains both agent workloads.
	for _, workload := range []string{
		"name: t-chatbot-mesh-chatbot",
		"name: t-chatbot-mesh-rag0",
	} {
		if !strings.Contains(render, workload) {
			t.Errorf("external-tier render removed agent workload %q", workload)
		}
	}
}

func TestLLMPreloadReadinessTransitionSequence(t *testing.T) {
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		switch {
		case strings.Contains(call, "get job/llm-chatbot-mesh-ollama-preload"):
			return []byte(`{"spec":{"suspend":true},"status":{"succeeded":0}}`), nil
		case strings.Contains(call, "get deployment"):
			return []byte(`{"items":[
				{"metadata":{"name":"llm-chatbot-mesh-chatbot","labels":{"app.kubernetes.io/component":"chatbot"}},"spec":{"replicas":1},"status":{"readyReplicas":0}},
				{"metadata":{"name":"llm-chatbot-mesh-rag0","labels":{"app.kubernetes.io/component":"rag-server"}},"spec":{"replicas":1},"status":{"readyReplicas":0}}
			]}`), nil
		default:
			return []byte("ok"), nil
		}
	}

	workloads, err := beginLLMPreloadTransition(run)
	if err != nil {
		t.Fatal(err)
	}
	if err := finishLLMPreloadTransition(run, workloads); err != nil {
		t.Fatal(err)
	}
	if got, want := workloads, []string{"llm-chatbot-mesh-chatbot", "llm-chatbot-mesh-rag0"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("workloads = %v, want %v", got, want)
	}
	sequence := strings.Join(calls, "\n")
	for _, ordered := range []string{
		"get job/llm-chatbot-mesh-ollama-preload",
		"get deployment",
		"patch job/llm-chatbot-mesh-ollama-preload",
		"wait --for=condition=complete",
		"rollout status deployment/llm-chatbot-mesh-chatbot",
		"rollout status deployment/llm-chatbot-mesh-rag0",
	} {
		index := strings.Index(sequence, ordered)
		if index < 0 {
			t.Fatalf("command sequence missing %q:\n%s", ordered, sequence)
		}
		sequence = sequence[index+len(ordered):]
	}
}

func TestLLMPreloadReadinessTransitionDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		job      string
		workload string
		failOn   string
		wantErr  string
	}{
		{
			name: "preload not suspended", job: `{"spec":{"suspend":false},"status":{}}`,
			wantErr: "requires suspended incomplete Job",
		},
		{
			name: "agent already ready", job: `{"spec":{"suspend":true},"status":{}}`,
			workload: `{"items":[
				{"metadata":{"name":"chatbot","labels":{"app.kubernetes.io/component":"chatbot"}},"spec":{"replicas":1},"status":{"readyReplicas":1}},
				{"metadata":{"name":"rag0","labels":{"app.kubernetes.io/component":"rag-server"}},"spec":{"replicas":1},"status":{}}
			]}`,
			wantErr: "became ready before preload",
		},
		{
			name: "preload wait failure", job: `{"spec":{"suspend":true},"status":{}}`,
			workload: unreadyLLMWorkloadsJSON(), failOn: "wait --for=condition=complete",
			wantErr: "preload Job did not complete",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			run := func(name string, args ...string) ([]byte, error) {
				call := name + " " + strings.Join(args, " ")
				if tc.failOn != "" && strings.Contains(call, tc.failOn) {
					return []byte("controlled kubectl diagnostic"), errors.New("controlled failure")
				}
				if strings.Contains(call, "get job/") {
					return []byte(tc.job), nil
				}
				if strings.Contains(call, "get deployment") {
					return []byte(tc.workload), nil
				}
				return []byte("ok"), nil
			}
			_, err := beginLLMPreloadTransition(run)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, tc.wantErr)
			}
			if tc.failOn != "" && !strings.Contains(err.Error(), "controlled kubectl diagnostic") {
				t.Fatalf("error missing live command diagnostic: %v", err)
			}
		})
	}
}

func TestHelmLLMTierInstallExposesTransition(t *testing.T) {
	var command string
	err := helmInstallLLMWithRunner("/chart", func(name string, args ...string) ([]byte, error) {
		command = name + " " + strings.Join(args, " ")
		return nil, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(command, " --wait") {
		t.Fatalf("helm install hides readiness transition behind --wait: %s", command)
	}
	if !strings.Contains(command, "--set ollama.preload.suspend=true") {
		t.Fatalf("helm install does not suspend preload for observation: %s", command)
	}
}

func unreadyLLMWorkloadsJSON() string {
	return `{"items":[
		{"metadata":{"name":"chatbot","labels":{"app.kubernetes.io/component":"chatbot"}},"spec":{"replicas":1},"status":{}},
		{"metadata":{"name":"rag0","labels":{"app.kubernetes.io/component":"rag-server"}},"spec":{"replicas":1},"status":{}}
	]}`
}
