// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const observerPollIntervalPattern = `^(?:(?:[1-9]|[1-9][0-9]|[1-5][0-9]{2})s|(?:[1-9]|10)m)$`

func TestChatbotMeshMachineTimeoutEnvelopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		path      string
		want      time.Duration
		authority time.Duration
	}{
		{name: "chatbot control await", path: "../agents/chatbot/machine.yaml", want: 11 * time.Minute, authority: 10 * time.Minute},
		{name: "chatbot request model", path: "../agents/chatbot/request-machine.yaml", want: 3 * time.Minute, authority: 2 * time.Minute},
		{name: "chatbot single-turn model", path: "../agents/chatbot/tests/single-turn/machine.yaml", want: 2 * time.Minute, authority: time.Minute},
		{name: "chatbot degraded model", path: "../agents/chatbot/tests/degraded-rag/machine.yaml", want: 2 * time.Minute, authority: time.Minute},
		{name: "creator control await", path: "../agents/creator/machine.yaml", want: 11 * time.Minute, authority: 10 * time.Minute},
		{name: "creator ingest request", path: "../agents/creator/request-machine.yaml", want: 11 * time.Minute, authority: 10 * time.Minute},
		{name: "observer bounded poll", path: "../agents/observer/machine.yaml", want: 11 * time.Minute, authority: 10 * time.Minute},
		{name: "orchestrator control await", path: "../agents/provisioning-workflow-orchestrator/machine.yaml", want: 11 * time.Minute, authority: 10 * time.Minute},
		{name: "orchestrator request", path: "../agents/provisioning-workflow-orchestrator/request-machine.yaml", want: 3 * time.Minute, authority: 130 * time.Second},
		{name: "orchestrator state request", path: "../agents/provisioning-workflow-orchestrator/state-machine.yaml", want: time.Minute, authority: 20 * time.Second},
		{name: "orchestrator rollout request", path: "../agents/provisioning-workflow-orchestrator/rollout-machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "RAG control await", path: "../agents/rag-server/machine.yaml", want: 11 * time.Minute, authority: 10 * time.Minute},
		{name: "RAG query request", path: "../agents/rag-server/request-machine.yaml", want: time.Minute, authority: 10 * time.Second},
		{name: "RAG query fixture", path: "../agents/rag-server/tests/query/machine.yaml", want: time.Minute, authority: 10 * time.Second},
		{name: "applier state request", path: "../agents/applier/state-machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "scenario rig validator", path: "../testdata/rig/machine.yaml", want: 6 * time.Minute, authority: 5 * time.Minute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := readMeshMachineCommandTimeout(t, test.path)
			if got != test.want {
				t.Errorf("%s command_timeout = %s, want %s", test.path, got, test.want)
			}
			if got <= test.authority {
				t.Errorf("%s command_timeout = %s, must exceed governing operation %s", test.path, got, test.authority)
			}
		})
	}
}

func TestObserverPollIntervalIsBoundedByMachineEnvelope(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../helm/values.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]interface{})
	observer := properties["observer"].(map[string]interface{})
	observerProperties := observer["properties"].(map[string]interface{})
	pollInterval := observerProperties["pollInterval"].(map[string]interface{})
	if pollInterval["pattern"] != observerPollIntervalPattern {
		t.Errorf("observer pollInterval pattern = %q, want %q", pollInterval["pattern"], observerPollIntervalPattern)
	}

	declaration, err := os.ReadFile("../agents/observer/declarations.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(declaration), "timeout: ${OBSERVER_POLL_INTERVAL:-10s}") {
		t.Error("observer await no longer reads the Helm-bounded poll interval")
	}
}

func readMeshMachineCommandTimeout(t *testing.T, path string) time.Duration {
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
