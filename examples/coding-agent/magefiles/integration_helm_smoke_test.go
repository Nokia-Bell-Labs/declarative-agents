// Copyright (c) 2026 Nokia. All rights reserved.

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodingHelmSmokeSkipsMissingExternalPrerequisite(t *testing.T) {
	t.Setenv("PATH", "")
	reason := codingHelmSmokeSkipReason(integrationRoots{}, func(
		context.Context, string, ...string,
	) ([]byte, error) {
		t.Fatal("runner must not execute after missing binary")
		return nil, nil
	})
	if !strings.Contains(reason, "docker not found") {
		t.Fatalf("skip reason = %q", reason)
	}
}

func TestCodingHelmInfrastructureProbeClassification(t *testing.T) {
	healthy := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "kubectl" && strings.Contains(strings.Join(args, " "), "/readyz") {
			return []byte("ok\n"), nil
		}
		return []byte("healthy"), nil
	}
	if err := checkCodingHelmInfrastructure(healthy); err != nil {
		t.Fatalf("healthy infrastructure rejected: %v", err)
	}
	unavailable := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "kubectl" {
			return []byte("TLS handshake timeout"), fmt.Errorf("exit status 1")
		}
		return []byte("healthy"), nil
	}
	err := checkCodingHelmInfrastructure(unavailable)
	var infrastructure *codingHelmInfrastructureError
	if !errors.As(err, &infrastructure) ||
		infrastructure.Step != "host-to-Kubernetes API" {
		t.Fatalf("infrastructure error = %#v", err)
	}
}

func TestCodingHelmFailureDistinguishesSemanticFromInfrastructure(t *testing.T) {
	healthy := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "kubectl" && strings.Contains(strings.Join(args, " "), "/readyz") {
			return []byte("ok"), nil
		}
		return []byte("diagnostic"), nil
	}
	err := classifyCodingHelmFailure(healthy, "planner request", errors.New("HTTP 422"))
	var semantic *codingHelmSemanticError
	if !errors.As(err, &semantic) || semantic.Step != "planner request" ||
		!strings.Contains(semantic.Diagnostics, "helm status") {
		t.Fatalf("semantic error = %#v", err)
	}
	unhealthy := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "kubectl" && strings.Contains(strings.Join(args, " "), "/readyz") {
			return []byte("connection lost"), errors.New("timeout")
		}
		return []byte("diagnostic"), nil
	}
	err = classifyCodingHelmFailure(unhealthy, "planner request", errors.New("HTTP 500"))
	var infrastructure *codingHelmInfrastructureError
	if !errors.As(err, &infrastructure) ||
		!strings.Contains(infrastructure.Diagnostics, "events") {
		t.Fatalf("infrastructure reclassification = %#v", err)
	}
	preclassified := &codingHelmInfrastructureError{
		Step: "dependency image load", Cause: errors.New("content digest missing"),
	}
	err = classifyCodingHelmFailure(healthy, "cluster preparation", preclassified)
	if !errors.As(err, &infrastructure) ||
		infrastructure.Step != "dependency image load" {
		t.Fatalf("preclassified infrastructure error = %#v", err)
	}
}

func TestCodingHelmDiagnosticsUseOneBoundedContext(t *testing.T) {
	var calls int
	runner := func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		calls++
		if calls == 1 {
			<-ctx.Done()
		}
		return nil, ctx.Err()
	}
	started := time.Now()
	report := collectCodingHelmDiagnostics(runner, 50*time.Millisecond)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("diagnostics exceeded one bounded budget: %s", elapsed)
	}
	if calls < 2 || !strings.Contains(report, "diagnostic failed") {
		t.Fatalf("diagnostic report did not continue after timeout: calls=%d report=%s", calls, report)
	}
}

func TestCodingHelmFixturesMatchServingAndStorageContract(t *testing.T) {
	root := filepath.Join("..", "helm", "ci")
	values, err := os.ReadFile(filepath.Join(root, "kind-values.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"existingClaim: coding-agent-kind-workspace",
		"externalURL: http://coding-model:11434",
		"pullPolicy: Never",
	} {
		if !strings.Contains(string(values), want) {
			t.Errorf("kind values missing %q", want)
		}
	}
	workspace, err := os.ReadFile(filepath.Join(root, "kind-workspace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"accessModes: [ReadWriteMany]",
		"path: /tmp/coding-agent-workspace",
		"namespace: coding-agent-smoke",
	} {
		if !strings.Contains(string(workspace), want) {
			t.Errorf("kind workspace missing %q", want)
		}
	}
}

func TestCodingHelmSmokeUsesProductionProfileFreeImageRecipe(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	dockerfile := string(data)
	for _, want := range []string{
		"ARG GOLANGCI_LINT_VERSION=v2.12.2",
		"FROM ${GO_IMAGE} AS runtime",
		"github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION}",
		"COPY --from=builder /out/agent /usr/local/bin/agent",
		"COPY --from=builder /out/golangci-lint /usr/local/bin/golangci-lint",
		"COPY agent-core/tools /opt/agent-core/tools",
		"USER 10001:10001",
	} {
		if !strings.Contains(dockerfile, want) {
			t.Errorf("production Dockerfile missing %q", want)
		}
	}
	for _, forbidden := range []string{"COPY profiles", "COPY agents", "v1.64.8"} {
		if strings.Contains(dockerfile, forbidden) {
			t.Errorf("production Dockerfile violates its image contract via %q", forbidden)
		}
	}
}

func TestCodingHelmUsesIsolatedLocalJaegerPort(t *testing.T) {
	if codingHelmJaegerURL != "http://127.0.0.1:18686" {
		t.Fatalf("Jaeger URL = %s, want isolated smoke port", codingHelmJaegerURL)
	}
}
