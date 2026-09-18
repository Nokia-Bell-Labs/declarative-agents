// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ScenarioClass records whether a scenario can share a running cluster. The
// classification belongs to the scenario definition, not to a flag (GH-2215).
type ScenarioClass uint8

const (
	// NamespacedScenario is the default: the scenario installs as a Helm release
	// into its own namespace on a shared cluster and leaves nothing behind.
	NamespacedScenario ScenarioClass = iota
	// ClusterMutatingScenario needs cluster-scoped configuration (for example a
	// kind config with disableDefaultCNI) and keeps an owned cluster acquired
	// through EnsureFreshCluster.
	ClusterMutatingScenario
)

func (c ScenarioClass) String() string {
	switch c {
	case NamespacedScenario:
		return "namespaced"
	case ClusterMutatingScenario:
		return "cluster-mutating"
	default:
		return fmt.Sprintf("ScenarioClass(%d)", uint8(c))
	}
}

// ScenarioNamespacePrefix prefixes every scenario namespace on a shared cluster.
const ScenarioNamespacePrefix = "da-"

const (
	scenarioPodDeleteTimeout       = "60s"
	scenarioNamespaceDeleteTimeout = "180s"
	dataPlaneReadyTimeout          = "120s"
	scenarioWorkloadControllers    = "deployment,statefulset,daemonset,replicaset,job"
)

var scenarioLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// ScenarioNamespaceName returns the namespace a scenario owns on a shared
// cluster, rejecting names Kubernetes would refuse as a namespace.
func ScenarioNamespaceName(scenario string) (string, error) {
	name := ScenarioNamespacePrefix + scenario
	if !scenarioLabel.MatchString(scenario) || len(name) > 63 {
		return "", fmt.Errorf(
			"scenario %q does not form a valid namespace %q (lowercase DNS label, at most 63 characters)",
			scenario, name)
	}
	return name, nil
}

// ScenarioNamespace is one scenario's namespace on a shared cluster, carrying
// the command runner bound to that cluster's kubeconfig (Commands.Run).
type ScenarioNamespace struct {
	Name        string
	HelmRelease string
	run         CommandRunner
}

// PrepareScenarioNamespace creates the scenario's namespace on the cluster run
// is bound to and selects it as the kubeconfig's current namespace. A namespace
// left by an interrupted run is torn down first, as a leftover cluster is
// (GH-2137). The caller must invoke Release on every exit path.
func PrepareScenarioNamespace(
	run CommandRunner,
	scenario, helmRelease string,
) (ScenarioNamespace, error) {
	if run == nil {
		return ScenarioNamespace{}, errors.New("scenario namespace: command runner is required")
	}
	if strings.TrimSpace(helmRelease) == "" {
		return ScenarioNamespace{}, fmt.Errorf("scenario %s: Helm release name is required", scenario)
	}
	name, err := ScenarioNamespaceName(scenario)
	if err != nil {
		return ScenarioNamespace{}, err
	}
	namespace := ScenarioNamespace{Name: name, HelmRelease: helmRelease, run: run}
	if _, err := run("kubectl", "get", "namespace", name); err == nil {
		fmt.Printf("kind: tearing down leftover scenario namespace %s before reuse\n", name)
		if err := namespace.Release(); err != nil {
			return ScenarioNamespace{}, fmt.Errorf("remove leftover scenario namespace %s: %w", name, err)
		}
	}
	if output, err := run("kubectl", "create", "namespace", name); err != nil {
		return ScenarioNamespace{}, fmt.Errorf("create scenario namespace %s: %w: %s",
			name, err, output)
	}
	if output, err := run(
		"kubectl", "config", "set-context", "--current", "--namespace", name,
	); err != nil {
		_, _ = run("kubectl", "delete", "namespace", name,
			"--ignore-not-found=true", "--wait=true", "--timeout="+scenarioPodDeleteTimeout)
		return ScenarioNamespace{}, fmt.Errorf("select scenario namespace %s: %w: %s",
			name, err, output)
	}
	return namespace, nil
}

// Release uninstalls the scenario's Helm release, deletes its workload
// controllers, pods, claims, and namespace, confirms the namespace is gone, and rechecks the shared data plane
// so the next scenario starts on a ready cluster. Every step runs even after an
// earlier one fails; the joined error names each failure. A zero value is a
// no-op, so a failure path may release a namespace that was never prepared.
func (n ScenarioNamespace) Release() error {
	if n.run == nil {
		return nil
	}
	run, name := n.run, n.Name
	var cleanupErrors []error
	step := func(description string, args ...string) {
		if output, err := run(args[0], args[1:]...); err != nil {
			cleanupErrors = append(cleanupErrors,
				fmt.Errorf("%s: %w: %s", description, err, output))
		}
	}
	step(fmt.Sprintf("uninstall %s/%s", name, n.HelmRelease),
		"helm", "uninstall", n.HelmRelease, "--namespace", name, "--ignore-not-found")
	// Workloads applied outside the release, or left by a failed uninstall,
	// would recreate pods that keep claims bound.
	step(fmt.Sprintf("delete scenario namespace %s workload controllers", name),
		"kubectl", "delete", scenarioWorkloadControllers, "--all", "--namespace", name,
		"--ignore-not-found=true", "--wait=true", "--timeout="+scenarioPodDeleteTimeout)
	step(fmt.Sprintf("drain scenario namespace %s pods", name),
		"kubectl", "delete", "pod", "--all", "--namespace", name,
		"--ignore-not-found=true", "--wait=true", "--timeout="+scenarioPodDeleteTimeout)
	step(fmt.Sprintf("delete scenario namespace %s PVCs", name),
		"kubectl", "delete", "persistentvolumeclaim", "--all", "--namespace", name,
		"--ignore-not-found=true", "--wait=true", "--timeout="+scenarioPodDeleteTimeout)
	step(fmt.Sprintf("delete scenario namespace %s", name),
		"kubectl", "delete", "namespace", name, "--ignore-not-found=true", "--wait=false")
	step(fmt.Sprintf("wait for scenario namespace %s deletion", name),
		"kubectl", "wait", "--for=delete", "namespace/"+name,
		"--timeout="+scenarioNamespaceDeleteTimeout)
	if _, err := run("kubectl", "get", "namespace", name); err == nil {
		cleanupErrors = append(cleanupErrors,
			fmt.Errorf("scenario namespace %s remains after cleanup", name))
	}
	if err := VerifyDataPlane(run); err != nil {
		cleanupErrors = append(cleanupErrors, err)
	}
	return errors.Join(cleanupErrors...)
}

// VerifyDataPlane confirms a shared cluster can take the next scenario:
// kube-proxy pods are Ready, CoreDNS is rolled out, and the API server reports
// /readyz. It stops at the first failed check and names it.
func VerifyDataPlane(run CommandRunner) error {
	checks := [][]string{
		{"kubectl", "-n", "kube-system", "wait", "--for=condition=Ready",
			"pod", "-l", "k8s-app=kube-proxy", "--timeout=" + dataPlaneReadyTimeout},
		{"kubectl", "-n", "kube-system", "rollout", "status",
			"deployment/coredns", "--timeout=" + dataPlaneReadyTimeout},
		{"kubectl", "get", "--raw=/readyz"},
	}
	for _, command := range checks {
		if output, err := run(command[0], command[1:]...); err != nil {
			return fmt.Errorf("shared kind data-plane readiness %s: %w: %s",
				strings.Join(command, " "), err, output)
		}
	}
	return nil
}
