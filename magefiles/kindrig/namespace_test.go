// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeCluster records host commands and scripts failures by command prefix.
// Namespaces listed in present answer `kubectl get namespace` successfully.
type fakeCluster struct {
	calls   []string
	present map[string]bool
	fail    map[string]string
}

func (f *fakeCluster) run(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	for prefix, output := range f.fail {
		if strings.HasPrefix(call, prefix) {
			return []byte(output), errors.New("command failed")
		}
	}
	if strings.HasPrefix(call, "kubectl get namespace ") {
		if f.present[strings.TrimPrefix(call, "kubectl get namespace ")] {
			return nil, nil
		}
		return nil, errors.New("not found")
	}
	if strings.HasPrefix(call, "kubectl create namespace ") {
		if f.present == nil {
			f.present = make(map[string]bool)
		}
		f.present[strings.TrimPrefix(call, "kubectl create namespace ")] = true
	}
	if strings.HasPrefix(call, "kubectl delete namespace ") {
		delete(f.present, strings.Fields(strings.TrimPrefix(call, "kubectl delete namespace "))[0])
	}
	return nil, nil
}

func releaseCalls(namespace, release string) []string {
	return []string{
		"helm uninstall " + release + " --namespace " + namespace + " --ignore-not-found",
		"kubectl delete pod --all --namespace " + namespace +
			" --ignore-not-found=true --wait=true --timeout=60s",
		"kubectl delete persistentvolumeclaim --all --namespace " + namespace +
			" --ignore-not-found=true --wait=true --timeout=60s",
		"kubectl delete namespace " + namespace + " --ignore-not-found=true --wait=false",
		"kubectl wait --for=delete namespace/" + namespace + " --timeout=180s",
		"kubectl get namespace " + namespace,
		"kubectl -n kube-system wait --for=condition=Ready pod -l k8s-app=kube-proxy --timeout=120s",
		"kubectl -n kube-system rollout status deployment/coredns --timeout=120s",
		"kubectl get --raw=/readyz",
	}
}

func TestScenarioClassDefaultsToNamespaced(t *testing.T) {
	var class ScenarioClass
	if class != NamespacedScenario || class.String() != "namespaced" {
		t.Fatalf("zero ScenarioClass = %v, want namespaced", class)
	}
	if ClusterMutatingScenario.String() != "cluster-mutating" {
		t.Fatalf("ClusterMutatingScenario = %v", ClusterMutatingScenario)
	}
}

func TestScenarioNamespaceNameRejectsInvalidLabels(t *testing.T) {
	if name, err := ScenarioNamespaceName("helm-smoke"); err != nil || name != "da-helm-smoke" {
		t.Fatalf("ScenarioNamespaceName(helm-smoke) = %q, %v", name, err)
	}
	for _, scenario := range []string{"", "Helm", "helm_smoke", "-smoke", "smoke-", strings.Repeat("a", 61)} {
		if _, err := ScenarioNamespaceName(scenario); err == nil {
			t.Errorf("ScenarioNamespaceName(%q) accepted an invalid namespace", scenario)
		}
	}
}

func TestPrepareScenarioNamespaceCreatesSelectsThenReleasesInOrder(t *testing.T) {
	cluster := &fakeCluster{}
	namespace, err := PrepareScenarioNamespace(cluster.run, "helm-smoke", "smoke")
	if err != nil {
		t.Fatal(err)
	}
	if namespace.Name != "da-helm-smoke" || namespace.HelmRelease != "smoke" {
		t.Fatalf("namespace = %+v", namespace)
	}
	if err := namespace.Release(); err != nil {
		t.Fatal(err)
	}
	want := append([]string{
		"kubectl get namespace da-helm-smoke",
		"kubectl create namespace da-helm-smoke",
		"kubectl config set-context --current --namespace da-helm-smoke",
	}, releaseCalls("da-helm-smoke", "smoke")...)
	if !reflect.DeepEqual(cluster.calls, want) {
		t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(cluster.calls, "\n"), strings.Join(want, "\n"))
	}
}

func TestPrepareScenarioNamespaceTearsDownLeftoverFirst(t *testing.T) {
	cluster := &fakeCluster{present: map[string]bool{"da-helm-swap": true}}
	namespace, err := PrepareScenarioNamespace(cluster.run, "helm-swap", "swap")
	if err != nil {
		t.Fatal(err)
	}
	want := append([]string{"kubectl get namespace da-helm-swap"},
		releaseCalls("da-helm-swap", "swap")...)
	want = append(want,
		"kubectl create namespace da-helm-swap",
		"kubectl config set-context --current --namespace da-helm-swap")
	if !reflect.DeepEqual(cluster.calls, want) {
		t.Fatalf("calls =\n%s\nwant\n%s", strings.Join(cluster.calls, "\n"), strings.Join(want, "\n"))
	}
	if namespace.Name != "da-helm-swap" {
		t.Fatalf("namespace = %q", namespace.Name)
	}
}

