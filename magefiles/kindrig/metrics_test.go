// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"testing"
)

func TestInstallMetricsServerLoadsPinnedImageAndWaitsForAPI(t *testing.T) {
	var calls []string
	var applied string
	run := func(name string, args ...string) ([]byte, error) {
		command := strings.Join(append([]string{name}, args...), " ")
		calls = append(calls, command)
		if inspectFailsWithoutDigest(name, args) {
			return []byte("No such image"), errors.New("absent")
		}
		if strings.HasPrefix(command, "kubectl --context kind-da-example get apiservice") {
			return []byte("Error from server (NotFound): apiservices.apiregistration.k8s.io"), errors.New("NotFound")
		}
		if name == "kubectl" && appliedManifestPath(args) != "" {
			data, err := os.ReadFile(appliedManifestPath(args))
			if err != nil {
				return nil, err
			}
			applied = string(data)
		}
		return nil, nil
	}
	cleanup, err := InstallMetricsServer(run, "da-example")
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	source, err := metricsServerImage(runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	runtimeImage := metricsServerRuntimeRepository + ":" + metricsServerImageVersion
	want := []string{
		"kubectl --context kind-da-example get apiservice " + metricsAPIService,
		"docker image inspect --format {{.Id}} " + source,
		"docker image inspect --format {{.Id}} " + runtimeImage,
		"docker tag " + source + " " + runtimeImage,
		"node-import " + runtimeImage + " da-example-control-plane linux/" + runtime.GOARCH,
		"docker image rm " + runtimeImage,
		"kubectl --context kind-da-example apply -f ",
		"kubectl --context kind-da-example rollout status deployment/metrics-server --namespace kube-system --timeout=180s",
		"kubectl --context kind-da-example wait --for=condition=Available apiservice/" + metricsAPIService + " --timeout=180s",
		"kubectl --context kind-da-example delete -f ",
	}
	if len(calls) != len(want) {
		t.Fatalf("calls = %v, want %d", calls, len(want))
	}
	for index, expected := range want {
		if !strings.Contains(calls[index], expected) {
			t.Errorf("call[%d] = %q, want %q", index, calls[index], expected)
		}
	}
	for _, expected := range []string{
		"image: " + runtimeImage,
		"imagePullPolicy: Never",
		"--kubelet-insecure-tls",
		"app.kubernetes.io/managed-by: kindrig",
	} {
		if !strings.Contains(applied, expected) {
			t.Errorf("applied manifest missing %q", expected)
		}
	}
	if strings.Contains(applied, metricsServerImagePlaceholder) {
		t.Fatal("applied manifest retains the image placeholder")
	}
}

func TestInstallMetricsServerReusesHealthyAPI(t *testing.T) {
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(append([]string{name}, args...), " "))
		return []byte("True"), nil
	}
	cleanup, err := InstallMetricsServer(run, "da-example")
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 1 || !strings.Contains(calls[0], "kubectl --context kind-da-example get apiservice") {
		t.Fatalf("healthy reuse calls = %v", calls)
	}
}

func TestInstallMetricsServerRefusesUnhealthyExistingAPI(t *testing.T) {
	run := func(string, ...string) ([]byte, error) { return []byte("False"), nil }
	if _, err := InstallMetricsServer(run, "da-example"); err == nil ||
		!strings.Contains(err.Error(), "not healthy") {
		t.Fatalf("error = %v, want unhealthy-API refusal", err)
	}
}

func TestMetricsServerManifestConfinesKindOnlyTLSException(t *testing.T) {
	if count := strings.Count(metricsServerKindManifest, "--kubelet-insecure-tls"); count != 1 {
		t.Fatalf("insecure kubelet flag count = %d, want 1", count)
	}
	for _, forbidden := range []string{"imagePullPolicy: Always", "latest"} {
		if strings.Contains(metricsServerKindManifest, forbidden) {
			t.Errorf("test manifest contains %q", forbidden)
		}
	}
}

func TestInstallMetricsServerLive(t *testing.T) {
	cluster := os.Getenv("KINDRIG_METRICS_LIVE_CLUSTER")
	if cluster == "" {
		t.Skip("set KINDRIG_METRICS_LIVE_CLUSTER to an existing kind cluster")
	}
	cleanup, err := InstallMetricsServer(DefaultCommandRun, cluster)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cleanup() })
}

func TestInstallMetricsServerPullsOnlyAbsentImageAndCleansUpOnFailure(t *testing.T) {
	source, err := metricsServerImage(runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		call := strings.Join(append([]string{name}, args...), " ")
		calls = append(calls, call)
		switch {
		case strings.HasPrefix(call, "kubectl --context kind-da-example get apiservice"):
			return []byte("NotFound"), errors.New("NotFound")
		case strings.HasPrefix(call, "docker image inspect") && strings.Contains(call, source):
			return []byte("No such image"), errors.New("absent")
		case inspectFailsWithoutDigest(name, args):
			return []byte("No such image"), errors.New("absent")
		case strings.HasPrefix(call, "kubectl --context kind-da-example rollout status"):
			return []byte("timed out"), errors.New("rollout failed")
		}
		return nil, nil
	}
	if _, err := InstallMetricsServer(run, "da-example"); err == nil ||
		!strings.Contains(err.Error(), "rollout status") {
		t.Fatalf("error = %v, want the failed rollout", err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "docker pull --platform linux/"+runtime.GOARCH+" "+source) {
		t.Fatalf("absent image was not pulled: %v", calls)
	}
	if !strings.HasPrefix(calls[len(calls)-1], "kubectl --context kind-da-example delete -f ") {
		t.Fatalf("failed install did not delete its manifest: %v", calls)
	}
}
