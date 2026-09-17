// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"strings"
	"testing"
)

// leftoverPolicyCluster fakes kind listing a policy cluster left by an
// interrupted run and records every kind subcommand.
type leftoverPolicyCluster struct{ calls []string }

func (k *leftoverPolicyCluster) run(args ...string) ([]byte, error) {
	k.calls = append(k.calls, strings.Join(args, " "))
	if len(args) >= 2 && args[0] == "get" && args[1] == "clusters" {
		return []byte(policyKindCluster + "\n"), nil
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

// A leftover policy cluster is never trusted or inspected: it is deleted,
// recreated, owned, and bootstrapped like any fresh cluster (GH-2097, GH-2137).
func TestLeftoverPolicyClusterIsRecreatedAndBootstrapped(t *testing.T) {
	t.Chdir("..")
	kind := &leftoverPolicyCluster{}
	log := &policyCommandLog{}

	cluster, err := ensurePolicyClusterWith(kind.run, log.run, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if !cluster.Created {
		t.Fatalf("recreated cluster not owned: %+v", cluster)
	}
	if len(kind.calls) != 3 || kind.calls[1] != "delete cluster --name "+policyKindCluster ||
		!strings.HasPrefix(kind.calls[2], "create cluster --name "+policyKindCluster) {
		t.Fatalf("kind calls = %v, want list, delete leftover, create", kind.calls)
	}
	if log.count("apply -f "+calicoManifest) != 1 {
		t.Fatalf("Calico not installed on the recreated cluster: %v", log.calls)
	}
	_, probeImage, err := policyProbeImageRefs("amd64")
	if err != nil {
		t.Fatal(err)
	}
	if log.count("kind load docker-image "+probeImage) != 1 {
		t.Fatalf("probe image not loaded on bootstrap: %v", log.calls)
	}
}
