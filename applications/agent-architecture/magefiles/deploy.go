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
)

// The catalog variants that install and remove a release host-side (srd022 R6).
const (
	applierDeployProfileRel   = "agents/applier/deploy-profile.yaml"
	applierUndeployProfileRel = "agents/applier/undeploy-profile.yaml"
)

// Deploy installs or upgrades the agent-architecture demo release by running
// the catalog applier's deploy machine against the demo cluster.
//
// It gates where mage diagnose reports: a failure terminal exits non-zero,
// because a release that did not come up is not something to build on.
func Deploy() error {
	request, shards, err := deployRequest()
	if err != nil {
		return err
	}
	request.Overrides = deployOverrides(smokeCollectorImage, shards)
	return kindrig.Deploy(request)
}

// Undeploy removes the agent-architecture demo release. A release that is
// already gone reaches Absent, so running this twice exits zero.
func Undeploy() error {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return err
	}
	binary, cleanup, err := buildApplierBinary(resolved.Core)
	if err != nil {
		return fmt.Errorf("undeploy: %w", err)
	}
	// An undeploy removes a release. It packages no chart and provisions no
	// shard ConfigMaps, because creating objects on the way to deleting a
	// release is how a teardown leaves more behind than it found.
	return kindrig.Undeploy(kindrig.DeployRequest{
		Cluster:         demoCluster,
		ApplicationRoot: resolved.Application,
		CatalogRoot:     resolved.Catalog,
		Coordinates: kindrig.UndeployCoordinates(
			demoRelease, demoNamespace, smokeInstallTimeout.String()),
		Agent: kindrig.DeployAgent{
			Binary:   binary,
			Profile:  filepath.Join(resolved.Catalog, filepath.FromSlash(applierUndeployProfileRel)),
			CoreRoot: resolved.Core,
			Cleanup:  cleanup,
		},
	})
}

// deployRequest resolves everything one deploy needs, and reports the curator
// UI shards it provisioned so the caller can name them in the overrides.
//
// A deploy that cannot resolve its agent has produced nothing, so every
// failure here is returned rather than reported and stepped over.
func deployRequest() (kindrig.DeployRequest, []string, error) {
	resolved, err := resolveRootsFromWorkingDirectory()
	if err != nil {
		return kindrig.DeployRequest{}, nil, err
	}
	chart, err := deployChart(resolved)
	if err != nil {
		return kindrig.DeployRequest{}, nil, err
	}
	shards, err := provisionShards(resolved)
	if err != nil {
		return kindrig.DeployRequest{}, nil, err
	}

	binary, cleanup, err := buildApplierBinary(resolved.Core)
	if err != nil {
		return kindrig.DeployRequest{}, nil, fmt.Errorf("deploy: %w", err)
	}
	return kindrig.DeployRequest{
		Cluster:         demoCluster,
		ApplicationRoot: resolved.Application,
		CatalogRoot:     resolved.Catalog,
		Coordinates:     deployCoordinates(resolved, chart),
		Agent: kindrig.DeployAgent{
			Binary:   binary,
			Profile:  filepath.Join(resolved.Catalog, filepath.FromSlash(applierDeployProfileRel)),
			CoreRoot: resolved.Core,
			Cleanup:  cleanup,
		},
	}, shards, nil
}

// provisionShards creates the curator UI shard ConfigMaps, which live outside
// the release and must exist before the chart renders references to them
// (GH-1402).
//
// It owns the namespace rather than assuming one. demo:up recreates the
// namespace for a clean demo before it gets here, but mage deploy is a verb of
// its own and a ConfigMap cannot be created into a namespace that is not there.
// helm --create-namespace is too late: it runs after this.
func provisionShards(resolved roots) ([]string, error) {
	kubeconfig, release, err := smokeKubeconfig(demoCluster)
	if err != nil {
		return nil, fmt.Errorf("deploy: %w", err)
	}
	defer release()
	environment := smokeEnvironment{kubeconfig: kubeconfig}
	if err := ensureDemoNamespace(environment); err != nil {
		return nil, err
	}
	// provisionCuratorUIShards creates rather than applies, so it fails against
	// ConfigMaps a previous deploy left behind. demo:up used to hide that by
	// recreating the namespace first; mage deploy is its own verb and has to be
	// repeatable on a namespace that is already populated.
	if err := clearCuratorUIShards(environment); err != nil {
		return nil, err
	}
	shards, err := provisionCuratorUIShards(environment, resolved.Catalog, demoNamespace, demoRelease)
	if err != nil {
		return nil, fmt.Errorf("deploy: provision curator UI shards: %w", err)
	}
	return shards, nil
}

