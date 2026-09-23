// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package kindrig

import (
	"fmt"
	"strings"
)

// Health states a platform or application component reports. They are coarse
// on purpose: the report says what each surface is, separately, and never
// rolls them into one verdict (#2477 R11).
type HealthState string

const (
	HealthOK       HealthState = "ok"
	HealthDegraded HealthState = "degraded"
)

// ComponentHealth is one surface's read-only state and a human detail.
type ComponentHealth struct {
	Name   string
	State  HealthState
	Detail string
}

// PlatformHealthReport separates the shared platform's own health from the
// health of the applications running on it, so a caller can tell a platform
// failure (every application is affected) from one application's failure (the
// platform and its peers are fine). This is the distinction R11 requires
// (#2477 R11): PlatformHealthy ignores application state entirely.
type PlatformHealthReport struct {
	Platform     []ComponentHealth
	Applications []ComponentHealth
}

// PlatformHealthy reports whether every shared platform surface is ok. A
// degraded application never makes it false.
func (r PlatformHealthReport) PlatformHealthy() bool {
	for _, component := range r.Platform {
		if component.State != HealthOK {
			return false
		}
	}
	return true
}

// PlatformHealth reads the shared platform surfaces and the managed application
// namespaces through the bound runner. It mutates nothing; each check maps a
// read-only kubectl query to a coarse state (#2477 R11).
func PlatformHealth(run CommandRunner) PlatformHealthReport {
	report := PlatformHealthReport{
		Platform: []ComponentHealth{
			dataPlaneHealth(run),
			deploymentHealth(run, "object-store", fakeGCSDeployment, fakeGCSNamespace),
			deploymentHealth(run, "ingress", "traefik", "traefik"),
		},
	}
	report.Applications = applicationHealth(run)
	return report
}

func dataPlaneHealth(run CommandRunner) ComponentHealth {
	if err := VerifyDataPlane(run); err != nil {
		return ComponentHealth{Name: "data-plane", State: HealthDegraded, Detail: err.Error()}
	}
	return ComponentHealth{Name: "data-plane", State: HealthOK}
}

// deploymentHealth reads one deployment's Available condition. A read error or
// any status other than True is degraded, never a panic or an assumed pass.
func deploymentHealth(run CommandRunner, label, name, namespace string) ComponentHealth {
	out, err := run("kubectl", "get", "deployment", name, "--namespace", namespace,
		"-o", `jsonpath={.status.conditions[?(@.type=="Available")].status}`)
	status := strings.TrimSpace(string(out))
	if err != nil {
		return ComponentHealth{Name: label, State: HealthDegraded, Detail: strings.TrimSpace(string(out))}
	}
	if status != "True" {
		return ComponentHealth{Name: label, State: HealthDegraded, Detail: "deployment is not Available"}
	}
	return ComponentHealth{Name: label, State: HealthOK}
}

// applicationHealth reports one entry per managed application namespace, each
// degraded when a workload in it is not Available. These are kept apart from
// the platform surfaces so an application's failure is attributed to the
// application (#2477 R11).
func applicationHealth(run CommandRunner) []ComponentHealth {
	namespaces, err := managedApplicationNamespaces(run)
	if err != nil {
		return []ComponentHealth{{Name: "applications", State: HealthDegraded, Detail: err.Error()}}
	}
	var health []ComponentHealth
	for _, namespace := range namespaces {
		health = append(health, namespaceWorkloadHealth(run, namespace))
	}
	return health
}

func namespaceWorkloadHealth(run CommandRunner, namespace string) ComponentHealth {
	out, err := run("kubectl", "get", "deployments", "--namespace", namespace,
		"-o", `jsonpath={range .items[*]}{.metadata.name}={.status.conditions[?(@.type=="Available")].status} {end}`)
	if err != nil {
		return ComponentHealth{Name: namespace, State: HealthDegraded, Detail: strings.TrimSpace(string(out))}
	}
	for _, pair := range strings.Fields(string(out)) {
		if parts := strings.SplitN(pair, "=", 2); len(parts) == 2 && parts[1] != "True" {
			return ComponentHealth{
				Name: namespace, State: HealthDegraded,
				Detail: fmt.Sprintf("workload %s is not Available", parts[0]),
			}
		}
	}
	return ComponentHealth{Name: namespace, State: HealthOK}
}
