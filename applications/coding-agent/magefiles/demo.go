// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	codingDemoCluster = "da-coding-agent-demo"
	codingDemoRelease = "demo"
)

type Demo mg.Namespace

// Doctor checks the shared ENG01 toolchain and host resources without mutation.
func Doctor() error {
	return kindrig.Doctor()
}

// Up creates or reuses the persistent demo cluster and deploys coding-agent.
func (Demo) Up() error {
	if err := Doctor(); err != nil {
		return fmt.Errorf("demo requested but preflight failed: %w", err)
	}
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return err
	}
	image, err := resolveCodingHelmImage(roots.Application)
	if err != nil {
		return err
	}
	config := filepath.Join(roots.Application, "helm", "ci", "kind-demo-config.yaml")
	return kindrig.DemoUp(kindrig.DefaultRun, codingDemoCluster, config,
		120*time.Second, func(kindrig.Cluster) error {
			kubeconfig, cleanup, err := codingKindKubeconfig(codingDemoCluster)
			if err != nil {
				return err
			}
			defer cleanup()
			environment := codingSmokeEnvironment{kubeconfig: kubeconfig}
			if err := recreateCodingDemoNamespace(environment); err != nil {
				return err
			}
			if err := prepareCodingHelmCluster(
				environment, codingDemoCluster, roots, image); err != nil {
				return err
			}
			if err := Package(); err != nil {
				return err
			}
			// The Helm step runs through the catalog applier's deploy
			// machine, so day-0 here and day-2 in the cluster are the same
			// declared sequence (srd022 R6). The ingress and health checks
			// below are not the Helm step and stay imperative.
			if err := Deploy(); err != nil {
				return err
			}
			run := func(name string, args ...string) ([]byte, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				return environment.run(ctx, name, args...)
			}
			if err := kindrig.InstallIngress(run, codingDemoCluster); err != nil {
				return err
			}
			ingress, cleanupIngress, err := codingDemoIngress()
			if err != nil {
				return err
			}
			defer cleanupIngress()
			if output, err := run("kubectl", "apply", "-f", ingress); err != nil {
				return fmt.Errorf("apply coding demo ingress: %w: %s",
					err, strings.TrimSpace(string(output)))
			}
			for role, endpoint := range map[string]string{
				"planner":  "http://planner-health.coding.localhost/api/lifecycle/health",
				"executor": "http://executor.coding.localhost/api/lifecycle/health",
				"critic":   "http://critic.coding.localhost/api/lifecycle/health",
			} {
				if err := waitServingHTTP(endpoint, 30*time.Second); err != nil {
					return fmt.Errorf("%s demo ingress not ready: %w", role, err)
				}
			}
			fmt.Printf("demo: revision %s planner at http://planner.coding.localhost/; health at http://planner-health.coding.localhost/api/lifecycle/health, http://executor.coding.localhost/api/lifecycle/health, http://critic.coding.localhost/api/lifecycle/health\n",
				image.Revision)
			return nil
		})
}

// recreateCodingDemoNamespace gives each demo:up a fresh namespace, clearing a
// prior install and its workspace claim before the workspace is re-seeded.
func recreateCodingDemoNamespace(environment codingSmokeEnvironment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	_, _ = environment.run(ctx, "kubectl", "delete", "namespace", codingHelmNamespace,
		"--ignore-not-found=true", "--wait=true", "--timeout=30s")
	cancel()
	return runCodingSmokeCommand(environment, 30*time.Second,
		"kubectl", "create", "namespace", codingHelmNamespace)
}

// Down deletes only the coding-agent demo cluster.
func (Demo) Down() error {
	if err := Doctor(); err != nil {
		return fmt.Errorf("demo teardown requested but preflight failed: %w", err)
	}
	return kindrig.DemoDown(kindrig.DefaultRun, codingDemoCluster)
}

func codingDemoIngress() (string, func(), error) {
	dir, err := os.MkdirTemp("", "coding-agent-demo-ingress-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	path := filepath.Join(dir, "ingress.yaml")
	service := codingDemoRelease + "-coding-agent-"
	manifest := fmt.Sprintf(`apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: coding-agent-demo
  namespace: %s
spec:
  ingressClassName: traefik
  rules:
    - host: planner.coding.localhost
      http: {paths: [{path: /, pathType: Prefix, backend: {service: {name: %splanner, port: {name: request}}}}]}
    - host: planner-health.coding.localhost
      http: {paths: [{path: /, pathType: Prefix, backend: {service: {name: %splanner, port: {name: control}}}}]}
    - host: executor.coding.localhost
      http: {paths: [{path: /, pathType: Prefix, backend: {service: {name: %sexecutor, port: {name: control}}}}]}
    - host: critic.coding.localhost
      http: {paths: [{path: /, pathType: Prefix, backend: {service: {name: %scritic, port: {name: control}}}}]}
`, codingHelmNamespace, service, service, service, service)
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	return path, cleanup, nil
}
