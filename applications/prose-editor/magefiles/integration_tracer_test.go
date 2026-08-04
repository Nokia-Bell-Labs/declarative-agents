// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"strings"
	"testing"
)

func TestRelease001InterpreterEvidence(t *testing.T) {
	if err := (Integration{}).Tracer(); err != nil {
		t.Fatal(err)
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
		"request_from: $from(capture_manifest).output",
		"request_from: $from(structure_manifest).output",
		"binary: prose-editor-tracer-boundary",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("shipped orchestrator declaration missing %q", required)
		}
	}
}
