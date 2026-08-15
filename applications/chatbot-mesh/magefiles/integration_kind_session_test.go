// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

func TestIntegrationKindSessionOwnsOneAggregateClusterLifecycle(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	var calls []string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return nil, nil
	}
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()

	if !adoptAggregateKindCluster(
		kindrig.Cluster{Name: aggregateKindCluster, Created: true},
		run,
		kindrig.FailureEvidence{Directory: t.TempDir()},
	) {
		t.Fatal("active aggregate did not adopt its created cluster")
	}
	if !adoptAggregateKindCluster(
		kindrig.Cluster{Name: aggregateKindCluster},
		run,
		kindrig.FailureEvidence{},
	) {
		t.Fatal("same aggregate cluster was not recognized")
	}
	session.close()

	want := []string{"delete cluster --name " + aggregateKindCluster}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("kind calls = %v, want %v", calls, want)
	}
}

func TestIntegrationKindSessionLeavesDeveloperClusterUntouched(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	called := false
	run := func(...string) ([]byte, error) {
		called = true
		return nil, nil
	}
	session.cluster = kindrig.Cluster{Name: aggregateKindCluster}
	session.kindRun = run
	session.close()
	if called {
		t.Fatal("session mutated a developer-owned cluster")
	}
}

func TestIntegrationKindSessionCapturesEvidenceBeforeOwnedDelete(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	var calls []string
	run := func(args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return nil, nil
	}
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	evidenceDir := filepath.Join(t.TempDir(), "evidence")
	wantErr := errors.New("scenario failed")
	err = session.runTarget("helmSmoke", func() error {
		if !adoptAggregateKindCluster(
			kindrig.Cluster{Name: aggregateKindCluster, Created: true},
			run,
			kindrig.FailureEvidence{Directory: evidenceDir},
		) {
			t.Fatal("cluster adoption failed")
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("target error = %v, want %v", err, wantErr)
	}
	if len(calls) != 2 ||
		!strings.HasPrefix(calls[0], "export logs ") ||
		calls[1] != "delete cluster --name "+aggregateKindCluster {
		t.Fatalf("evidence/delete ordering = %v", calls)
	}
}

func TestIntegrationKindSessionPoisonBlocksLaterSharedTargets(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	wantErr := errors.New("contaminated")
	if err := session.runTarget("helmSmoke", func() error { return wantErr }); !errors.Is(err, wantErr) {
		t.Fatalf("first error = %v, want %v", err, wantErr)
	}
	called := false
	err := session.runTarget("helmSwap", func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "poisoned") {
		t.Fatalf("later target error = %v, want poisoned session", err)
	}
	if called {
		t.Fatal("later shared target ran after session poison")
	}
}

func TestIntegrationTargetRosterKeepsPolicyProofOutsideSharedSession(t *testing.T) {
	var shared []string
	for _, target := range integrationTargets(Integration{}) {
		if target.sharedKind {
			shared = append(shared, target.name)
		}
		if target.name == "policyProof" && target.sharedKind {
			t.Fatal("policy-proof entered the default-CNI shared session")
		}
	}
	want := []string{"helmSmoke", "helmSwap", "helmLLMTier", "applierLive"}
	if !reflect.DeepEqual(shared, want) {
		t.Fatalf("shared targets = %v, want %v", shared, want)
	}
}

func TestDirectIntegrationTargetHasNoSharedSession(t *testing.T) {
	if activeIntegrationKindSession() != nil {
		t.Fatal("test started with leaked aggregate session")
	}
	if adoptAggregateKindCluster(
		kindrig.Cluster{Name: aggregateKindCluster, Created: true},
		kindrig.DefaultRun,
		kindrig.FailureEvidence{},
	) {
		t.Fatal("direct target transferred ownership without an aggregate")
	}
}

func TestPrepareAggregateNamespaceCreatesSelectsAndCleansOnlyOwnedNamespace(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if call == "kubectl get namespace da-helm-smoke" {
			return nil, errors.New("not found")
		}
		return nil, nil
	}
	namespace, cleanup, err := prepareAggregateNamespace(run, "helm-smoke", "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if namespace != "da-helm-smoke" {
		t.Fatalf("namespace = %q, want da-helm-smoke", namespace)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"kubectl create namespace da-helm-smoke",
		"kubectl config set-context --current --namespace da-helm-smoke",
		"helm uninstall smoke --namespace da-helm-smoke --ignore-not-found",
		"kubectl delete pod --all --namespace da-helm-smoke --ignore-not-found=true --wait=true --timeout=60s",
		"kubectl delete persistentvolumeclaim --all --namespace da-helm-smoke --ignore-not-found=true --wait=true --timeout=60s",
		"kubectl delete namespace da-helm-smoke --ignore-not-found=true --wait=false",
		"kubectl wait --for=delete namespace/da-helm-smoke --timeout=180s",
		"kubectl get namespace da-helm-smoke",
		"kubectl -n kube-system wait --for=condition=Ready pod -l k8s-app=kube-proxy --timeout=120s",
		"kubectl -n kube-system rollout status deployment/coredns --timeout=120s",
		"kubectl get --raw=/readyz",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("namespace calls = %v, want %v", calls, want)
	}
}

func TestPrepareAggregateNamespaceContinuesCleanupAfterUninstallFailure(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		switch {
		case strings.HasPrefix(call, "helm uninstall"):
			return []byte("release busy"), errors.New("uninstall failed")
		case strings.HasPrefix(call, "kubectl get namespace"):
			return nil, errors.New("not found")
		default:
			return nil, nil
		}
	}
	_, cleanup, err := prepareAggregateNamespace(run, "helm-swap", "swap")
	if err != nil {
		t.Fatal(err)
	}
	err = cleanup()
	if err == nil || !strings.Contains(err.Error(), "release busy") {
		t.Fatalf("cleanup error = %v, want uninstall diagnostics", err)
	}
	if !strings.Contains(strings.Join(calls, "\n"), "kubectl delete namespace da-helm-swap") {
		t.Fatalf("namespace delete did not run after uninstall failure: %v", calls)
	}
}

