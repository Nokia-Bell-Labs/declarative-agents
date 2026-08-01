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

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

const (
	observabilityComposeFile       = "observability/docker-compose.yml"
	observabilityCollectorImageEnv = "DA_COLLECTOR_AGENT_IMAGE"
)

var (
	runObservabilityCommand = observabilityCommand
	observabilityOutput     = observabilityCommandOutput
	checkObservability      = observabilityHealth
	checkObservabilityPort  = portAvailable
	buildCollectorImage     = kindrig.BuildAgentCoreImage
)

// Observability manages the persistent integration OTLP ingress, collector agent
// trace backend, and Prometheus metric backend the mesh integration gates
// require (srd008-telemetry R9). The stack outlives any one integration run;
// down keeps backend volumes and only reset deletes them.
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
	if err := ensureCollectorAgentImage(); err != nil {
		return err
	}
	if err := observabilityCompose("up", "-d", "--wait"); err != nil {
		return err
	}
	return checkObservability()
}

// ensureCollectorAgentImage builds the repo-local runtime image the compose
// file defaults to. The published ghcr.io image is not pullable from every
// environment, so the local tag is built from the agent-core checkout; an
// operator-supplied DA_COLLECTOR_AGENT_IMAGE names an existing image and is
// never built over.
func ensureCollectorAgentImage() error {
	if os.Getenv(observabilityCollectorImageEnv) != "" {
		return nil
	}
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve observability working directory: %w", err)
	}
	coreRoot := envOrDefault(agentCoreRootEnv, siblingPath(root, "agent-core"))
	fmt.Printf("+ building %s from %s\n", kindrig.DefaultAgentCoreImage, coreRoot)
	return buildCollectorImage(coreRoot, kindrig.DefaultAgentCoreImage)
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
		{"OTLP gRPC", envOrDefault("DA_OTEL_GRPC_PORT", "4317")},
		{"OTLP HTTP", envOrDefault("DA_OTEL_HTTP_PORT", "4318")},
		{"Collector health", envOrDefault("DA_OTEL_HEALTH_PORT", "13133")},
		{"Collector query", envOrDefault("DA_COLLECTOR_QUERY_PORT", "18193")},
		{"Prometheus query", envOrDefault("DA_PROMETHEUS_QUERY_PORT", "9090")},
	}
}

func portAvailable(name, port string) error {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err == nil {
		return listener.Close()
	}
	owner, _ := observabilityCommandOutput("lsof", "-nP", "-iTCP:"+port, "-sTCP:LISTEN")
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
		{"Collector", "http://127.0.0.1:" + envOrDefault("DA_OTEL_HEALTH_PORT", "13133") + "/"},
		{"Collector agent", "http://127.0.0.1:" + envOrDefault("DA_COLLECTOR_QUERY_PORT", "18193") + "/query/traces"},
		{"Prometheus", "http://127.0.0.1:" + envOrDefault("DA_PROMETHEUS_QUERY_PORT", "9090") + "/-/healthy"},
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

func observabilityCommandOutput(name string, args ...string) (string, error) {
	out, err := exec.Command(name, args...).CombinedOutput()
	return string(out), err
}