func TestPrepareScenarioNamespaceFailsWhenLeftoverCannotBeRemoved(t *testing.T) {
	cluster := &fakeCluster{
		present: map[string]bool{"da-helm-swap": true},
		fail:    map[string]string{"kubectl delete namespace": "finalizer stuck"},
	}
	_, err := PrepareScenarioNamespace(cluster.run, "helm-swap", "swap")
	if err == nil || !strings.Contains(err.Error(), "leftover") ||
		!strings.Contains(err.Error(), "finalizer stuck") {
		t.Fatalf("error = %v, want leftover removal failure", err)
	}
	for _, call := range cluster.calls {
		if strings.HasPrefix(call, "kubectl create namespace") {
			t.Fatal("created a namespace over an unremoved leftover")
		}
	}
}

func TestPrepareScenarioNamespaceDeletesNamespaceWhenSelectFails(t *testing.T) {
	cluster := &fakeCluster{fail: map[string]string{"kubectl config set-context": "no context"}}
	_, err := PrepareScenarioNamespace(cluster.run, "applier-live", "live")
	if err == nil || !strings.Contains(err.Error(), "select scenario namespace da-applier-live") {
		t.Fatalf("error = %v", err)
	}
	last := cluster.calls[len(cluster.calls)-1]
	if last != "kubectl delete namespace da-applier-live --ignore-not-found=true --wait=true --timeout=60s" {
		t.Fatalf("select failure did not delete the created namespace; last call %q", last)
	}
}

func TestPrepareScenarioNamespaceRequiresRunnerAndRelease(t *testing.T) {
	if _, err := PrepareScenarioNamespace(nil, "smoke", "smoke"); err == nil {
		t.Fatal("nil runner accepted")
	}
	cluster := &fakeCluster{}
	if _, err := PrepareScenarioNamespace(cluster.run, "smoke", " "); err == nil {
		t.Fatal("empty release accepted")
	}
	if _, err := PrepareScenarioNamespace(cluster.run, "Bad_Name", "smoke"); err == nil {
		t.Fatal("invalid scenario accepted")
	}
	if len(cluster.calls) != 0 {
		t.Fatalf("validation failures issued commands: %v", cluster.calls)
	}
}

func TestScenarioNamespaceReleaseContinuesAfterEachFailure(t *testing.T) {
	cluster := &fakeCluster{fail: map[string]string{
		"helm uninstall":                       "release busy",
		"kubectl delete persistentvolumeclaim": "pvc protected",
	}}
	namespace, err := PrepareScenarioNamespace(cluster.run, "helm-swap", "swap")
	if err != nil {
		t.Fatal(err)
	}
	cluster.calls = nil
	err = namespace.Release()
	if err == nil || !strings.Contains(err.Error(), "release busy") ||
		!strings.Contains(err.Error(), "pvc protected") {
		t.Fatalf("release error = %v, want both failures joined", err)
	}
	if !reflect.DeepEqual(cluster.calls, releaseCalls("da-helm-swap", "swap")) {
		t.Fatalf("release skipped steps after a failure: %v", cluster.calls)
	}
}

func TestScenarioNamespaceReleaseReportsResidueAndDataPlaneFailure(t *testing.T) {
	cluster := &fakeCluster{fail: map[string]string{
		"kubectl delete namespace":              "stuck",
		"kubectl -n kube-system rollout status": "zero ready replicas",
	}}
	namespace, err := PrepareScenarioNamespace(cluster.run, "helm-llm", "llm")
	if err != nil {
		t.Fatal(err)
	}
	err = namespace.Release()
	for _, want := range []string{
		"scenario namespace da-helm-llm remains after cleanup",
		"shared kind data-plane readiness",
		"deployment/coredns",
		"zero ready replicas",
	} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("release error = %v, want %q", err, want)
		}
	}
}

func TestZeroScenarioNamespaceReleaseIsNoop(t *testing.T) {
	if err := (ScenarioNamespace{}).Release(); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyDataPlaneStopsAtFirstFailedCheck(t *testing.T) {
	cluster := &fakeCluster{fail: map[string]string{
		"kubectl -n kube-system wait": "kube-proxy not ready",
	}}
	err := VerifyDataPlane(cluster.run)
	if err == nil || !strings.Contains(err.Error(), "k8s-app=kube-proxy") {
		t.Fatalf("error = %v", err)
	}
	if len(cluster.calls) != 1 {
		t.Fatalf("checks after first failure ran: %v", cluster.calls)
	}
}
