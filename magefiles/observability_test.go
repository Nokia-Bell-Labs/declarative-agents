// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestObservabilityUpReusesHealthyStack(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return nil }
	runObservabilityCommand = func(string, ...string) error {
		t.Fatal("healthy stack must not run docker")
		return nil
	}

	if err := (Observability{}).Up(); err != nil {
		t.Fatal(err)
	}
}

func TestObservabilityUpChecksPortsAndWaitsForHealth(t *testing.T) {
	restoreObservabilityHooks(t)
	healthCalls := 0
	checkObservability = func() error {
		healthCalls++
		if healthCalls == 1 {
			return errors.New("not started")
		}
		return nil
	}
	observabilityOutput = func(string, ...string) (string, error) { return "", nil }
	var checked []string
	checkObservabilityPort = func(name, port string) error {
		checked = append(checked, name+":"+port)
		return nil
	}
	var command []string
	runObservabilityCommand = func(name string, args ...string) error {
		command = append([]string{name}, args...)
		return nil
	}

	if err := (Observability{}).Up(); err != nil {
		t.Fatal(err)
	}
	if len(checked) != 5 { // OTLP gRPC, OTLP HTTP, Collector health, Collector query, Prometheus query
		t.Fatalf("checked ports = %v", checked)
	}
	want := []string{"docker", "compose", "-f", observabilityComposeFile, "up", "-d", "--wait"}
	if !reflect.DeepEqual(command, want) {
		t.Fatalf("command = %v, want %v", command, want)
	}
}

func TestObservabilityUpReportsCollisionBeforeCompose(t *testing.T) {
	restoreObservabilityHooks(t)
	checkObservability = func() error { return errors.New("not started") }
	observabilityOutput = func(string, ...string) (string, error) { return "", nil }
	checkObservabilityPort = func(name, port string) error {
		return errors.New("port owner")
	}
	runObservabilityCommand = func(string, ...string) error {
		t.Fatal("compose must not run after collision")
		return nil
	}

	if err := (Observability{}).Up(); err == nil || !strings.Contains(err.Error(), "port owner") {
		t.Fatalf("error = %v", err)
	}
}

func TestObservabilityLifecycleCommandsPreserveOrDeleteVolumes(t *testing.T) {
	restoreObservabilityHooks(t)
	var commands [][]string
	runObservabilityCommand = func(name string, args ...string) error {
		commands = append(commands, append([]string{name}, args...))
		return nil
	}
	checkObservability = func() error { return nil }

	if err := (Observability{}).Status(); err != nil {
		t.Fatal(err)
	}
	if err := (Observability{}).Down(); err != nil {
		t.Fatal(err)
	}
	if err := (Observability{}).Reset(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(commands[1], " "); strings.Contains(got, " -v") {
		t.Fatalf("down deletes volumes: %s", got)
	}
	if got := strings.Join(commands[2], " "); !strings.HasSuffix(got, "down -v") {
		t.Fatalf("reset command = %s", got)
	}
}

func TestObservabilityComposePinsImagesAndRoutesSignals(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", observabilityComposeFile))
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, required := range []string{
		"otel/opentelemetry-collector-contrib:0.157.0",
		"${DA_COLLECTOR_AGENT_IMAGE:-declarative-agents/agent-core:local}",
		"pull_policy: never",
		"prom/prometheus:v3.13.1",
		"exporters: [otlp_grpc/collector_agent]",
		"exporters: [prometheus_remote_write]",
		"collector-spool:/data",
		"prometheus-data:/prometheus",
	} {
		if !strings.Contains(content, required) {
			t.Errorf("compose file missing %q", required)
		}
	}
	if strings.Contains(content, ":latest") {
		t.Error("compose file contains an unpinned latest image")
	}
	if strings.Contains(content, "ghcr.io") {
		t.Error("compose file references a registry image; the collector agent must build locally")
	}
}

func TestObservabilityCollectorImageBuildRespectsOperatorOverride(t *testing.T) {
	var built []string
	restore := buildCollectorImage
	buildCollectorImage = func(coreRoot, image string) error {
		built = append(built, coreRoot+" "+image)
		return nil
	}
	defer func() { buildCollectorImage = restore }()

	t.Setenv(observabilityCollectorImageEnv, "example.com/operator/agent-core:pinned")
	if err := ensureCollectorAgentImage(); err != nil {
		t.Fatal(err)
	}
	if len(built) != 0 {
		t.Fatalf("operator-supplied image must not be built over, built %v", built)
	}

	t.Setenv(observabilityCollectorImageEnv, "")
	if err := ensureCollectorAgentImage(); err != nil {
		t.Fatal(err)
	}
	if len(built) != 1 || built[0] != "agent-core declarative-agents/agent-core:local" {
		t.Fatalf("default build = %v", built)
	}
}

func TestObservabilityPortsAreConfigurable(t *testing.T) {
	t.Setenv("DA_OTEL_GRPC_PORT", "24317")
	t.Setenv("DA_PROMETHEUS_QUERY_PORT", "29090")
	ports := observabilityPorts()
	if ports[0].value != "24317" || ports[4].value != "29090" {
		t.Fatalf("ports = %#v", ports)
	}
}

func restoreObservabilityHooks(t *testing.T) {
	t.Helper()
	command := runObservabilityCommand
	output := observabilityOutput
	health := checkObservability
	port := checkObservabilityPort
	build := buildCollectorImage
	t.Cleanup(func() {
		runObservabilityCommand = command
		observabilityOutput = output
		checkObservability = health
		checkObservabilityPort = port
		buildCollectorImage = build
	})
	buildCollectorImage = func(string, string) error { return nil }
}
