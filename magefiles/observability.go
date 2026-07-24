// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/magefile/mage/mg"
)

const observabilityComposeFile = "deploy/observability/docker-compose.yml"

var (
	runObservabilityCommand = observabilityCommand
	observabilityOutput     = commandOutput
	checkObservability      = observabilityHealth
	checkObservabilityPort  = portAvailable
)

// Observability manages the persistent integration OTLP ingress, Jaeger trace
// backend, and Prometheus metric backend specified by srd008 R9.
type Observability mg.Namespace

// Up starts the shared stack or reuses an already healthy one.
func (Observability) Up() error {
	if checkObservability() == nil {
		fmt.Println("observability stack already healthy; reusing it")
		return nil
	}
	running, err := observabilityStackRunning()
	if err != nil {
		return err
	}
	if !running {
		for _, port := range observabilityPorts() {
			if err := checkObservabilityPort(port.name, port.value); err != nil {
				return err
			}
		}
	}
	if err := observabilityCompose("up", "-d", "--wait"); err != nil {
		return err
	}
	return checkObservability()
}

// Down removes stack containers and keeps retained backend volumes.
func (Observability) Down() error {
	return observabilityCompose("down")
}

// Reset removes stack containers and retained backend volumes.
func (Observability) Reset() error {
	return observabilityCompose("down", "-v")
}

// Status prints compose state and verifies every public health endpoint.
func (Observability) Status() error {
	if err := observabilityCompose("ps"); err != nil {
		return err
	}
	return checkObservability()
}

func observabilityCompose(args ...string) error {
	full := append([]string{"compose", "-f", observabilityComposeFile}, args...)
	fmt.Printf("+ docker %s\n", strings.Join(full, " "))
	return runObservabilityCommand("docker", full...)
}

func observabilityStackRunning() (bool, error) {
	out, err := observabilityOutput(
		"docker", "compose", "-f", observabilityComposeFile,
		"ps", "--services", "--filter", "status=running",
	)
	if err != nil {
		return false, fmt.Errorf("inspect observability stack: %w", err)
	}
	return strings.TrimSpace(out) != "", nil
}

type namedPort struct {
	name  string
	value string
}

func observabilityPorts() []namedPort {
	return []namedPort{
		{"OTLP gRPC", envOr("DA_OTEL_GRPC_PORT", "4317")},
		{"OTLP HTTP", envOr("DA_OTEL_HTTP_PORT", "4318")},
		{"Collector health", envOr("DA_OTEL_HEALTH_PORT", "13133")},
		{"Jaeger query", envOr("DA_JAEGER_QUERY_PORT", "16686")},
		{"Prometheus query", envOr("DA_PROMETHEUS_QUERY_PORT", "9090")},
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func portAvailable(name, port string) error {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err == nil {
		return listener.Close()
	}
	owner, _ := commandOutput("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN")
	if owner != "" {
		return fmt.Errorf("%s port %s is unavailable:\n%s", name, port, strings.TrimSpace(owner))
	}
	return fmt.Errorf("%s port %s is unavailable: %w", name, port, err)
}

func observabilityHealth() error {
	checks := []struct {
		name string
		url  string
	}{
		{"Collector", "http://127.0.0.1:" + envOr("DA_OTEL_HEALTH_PORT", "13133") + "/"},
		{"Jaeger", "http://127.0.0.1:" + envOr("DA_JAEGER_QUERY_PORT", "16686") + "/api/services"},
		{"Prometheus", "http://127.0.0.1:" + envOr("DA_PROMETHEUS_QUERY_PORT", "9090") + "/-/healthy"},
	}
	client := &http.Client{Timeout: 2 * time.Second}
	for _, check := range checks {
		response, err := client.Get(check.url)
		if err != nil {
			return fmt.Errorf("%s health %s: %w", check.name, check.url, err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return fmt.Errorf("%s health %s: HTTP %s", check.name, check.url, response.Status)
		}
	}
	return nil
}

func observabilityCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func commandOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
