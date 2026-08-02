// Copyright (c) 2026 Nokia. All rights reserved.

package kindrig

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCheckPreflightIsNonMutatingAndChecksResources(t *testing.T) {
	outputs := map[string]string{
		"docker version --format {{.Client.Version}}": "29.5.3",
		"kind version":                                 "kind v0.32.0 go1.26 darwin/arm64",
		"helm version --short":                         "v4.2.3+g123",
		"kubectl version --client=true -o json":        `{"clientVersion":{"gitVersion":"v1.35.0"}}`,
		"docker info --format {{.NCPU}} {{.MemTotal}}": "8 8589934592",
	}
	var calls []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		return []byte(outputs[call]), nil
	}
	report, err := CheckPreflight(context.Background(), run, "darwin")
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Tools) != len(StandardToolchain) ||
		report.CPUs != 8 || report.MemoryGiB != 8 {
		t.Fatalf("report = %+v", report)
	}
	for _, call := range calls {
		for _, mutating := range []string{"create", "delete", "apply", "install", "build"} {
			if strings.Contains(call, " "+mutating+" ") {
				t.Errorf("preflight issued mutating command %q", call)
			}
		}
	}
}

func TestCheckPreflightReportsMissingToolWithGuidance(t *testing.T) {
	run := func(_ context.Context, name string, _ ...string) ([]byte, error) {
		if name == "kind" {
			return nil, errors.New("executable file not found")
		}
		return []byte("v99.0.0"), nil
	}
	_, err := CheckPreflight(context.Background(), run, "linux")
	if err == nil || !strings.Contains(err.Error(), "kind") ||
		!strings.Contains(err.Error(), "https://") {
		t.Fatalf("missing-tool error = %v", err)
	}
}

func TestCheckPreflightRequiresDockerDaemonOnLinux(t *testing.T) {
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "docker" && len(args) > 0 && args[0] == "info" {
			return []byte("Cannot connect to the Docker daemon"), errors.New("exit status 1")
		}
		switch name {
		case "docker":
			return []byte("29.5.3"), nil
		case "kind":
			return []byte("kind v0.32.0"), nil
		case "helm":
			return []byte("v4.2.3"), nil
		case "kubectl":
			return []byte(`{"clientVersion":{"gitVersion":"v1.35.0"}}`), nil
		}
		return nil, errors.New("unexpected command")
	}
	_, err := CheckPreflight(context.Background(), run, "linux")
	if err == nil || !strings.Contains(err.Error(), "Docker daemon is unavailable") {
		t.Fatalf("daemon error = %v", err)
	}
}

func TestCheckPreflightRejectsOldVersionAndSmallDockerDesktop(t *testing.T) {
	t.Run("old version", func(t *testing.T) {
		run := func(_ context.Context, name string, _ ...string) ([]byte, error) {
			if name == "kind" {
				return []byte("kind v0.20.0"), nil
			}
			return []byte("v99.0.0"), nil
		}
		_, err := CheckPreflight(context.Background(), run, "linux")
		if err == nil || !strings.Contains(err.Error(), "below required") {
			t.Fatalf("old-version error = %v", err)
		}
	})
	t.Run("small Docker Desktop", func(t *testing.T) {
		run := healthyPreflightRunner("2 4294967296")
		_, err := CheckPreflight(context.Background(), run, "darwin")
		if err == nil || !strings.Contains(err.Error(), "below the required") {
			t.Fatalf("resource error = %v", err)
		}
	})
}

func healthyPreflightRunner(resources string) HostRunner {
	return func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name == "docker" && len(args) > 0 && args[0] == "info" {
			return []byte(resources), nil
		}
		switch name {
		case "docker":
			return []byte("29.5.3"), nil
		case "kind":
			return []byte("kind v0.32.0"), nil
		case "helm":
			return []byte("v4.2.3"), nil
		case "kubectl":
			return []byte(`{"clientVersion":{"gitVersion":"v1.35.0"}}`), nil
		}
		return nil, errors.New("unexpected command")
	}
}
