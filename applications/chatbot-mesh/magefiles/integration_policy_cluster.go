// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// policyReuseReadyTimeout bounds how long a reused cluster whose Calico is
// already installed may take to report Ready. Calico restarting after a Docker
// restart settles in well under this; a node that has not by then is broken.
const policyReuseReadyTimeout = "120s"

// ensurePolicyCluster reuses or creates the policy cluster. It uses its own name
// rather than the smoke cluster so the CNI is the one this proof was written
// against; reusing a cluster built with a different CNI would measure a different
// system. Ownership follows the same rule as the other targets -- only a cluster
// this run created may be deleted (GH-589).
//
// kindrig's reuse check is API health only, and this cluster disables the
// default CNI, so an API-healthy cluster can still have no network: a node that
// never schedules a pod (GH-2097). A reused cluster is therefore not taken on
// trust. Without Calico it gets the same bootstrap a fresh cluster gets; with
// Calico it must reach Ready within a bound. The self-test still runs after
// either, so a reused cluster that does not enforce is caught; one that enforces
// differently is not, which is why the printed notice names the risk.
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
	// time out (wait 0). Readiness is checked below, once Calico is known to exist.
	cluster, err := kindrig.EnsureClusterWithOptions(
		kindRun, policyKindCluster, policyKindConfig, 0,
		kindrig.EnsureOptions{HealthRun: commandRun})
	if err != nil {
		return kindrig.Cluster{}, err
	}
	if cluster.Created {
		fmt.Printf("policyProof: created %s with the default CNI disabled\n", policyKindCluster)
		return cluster, bootstrapPolicyCluster(commandRun, cluster.Name, arch)
	}

	fmt.Printf("kind: reusing pre-existing cluster %s; it will not be deleted. "+
		"If it was not created by this target its CNI may differ from %s\n",
		policyKindCluster, calicoManifest)
	installed, err := policyClusterHasCalico(commandRun, cluster.Name)
	if err != nil {
		return cluster, err
	}
	if !installed {
		fmt.Printf("policyProof: reused %s has no Calico, so its node cannot "+
			"schedule pods; bootstrapping it as a fresh cluster\n", cluster.Name)
		return cluster, bootstrapPolicyCluster(commandRun, cluster.Name, arch)
	}
	if err := waitCalicoReady(commandRun, cluster.Name, policyReuseReadyTimeout); err != nil {
		return cluster, fmt.Errorf("reused cluster %s is not ready: %w; %s",
			cluster.Name, err, policyNodeReadiness(commandRun, cluster.Name))
	}
	return cluster, nil
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

// policyClusterHasCalico reports whether the Calico node daemonset exists. It
// does not judge whether Calico works; waitCalicoReady does that.
func policyClusterHasCalico(run kindrig.CommandRunner, cluster string) (bool, error) {
	args := []string{"--context", "kind-" + cluster, "-n", "kube-system",
		"get", "daemonset", "calico-node", "--ignore-not-found", "-o", "name"}
	output, err := run("kubectl", args...)
	if err != nil {
		return false, fmt.Errorf("check Calico on %s: kubectl %s: %w: %s",
			cluster, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)) != "", nil
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