// clearCuratorUIShards removes the shard ConfigMaps a previous deploy left, so
// provisioning can create them again. The names are deterministic, so the
// prefix identifies exactly this release's shards and nothing else.
func clearCuratorUIShards(environment smokeEnvironment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	listed, err := environment.run(ctx, "kubectl", "get", "configmap", "-n", demoNamespace, "-o", "name")
	if err != nil {
		// A namespace with nothing in it is not an error worth failing on; the
		// create that follows reports anything that actually blocks it.
		return nil
	}
	prefix := "configmap/" + demoRelease + curatorUIShardInfix
	var stale []string
	for _, line := range strings.Split(string(listed), "\n") {
		if name := strings.TrimSpace(line); strings.HasPrefix(name, prefix) {
			stale = append(stale, strings.TrimPrefix(name, "configmap/"))
		}
	}
	if len(stale) == 0 {
		return nil
	}
	deleteCtx, cancelDelete := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelDelete()
	args := append([]string{"delete", "configmap", "-n", demoNamespace, "--ignore-not-found=true"}, stale...)
	if output, err := environment.run(deleteCtx, "kubectl", args...); err != nil {
		return fmt.Errorf("deploy: clear curator UI shards: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

// curatorUIShardInfix is the fixed middle of a shard ConfigMap name, between
// the release prefix and the part number that provisionCuratorUIShards appends.
const curatorUIShardInfix = "-curator-ui-"

// ensureDemoNamespace creates the demo namespace when it is absent and accepts
// one that is already there, so deploy is repeatable.
func ensureDemoNamespace(environment smokeEnvironment) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := environment.run(ctx, "kubectl", "get", "namespace", demoNamespace); err == nil {
		return nil
	}
	createCtx, cancelCreate := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelCreate()
	if output, err := environment.run(createCtx, "kubectl", "create", "namespace", demoNamespace); err != nil {
		return fmt.Errorf("deploy: create namespace %s: %w: %s",
			demoNamespace, err, strings.TrimSpace(string(output)))
	}
	return nil
}

// deployCoordinates names this application's deploy coordinates. The harness
// fills Kubeconfig from the cluster and OverridesPath from the workspace it
// owns, so those two stay empty here.
func deployCoordinates(resolved roots, chart string) kindrig.DeployCoordinates {
	return kindrig.DeployCoordinates{
		Release:    demoRelease,
		Namespace:  demoNamespace,
		ChartPath:  chart,
		ValuesPath: filepath.Join(resolved.Application, "helm", "ci", "kind-values.yaml"),
		Timeout:    smokeInstallTimeout.String(),
	}
}

// deployChart packages the chart into the render tree, so a failed deploy
// leaves the chart that produced it beside the argv that ran.
func deployChart(resolved roots) (string, error) {
	destination := filepath.Join(
		kindrig.DeployRenderDirectory(resolved.Application, demoRelease), "chart")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", fmt.Errorf("deploy: create chart directory %s: %w", destination, err)
	}
	chart, err := packageHelmChart(
		filepath.Join(resolved.Application, "helm"), resolved.Catalog, destination)
	if err != nil {
		return "", fmt.Errorf("deploy: package chart: %w", err)
	}
	return chart, nil
}

// deployOverrides is the values document the machine writes to its workspace
// and the apply words read with -f.
//
// Both workloads run the one locally built agent-core image, which is why the
// same repository and tag appear twice (GH-1368). The tags are quoted: a
// commit-shaped tag such as 20260919 decodes as an integer when bare and the
// image reference stops resolving, which is what --set-string prevented here
// before the migration.
func deployOverrides(image string, shards []string) string {
	repository, tag := splitImageRef(image)
	var document strings.Builder
	fmt.Fprintf(&document, "image:\n  repository: %q\n  tag: %q\n", repository, tag)
	fmt.Fprintf(&document, "collector:\n  image:\n    repository: %q\n    tag: %q\n", repository, tag)
	// curatorUI.shards was a repeated --set curatorUI.shards[N]=name; the same
	// list, written as a list.
	if len(shards) == 0 {
		document.WriteString("curatorUI:\n  shards: []\n")
		return document.String()
	}
	document.WriteString("curatorUI:\n  shards:\n")
	for _, shard := range shards {
		fmt.Fprintf(&document, "    - %q\n", shard)
	}
	return document.String()
}
