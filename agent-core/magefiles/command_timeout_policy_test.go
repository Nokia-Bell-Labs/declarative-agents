// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"regexp"
	"testing"
	"time"
)

func TestAgentCoreMachineTimeoutEnvelopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		path      string
		want      time.Duration
		authority time.Duration
	}{
		{name: "OTLP replay relay", path: "../testdata/integration/profiles/otlp-replay/machine.yaml", want: time.Minute, authority: 10 * time.Second},
		{name: "Ollama REST model", path: "../testdata/integration/profiles/ollama-rest/machine.yaml", want: 3 * time.Minute, authority: 2 * time.Minute},
		{name: "Ollama monitor model", path: "../testdata/integration/profiles/ollama-monitor/machine.yaml", want: 3 * time.Minute, authority: 2 * time.Minute},
		{name: "monitor control await", path: "../testdata/integration/profiles/monitor/machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "lifecycle operation", path: "../testdata/integration/profiles/lifecycle/machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "control await", path: "../testdata/integration/profiles/control/machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "specification audit", path: "../testdata/integration/profiles/audit/machine.yaml", want: 15 * time.Minute, authority: 10 * time.Minute},
		{name: "test evidence audit", path: "../testdata/integration/profiles/audit/audit-machine.yaml", want: 15 * time.Minute, authority: 10 * time.Minute},
		{name: "monitored Qwen model", path: "fixtures/uc008/machine.yaml", want: 3 * time.Minute, authority: 2 * time.Minute},
		{name: "Ollama REST mirror", path: "../internal/tools/rest/testdata/ollama_profile/ollama-machine.yaml", want: 3 * time.Minute, authority: 2 * time.Minute},
		{name: "spec corpus sample", path: "../pkg/spec/testdata/valid/agents/test-agent/machine.yaml", want: 30 * time.Second, authority: 10 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := readMachineCommandTimeout(t, test.path)
			if got != test.want {
				t.Errorf("%s command_timeout = %s, want %s", test.path, got, test.want)
			}
			if got <= test.authority {
				t.Errorf("%s command_timeout = %s, must exceed governing operation %s", test.path, got, test.authority)
			}
		})
	}
}

func readMachineCommandTimeout(t *testing.T, path string) time.Duration {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`command_timeout:\s*([0-9]+(?:ns|us|µs|ms|s|m|h))`).FindSubmatch(data)
	if len(match) != 2 {
		t.Fatalf("%s has no command_timeout", path)
	}
	timeout, err := time.ParseDuration(string(match[1]))
	if err != nil {
		t.Fatalf("%s command_timeout: %v", path, err)
	}
	return timeout
}
