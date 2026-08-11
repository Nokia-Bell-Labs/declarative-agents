// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"strings"
	"testing"
)

func TestConcurrentRelease001InterpreterEvidence(t *testing.T) {
	const gates = 2
	start := make(chan struct{})
	results := make(chan error, gates)
	for gate := 0; gate < gates; gate++ {
		go func() {
			<-start
			results <- (Integration{}).Tracer()
		}()
	}
	close(start)
	for gate := 0; gate < gates; gate++ {
		if err := <-results; err != nil {
			t.Errorf("concurrent tracer gate %d: %v", gate+1, err)
		}
	}
}

func TestIntegrationTracerExecutesShippedMachines(t *testing.T) {
	data, err := os.ReadFile("integration_tracer.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{
		`"--profile", runtime.orchestrator`,
		`"--child-agent-binary", runtime.agent`,
		`"self_invoke.profile"`,
		`"specialist-editor/profile.yaml"`,
		`"voice-critic/profile.yaml"`,
		`"terminal state: succeeded"`,
		`"PROSE_TRACER_MODEL_PORT="+modelPort`,
		`"PROSE_TRACER_RAG_PORT="+ragPort`,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("interpreter tracer missing execution proof %q", required)
		}
	}
	for _, forbidden := range []string{
		"func runTracerScenario(",
		"func tracerStructure(",
		"func tracerCritique(",
		"manifest.Phase",
		`OLLAMA_URL=http://127.0.0.1:18086`,
		`STRUCTURE_RAG_URL=http://127.0.0.1:18085`,
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("interpreter tracer still duplicates sequencing with %q", forbidden)
		}
	}
}

func TestShippedOrchestratorOwnsChildRouting(t *testing.T) {
	data, err := os.ReadFile("../agents/workflow-orchestrator/declarations.yaml")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, required := range []string{
		"init: self_invoke",
		"profile: ../specialist-editor/profile.yaml",
		"profile: ../voice-critic/profile.yaml",
		"request_from: $from(structure_request_file).$",
		"request_from: $from(critic_request_file).$",
		"request_from: $from(structure_retry_request_file).$",
		"request_from: $from(critic_retry_request_file).$",
		"binary: prose-editor-tracer-boundary",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("shipped orchestrator declaration missing %q", required)
		}
	}
}
