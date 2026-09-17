// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// ensurePolicyCluster creates the policy cluster fresh. It uses its own name
// rather than the smoke cluster so the CNI is the one this proof was written
// against. The cluster disables the default CNI, so an API-healthy leftover can
// still have no network (GH-2097); a leftover from an interrupted run is
// therefore deleted and recreated rather than inspected and adopted, and every
// run bootstraps Calico onto a cluster it owns (GH-2137).
func ensurePolicyCluster() (kindrig.Cluster, error) {
	return ensurePolicyClusterWith(
		kindrig.DefaultRun,
		kindrig.DefaultCommandRun,
		runtime.GOARCH,
	)
}

func ensurePolicyClusterWith(
	kindRun kindrig.Runner,
	commandRun kindrig.CommandRunner,
	arch string,
) (kindrig.Cluster, error) {
	// The node stays NotReady until a CNI lands, so a Ready wait here would always
	// time out (wait 0).
	cluster, err := kindrig.EnsureFreshCluster(kindRun, policyKindCluster, policyKindConfig, 0)
	if err != nil {
		return kindrig.Cluster{}, err
	}
	fmt.Printf("policyProof: created %s with the default CNI disabled\n", policyKindCluster)
	return cluster, bootstrapPolicyCluster(commandRun, cluster.Name, arch)
}

// bootstrapPolicyCluster installs Calico and loads the probe image: everything a
// cluster created with the default CNI disabled needs before pods can run.
func bootstrapPolicyCluster(run kindrig.CommandRunner, cluster, arch string) error {
	fmt.Printf("policyProof: installing Calico %s from locally loaded images\n", calicoVersion)
	if err := installCalico(run, cluster, arch); err != nil {
		return err
	}
	fmt.Printf("policyProof: loading the policy probe image into %s\n", cluster)
	return preloadPolicyProbeImage(run, cluster, arch)
}

// policyNodeReadiness renders each node's Ready condition message, the line that
// says why a node cannot schedule ("cni plugin not initialized"). A failure to
// read it is itself reported, never a reason to hide the original error.
func policyNodeReadiness(run kindrig.CommandRunner, cluster string) string {
	output, err := run("kubectl", "--context", "kind-"+cluster, "get", "nodes", "-o",
		`jsonpath={range .items[*]}{.metadata.name} Ready={.status.conditions[?(@.type=="Ready")].status}: {.status.conditions[?(@.type=="Ready")].message}{"; "}{end}`)
	detail := strings.TrimSuffix(strings.TrimSpace(string(output)), ";")
	if err != nil {
		return fmt.Sprintf("node readiness unavailable: %v: %s", err, detail)
	}
	return "nodes: " + detail
}
