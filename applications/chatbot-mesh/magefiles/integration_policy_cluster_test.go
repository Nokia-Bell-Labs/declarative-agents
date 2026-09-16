// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"errors"
	"strings"
	"testing"
)

// reusedPolicyCluster fakes kind reporting the policy cluster as listed, with a
// kubeconfig, so EnsureCluster takes the reuse path.
func reusedPolicyCluster(args ...string) ([]byte, error) {
	switch {
	case len(args) >= 2 && args[0] == "get" && args[1] == "clusters":
		return []byte(policyKindCluster + "\n"), nil
	case len(args) >= 2 && args[0] == "get" && args[1] == "kubeconfig":
		return []byte("apiVersion: v1\nkind: Config\n"), nil
	}
	return nil, nil
}

type policyCommandLog struct {
	calls   []string
	respond func(call string) ([]byte, error)
}

func (l *policyCommandLog) run(name string, args ...string) ([]byte, error) {
	call := strings.Join(append([]string{name}, args...), " ")
	l.calls = append(l.calls, call)
	if l.respond != nil {
		return l.respond(call)
	}
	return nil, nil
}

func (l *policyCommandLog) count(fragment string) int {
	n := 0
	for _, call := range l.calls {
		if strings.Contains(call, fragment) {
			n++
		}
	}
	return n
}

// TestReusedPolicyClusterWithoutCalicoIsBootstrapped is GH-2097: an API-healthy
// cluster whose Calico is gone must get the fresh-cluster bootstrap, not trust.
func TestReusedPolicyClusterWithoutCalicoIsBootstrapped(t *testing.T) {
	t.Chdir("..")
	log := &policyCommandLog{} // daemonset lookup prints nothing: absent

	cluster, err := ensurePolicyClusterWith(reusedPolicyCluster, log.run, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if cluster.Created {
		t.Fatalf("reused cluster reported as created: %+v", cluster)
	}
	if log.count("get daemonset calico-node") != 1 {
		t.Fatalf("Calico presence not checked: %v", log.calls)
	}
	if log.count("apply -f "+calicoManifest) != 1 {
		t.Fatalf("Calico not installed on a reused cluster without it: %v", log.calls)
	}
	_, probeImage, err := policyProbeImageRefs("amd64")
	if err != nil {
		t.Fatal(err)
	}
	if log.count("kind load docker-image "+probeImage) != 1 {
		t.Fatalf("probe image not loaded on bootstrap: %v", log.calls)
	}
}

func TestReusedPolicyClusterWithCalicoWaitsForReadyWithoutReinstalling(t *testing.T) {
	t.Chdir("..")
	log := &policyCommandLog{respond: func(call string) ([]byte, error) {
		if strings.Contains(call, "get daemonset calico-node") {
			return []byte("daemonset.apps/calico-node\n"), nil
		}
		return nil, nil
	}}

	if _, err := ensurePolicyClusterWith(reusedPolicyCluster, log.run, "amd64"); err != nil {
		t.Fatal(err)
	}
	if log.count("docker pull") != 0 || log.count("apply -f") != 0 {
		t.Fatalf("a reused cluster with Calico was bootstrapped again: %v", log.calls)
	}
	if log.count("wait --for=condition=Ready node --all --timeout="+policyReuseReadyTimeout) != 1 {
		t.Fatalf("reused cluster readiness not awaited with the reuse bound: %v", log.calls)
	}
}

func TestReusedPolicyClusterNotReadyNamesTheNodeCondition(t *testing.T) {
	t.Chdir("..")
	notReady := errors.New("timed out waiting for the condition")
	log := &policyCommandLog{respond: func(call string) ([]byte, error) {
		switch {
		case strings.Contains(call, "get daemonset calico-node"):
			return []byte("daemonset.apps/calico-node\n"), nil
		case strings.Contains(call, "rollout status daemonset/calico-node"):
			return nil, notReady
		case strings.Contains(call, "get nodes"):
			return []byte("da-chatbot-mesh-policy-control-plane Ready=False: " +
				"container runtime network not ready: cni plugin not initialized;"), nil
		}
		return nil, nil
	}}

	_, err := ensurePolicyClusterWith(reusedPolicyCluster, log.run, "amd64")
	if !errors.Is(err, notReady) {
		t.Fatalf("error = %v, want the readiness failure", err)
	}
	if !strings.Contains(err.Error(), "cni plugin not initialized") {
		t.Fatalf("error does not name the node condition: %v", err)
	}
}

func TestPolicyClusterCalicoCheckFailureIsReported(t *testing.T) {
	t.Chdir("..")
	unreachable := errors.New("connection refused")
	log := &policyCommandLog{respond: func(call string) ([]byte, error) {
		if strings.Contains(call, "get daemonset calico-node") {
			return []byte("the server could not be reached"), unreachable
		}
		return nil, nil
	}}

	_, err := ensurePolicyClusterWith(reusedPolicyCluster, log.run, "amd64")
	if !errors.Is(err, unreachable) {
		t.Fatalf("error = %v, want the check failure", err)
	}
	if log.count("apply -f") != 0 {
		t.Fatalf("bootstrapped after an inconclusive Calico check: %v", log.calls)
	}
}
