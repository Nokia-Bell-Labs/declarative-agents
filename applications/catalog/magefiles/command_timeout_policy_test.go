// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"os"
	"regexp"
	"testing"
	"time"
)

const (
	maxEvaluatorPointDeadline   = 15 * time.Minute
	criticPointProcessingMargin = time.Minute
	maxEvaluatorSessionDeadline = 24 * time.Hour
	benchSessionCleanupMargin   = time.Hour
)

func TestCatalogShortMachineTimeoutEnvelopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		path      string
		want      time.Duration
		authority time.Duration
	}{
		{name: "applier control await", path: "../agents/applier/machine.yaml", want: 11 * time.Minute, authority: 10 * time.Minute},
		{name: "applier apply request", path: "../agents/applier/apply-machine.yaml", want: 3 * time.Minute, authority: 130 * time.Second},
		{name: "applier rollout request", path: "../agents/applier/rollout-machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "collector relay", path: "../agents/collector/machine.yaml", want: time.Minute, authority: 10 * time.Second},
		{name: "collector query", path: "../agents/collector/query-machine.yaml", want: time.Minute, authority: 10 * time.Second},
		{name: "curator control await", path: "../agents/knowledge-manager/documentation-curator/machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "curator request", path: "../agents/knowledge-manager/documentation-curator/request-machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "corpus reader model", path: "../agents/knowledge-manager/corpus-reader/machine.yaml", want: 3 * time.Minute, authority: 2 * time.Minute},
		{name: "corpus ingest model", path: "../agents/knowledge-manager/corpus-ingest/machine.yaml", want: 6 * time.Minute, authority: 5 * time.Minute},
		{name: "lifecycle exit REST", path: "../agents/lifecycle-exit/machine.yaml", want: time.Minute, authority: 10 * time.Second},
		{name: "mock control await", path: "../agents/mock/machine.yaml", want: 11 * time.Minute, authority: 10 * time.Minute},
		{name: "runtime state control await", path: "../agents/runtime-state-reader/machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "specification critic", path: "../agents/specification-critic/machine.yaml", want: 15 * time.Minute, authority: 10 * time.Minute},
		{name: "test evidence critic", path: "../agents/specification-critic/audit-machine.yaml", want: 15 * time.Minute, authority: 10 * time.Minute},
		{name: "control conformance", path: "../testdata/conformance/control/machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "approval conformance", path: "../testdata/conformance/lifecycle/approval/machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "history conformance", path: "../testdata/conformance/lifecycle/history.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "rollback conformance", path: "../testdata/conformance/lifecycle/rollback.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "REST conformance", path: "../testdata/conformance/rest/machine.yaml", want: time.Minute, authority: 2 * time.Second},
		{name: "Ollama conformance model", path: "../testdata/conformance/rest/ollama-machine.yaml", want: 3 * time.Minute, authority: 2 * time.Minute},
		{name: "rig subject control await", path: "../testdata/conformance/rig-subject/machine.yaml", want: 11 * time.Minute, authority: 10 * time.Minute},
		{name: "rig subject request", path: "../testdata/conformance/rig-subject/request-machine.yaml", want: time.Minute, authority: 20 * time.Second},
		{name: "rig broken request", path: "../testdata/conformance/rig-subject/tests/broken/machine.yaml", want: time.Minute, authority: 10 * time.Second},
		{name: "rig dependency failure request", path: "../testdata/conformance/rig-subject/tests/dep-failure/machine.yaml", want: time.Minute, authority: 10 * time.Second},
		{name: "rig happy request", path: "../testdata/conformance/rig-subject/tests/happy-path/machine.yaml", want: time.Minute, authority: 10 * time.Second},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := readCatalogMachineCommandTimeout(t, test.path)
			if got != test.want {
				t.Errorf("%s command_timeout = %s, want %s", test.path, got, test.want)
			}
			if got <= test.authority {
				t.Errorf("%s command_timeout = %s, must exceed governing operation %s", test.path, got, test.authority)
			}
		})
	}
}

func TestCatalogLongMachineTimeoutEnvelopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		path      string
		want      time.Duration
		authority time.Duration
	}{
		{name: "bench request", path: "../agents/bench/request-machine.yaml", want: time.Minute, authority: 30 * time.Second},
		{name: "critic workspace model", path: "../agents/critic/machine-workspace.yaml", want: 16 * time.Minute, authority: 15 * time.Minute},
		{name: "planner session model", path: "../agents/planner/machine.yaml", want: 16 * time.Minute, authority: 15 * time.Minute},
		{name: "planner plan-only model", path: "../agents/planner/machine-plan-only.yaml", want: 16 * time.Minute, authority: 15 * time.Minute},
		{name: "planner passthrough child", path: "../agents/planner/machine-passthrough.yaml", want: 16 * time.Minute, authority: 15 * time.Minute},
		{name: "executor model", path: "../agents/executor/machine.yaml", want: 16 * time.Minute, authority: 15 * time.Minute},
		{name: "deepseek executor model", path: "../agents/executor/deepseek-coding-agent.yaml", want: 16 * time.Minute, authority: 15 * time.Minute},
		{name: "scenario validator", path: "../agents/scenario-critic/machine.yaml", want: 6 * time.Minute, authority: 5 * time.Minute},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := readCatalogMachineCommandTimeout(t, test.path)
			if got != test.want {
				t.Errorf("%s command_timeout = %s, want %s", test.path, got, test.want)
			}
			if got <= test.authority {
				t.Errorf("%s command_timeout = %s, must exceed governing operation %s", test.path, got, test.authority)
			}
		})
	}
}

func TestCatalogEvaluatorTimeoutEnvelopes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		machinePath    string
		action         string
		authority      time.Duration
		requiredMargin time.Duration
	}{
		{
			name: "critic point child runtime", machinePath: "../agents/critic/point.yaml",
			action: "run_agent", authority: maxEvaluatorPointDeadline, requiredMargin: criticPointProcessingMargin,
		},
		{
			name: "critic session run_point", machinePath: "../agents/critic/machine.yaml",
			action: "run_point", authority: maxEvaluatorPointDeadline, requiredMargin: criticPointProcessingMargin,
		},
		{
			name: "bench evaluator child session", machinePath: "../agents/bench/machine.yaml",
			action: "launch_evaluator", authority: maxEvaluatorSessionDeadline, requiredMargin: benchSessionCleanupMargin,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := readCatalogMachineCommandTimeout(t, test.machinePath)
			if minimum := test.authority + test.requiredMargin; got < minimum {
				t.Errorf("%s command_timeout = %s, must be at least %s (%s authority + %s margin)",
					test.machinePath, got, minimum, test.authority, test.requiredMargin)
			}
			requireCatalogMachineAction(t, test.machinePath, test.action)
		})
	}
}

func readCatalogMachineCommandTimeout(t *testing.T, path string) time.Duration {
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

func requireCatalogMachineAction(t *testing.T, path, action string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(`(?m)^\s*action:\s*` + regexp.QuoteMeta(action) + `\s*$`)
	if !pattern.Match(data) {
		t.Errorf("%s does not dispatch action %q", path, action)
	}
}