func TestPrepareAggregateNamespaceIsNoopForDirectTarget(t *testing.T) {
	called := false
	namespace, cleanup, err := prepareAggregateNamespace(
		func(string, ...string) ([]byte, error) {
			called = true
			return nil, nil
		},
		"helm-smoke", "smoke",
	)
	if err != nil || namespace != "default" {
		t.Fatalf("direct namespace = %q err=%v, want default/nil", namespace, err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("direct target invoked aggregate namespace commands")
	}
}

func TestApplierLiveUsesOwnedAggregateClusterAndIsolatedNamespace(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	var calls []string
	run := func(name string, args ...string) ([]byte, error) {
		call := name + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if call == "kubectl get namespace da-applier-live" {
			return nil, errors.New("not found")
		}
		return nil, nil
	}
	if cluster := aggregateClusterName(applierLiveCluster); cluster != aggregateKindCluster {
		t.Fatalf("applierLive cluster = %q, want aggregate %q", cluster, aggregateKindCluster)
	}
	if !adoptAggregateKindCluster(
		kindrig.Cluster{Name: aggregateKindCluster, Created: true},
		func(args ...string) ([]byte, error) { return run("kind", args...) },
		kindrig.FailureEvidence{Directory: t.TempDir()},
	) {
		t.Fatal("applierLive aggregate cluster was not adopted")
	}
	if !aggregateKindClusterOwned(aggregateKindCluster) {
		t.Fatal("session did not retain aggregate ownership for applierLive diagnostics")
	}
	namespace, cleanup, err := prepareAggregateNamespace(
		run, "applier-live", applierLiveRelease)
	if err != nil {
		t.Fatal(err)
	}
	if namespace != "da-applier-live" {
		t.Fatalf("namespace = %q, want da-applier-live", namespace)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	for _, want := range []string{
		"kubectl create namespace da-applier-live",
		"helm uninstall live --namespace da-applier-live --ignore-not-found",
		"kubectl delete namespace da-applier-live --ignore-not-found=true --wait=false",
		"kubectl wait --for=delete namespace/da-applier-live --timeout=180s",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("applierLive lifecycle missing %q:\n%s", want, joined)
		}
	}
}

func TestPreparedHostImageReuseRequiresTheRecordedDigest(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	imageID := "sha256:" + strings.Repeat("a", 64)
	inspectCalls := 0
	run := func(name string, args ...string) ([]byte, error) {
		if name != "docker" ||
			strings.Join(args, " ") != "image inspect --format={{.Id}} busybox:1.36" {
			t.Fatalf("unexpected image inspection: %s %s", name, strings.Join(args, " "))
		}
		inspectCalls++
		return []byte(imageID + "\n"), nil
	}
	if reused, err := reusePreparedHostImage(run, "busybox:1.36"); err != nil || reused {
		t.Fatalf("unrecorded image reused=%v err=%v, want false/nil", reused, err)
	}
	if inspectCalls != 0 {
		t.Fatalf("unrecorded image was inspected %d times, want zero", inspectCalls)
	}
	if err := recordPreparedHostImage(run, "busybox:1.36"); err != nil {
		t.Fatal(err)
	}
	if reused, err := reusePreparedHostImage(run, "busybox:1.36"); err != nil || !reused {
		t.Fatalf("recorded image reused=%v err=%v, want true/nil", reused, err)
	}
	imageID = "sha256:" + strings.Repeat("b", 64)
	if reused, err := reusePreparedHostImage(run, "busybox:1.36"); err != nil || reused {
		t.Fatalf("retagged image reused=%v err=%v, want false/nil", reused, err)
	}
}

func TestNormalizedDockerImageReferenceMatchesContainerdNames(t *testing.T) {
	tests := map[string]string{
		"busybox:1.36":                            "docker.io/library/busybox:1.36",
		"otel/opentelemetry-collector:0.127.0":    "docker.io/otel/opentelemetry-collector:0.127.0",
		"ghcr.io/example/runtime:latest":          "ghcr.io/example/runtime:latest",
		"localhost:5000/example/runtime@sha256:a": "localhost:5000/example/runtime@sha256:a",
	}
	for input, want := range tests {
		if got := normalizedDockerImageReference(input); got != want {
			t.Errorf("normalizedDockerImageReference(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAggregateDataPlaneFailureNamesReadinessBoundary(t *testing.T) {
	session := newIntegrationKindSession(t.TempDir())
	deactivate, err := activateIntegrationKindSession(session)
	if err != nil {
		t.Fatal(err)
	}
	defer deactivate()
	err = verifyAggregateDataPlane(func(name string, args ...string) ([]byte, error) {
		command := name + " " + strings.Join(args, " ")
		if strings.Contains(command, "deployment/coredns") {
			return []byte("zero ready replicas"), errors.New("rollout timed out")
		}
		return nil, nil
	})
	if err == nil ||
		!strings.Contains(err.Error(), "shared kind data-plane readiness") ||
		!strings.Contains(err.Error(), "deployment/coredns") ||
		!strings.Contains(err.Error(), "zero ready replicas") {
		t.Fatalf("readiness error = %v", err)
	}
}

func TestCollectorOTLPReadinessWaitsForAcceptingEndpoint(t *testing.T) {
	stopped := false
	attempts := 0
	err := assertCollectorOTLPReadyWith(
		24317,
		func(port int) (func(), error) {
			if port != 24317 {
				t.Fatalf("forward port = %d, want 24317", port)
			}
			return func() { stopped = true }, nil
		},
		func(address string, _ time.Duration) error {
			if address != "127.0.0.1:24317" {
				t.Fatalf("probe address = %q", address)
			}
			attempts++
			if attempts < 3 {
				return errors.New("connection refused")
			}
			return nil
		},
		time.Second,
	)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 3 || !stopped {
		t.Fatalf("attempts=%d stopped=%v, want 3/true", attempts, stopped)
	}
}

func TestCollectorOTLPReadinessReportsForwardFailure(t *testing.T) {
	want := errors.New("forward failed")
	err := assertCollectorOTLPReadyWith(
		24317,
		func(int) (func(), error) { return nil, want },
		func(string, time.Duration) error { return nil },
		time.Second,
	)
	if !errors.Is(err, want) || !strings.Contains(err.Error(), "readiness forward") {
		t.Fatalf("error = %v, want wrapped forward failure", err)
	}
}
