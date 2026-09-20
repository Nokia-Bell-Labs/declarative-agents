// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
	"github.com/magefile/mage/mg"
)

const (
	demoCluster   = "da-agent-architecture-demo"
	demoRelease   = "demo"
	demoNamespace = "agent-architecture-demo"
)

// Demo groups the persistent kind demo cluster targets.
type Demo mg.Namespace

// Doctor checks the shared ENG01 toolchain and host resources without mutation.
func Doctor() error {
	return kindrig.Doctor()
}

// Up creates or reuses the persistent demo cluster and deploys the composition.
func (Demo) Up() error {
	if err := Doctor(); err != nil {
		return fmt.Errorf("demo requested but preflight failed: %w", err)
	}
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	config := filepath.Join(resolved.Application, "helm", "ci", "kind-demo-config.yaml")
	return kindrig.DemoUp(kindrig.DefaultRun, demoCluster, config, 120*time.Second, func(kindrig.Cluster) error {
		kubeconfig, cleanup, err := smokeKubeconfig(demoCluster)
		if err != nil {
			return err
		}
		defer cleanup()
		environment := smokeEnvironment{kubeconfig: kubeconfig}
		if err := deployDemo(environment, resolved); err != nil {
			return err
		}
		fmt.Printf("demo: curator control at deploy/%s-agent-architecture-curator:18082, documentation :18081; collector query :18193\n", demoRelease)
		fmt.Printf("demo: kubectl -n %s port-forward deploy/%s-agent-architecture-curator 18082:18082 18081:18081\n", demoNamespace, demoRelease)
		return nil
	})
}

// Down deletes only the agent-architecture demo cluster.
func (Demo) Down() error {
	if err := Doctor(); err != nil {
		return fmt.Errorf("demo teardown requested but preflight failed: %w", err)
	}
	return kindrig.DemoDown(kindrig.DefaultRun, demoCluster)
}

func deployDemo(environment smokeEnvironment, resolved roots) error {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	_, _ = environment.run(ctx, "kubectl", "delete", "namespace", demoNamespace,
		"--ignore-not-found=true", "--wait=true", "--timeout=30s")
	cancel()
	// Both workloads run one locally built agent-core image (GH-1368).
	if err := kindrig.BuildAgentCoreImage(resolved.Core, smokeCollectorImage); err != nil {
		return fmt.Errorf("build agent-core image: %w", err)
	}
	kindLoad := func(ctx context.Context, args ...string) ([]byte, error) {
		return smokeEnvironment{}.run(ctx, "kind", args...)
	}
	loadCtx, cancelLoad := context.WithTimeout(context.Background(), smokeClusterTimeout)
	loadErr := kindrig.LoadImage(loadCtx, kindLoad, demoCluster, smokeCollectorImage)
	cancelLoad()
	if loadErr != nil {
		return loadErr
	}
	if err := runSmokeCommand(environment, 30*time.Second, "kubectl", "create", "namespace", demoNamespace); err != nil {
		return err
	}
	// The Helm step, the chart packaging it needs, and the curator UI shard
	// provisioning it depends on all run through the deploy machine, so day-0
	// here and day-2 in the cluster are the same declared sequence (srd022 R6).
	// The cluster, the namespace reset above, and the image load stay
	// imperative; none of them is the Helm step.
	if err := Deploy(); err != nil {
		return err
	}
	return nil
}
