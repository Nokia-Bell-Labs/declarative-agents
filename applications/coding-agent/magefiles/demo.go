// Copyright (c) 2026 Nokia. All rights reserved.

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
	images, err := resolveCodingHelmImages(roots.Application)
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
			if err := prepareCodingHelmCluster(
				environment, codingDemoCluster, roots, images); err != nil {
				return err
			}
			if err := Package(); err != nil {
				return err
			}
			destination, err := os.MkdirTemp("", "coding-agent-demo-chart-*")
			if err != nil {
				return err
			}
			defer func() { _ = os.RemoveAll(destination) }()
			archive, err := packageHelmChart(
				filepath.Join(roots.Application, "helm"),
				filepath.Join(roots.Application, filepath.FromSlash(defaultProfileOutput)),
				destination)
			if err != nil {
				return err
			}
			repository, tag := splitCodingImageRef(images.Agent)
			collectorRepository, collectorTag := splitCodingImageRef(codingHelmCollectorImage)
			ctx, cancel := context.WithTimeout(context.Background(), codingHelmInstallTimeout)
			output, err := environment.run(ctx, "helm",
				"upgrade", "--install", codingDemoRelease, archive,
				"--namespace", codingHelmNamespace,
				"--values", filepath.Join(roots.Application, "helm", "ci", "kind-values.yaml"),
				"--set", "image.repository="+repository,
				"--set-string", "image.tag="+tag,
				"--set", "collector.image.repository="+collectorRepository,
				"--set-string", "collector.image.tag="+collectorTag,
				"--wait", "--timeout", codingHelmInstallTimeout.String())
			cancel()
			if err != nil {
				return fmt.Errorf("helm demo install: %w: %s",
					err, strings.TrimSpace(string(output)))
			}
			run := func(name string, args ...string) ([]byte, error) {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
				defer cancel()
				return environment.run(ctx, name, args...)
			}
			if err := kindrig.InstallIngress(run); err != nil {
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
			fmt.Printf("demo: revision %s planner at http://planner.coding.localhost/; health at http://planner-health.coding.localhost/api/lifecycle/health, http://executor.coding.localhost/api/lifecycle/health, http://critic.coding.localhost/api/lifecycle/health\n",
				images.Revision)
			return nil
		})
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
  ingressClassName: nginx
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
