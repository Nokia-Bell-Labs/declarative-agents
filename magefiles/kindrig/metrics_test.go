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
		if strings.HasPrefix(command, "kubectl get apiservice") {
			return []byte("Error from server (NotFound): apiservices.apiregistration.k8s.io"), errors.New("NotFound")
		}
		if name == "kubectl" && len(args) == 3 && args[0] == "apply" {
			data, err := os.ReadFile(args[2])
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
		"kubectl get apiservice " + metricsAPIService,
		"docker pull --platform linux/" + runtime.GOARCH + " " + source,
		"docker tag " + source + " " + runtimeImage,
		"kind load docker-image " + runtimeImage + " --name da-example",
		"kubectl apply -f ",
		"kubectl rollout status deployment/metrics-server --namespace kube-system --timeout=180s",
		"kubectl wait --for=condition=Available apiservice/" + metricsAPIService + " --timeout=180s",
		"kubectl delete -f ",
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
	if len(calls) != 1 || !strings.Contains(calls[0], "kubectl get apiservice") {
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
