// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/gcprig"
	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// gcpValuesFile is the one values file that makes the unmodified chart
// install on GKE (eng08): registry image paths and the donor mirror pin.
const gcpValuesFile = "gcp-values.yaml"

// GcpDeploy installs or upgrades the chatbot-mesh release on the GKE
// cluster gcp.yaml names, through the same applier deploy machine every
// kind deploy runs (srd022 R6, eng08). It pushes the current checkout's
// agent-core image to the project registry first, stages the same release
// demo:up stages, and hands the machine a kubeconfig written for this run
// alone, so no ambient context is involved.
func GcpDeploy() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	repoRoot := filepath.Join(root, "..", "..")
	config, err := gcprig.Load(repoRoot)
	if err != nil {
		return err
	}
	if err := gcprig.Preflight(gcprig.DefaultRun, config); err != nil {
		return err
	}

	images, err := resolveChatbotIntegrationImages(root)
	if err != nil {
		return err
	}
	// Build before pushing, the way demo:up builds before loading. Without
	// this the deploy only works on a commit whose image the operator
	// happened to build by hand, and fails with a docker daemon error
	// rather than a sentence (GH-2439).
	// ...and for the cluster's architecture, not the workstation's: an
	// arm64 host pushing its own build leaves every amd64 node reporting
	// "no match for platform in manifest" (GH-2457).
	if err := buildRuntimeImageForPlatform(
		demoCoreRoot(root), images.Runtime, config.NodePlatform); err != nil {
		return err
	}
	pushed, err := gcprig.PushAgentCore(gcprig.DefaultRun, config, images.Runtime, images.Revision)
	if err != nil {
		return err
	}

	// The kubeconfig comes first: staging provisions the external UI
	// ConfigMaps, and on this path there is no kind cluster to ask for a
	// credential (GH-2451).
	kubeconfigDir, err := os.MkdirTemp("", "gcp-deploy-kubeconfig-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(kubeconfigDir) }()
	kubeconfig := filepath.Join(kubeconfigDir, "config")
	if err := gcprig.WriteKubeconfig(config, kubeconfig); err != nil {
		return err
	}

	staged, err := stageReleaseForDeploy(root, deployCluster{Kubeconfig: kubeconfig})
	if err != nil {
		return err
	}
	defer staged.cleanup()
	release := staged.release

	agent, err := chatbotDeployAgent(root, applierDeployProfileRel)
	if err != nil {
		return err
	}
	overrides := gcpDeployOverrides(pushed, config, release.Assets)
	if err := writeDeployOverrides(root, overrides); err != nil {
		return err
	}

	coordinates := chatbotDeployCoordinates(release.Staged)
	coordinates.ValuesPath = filepath.Join(release.Staged, "ci", gcpValuesFile)
	fmt.Printf("gcp:deploy: %s -> cluster %s (%s)\n", pushed, config.Cluster, config.Region)
	return kindrig.Deploy(kindrig.DeployRequest{
		Cluster:         config.Cluster,
		KubeconfigPath:  kubeconfig,
		ApplicationRoot: root,
		CatalogRoot:     agent.catalogRoot,
		Coordinates:     coordinates,
		Overrides:       overrides,
		Agent:           agent.agent,
	})
}

// gcpDeployOverrides is the decided values document for a GKE deploy: the
// pushed registry image everywhere the checked-in overlay carries its
// placeholder path, the donor mirror, and the same externalized UI assets
// the kind deploy names. Pull policy is IfNotPresent: GKE pulls from the
// project registry, where kind loads and never pulls.
func gcpDeployOverrides(pushedImage string, config gcprig.Config, assets []externalUIAsset) string {
	repository, tag := splitImageRef(pushedImage)
	var document strings.Builder
	fmt.Fprintf(&document, "image:\n  repository: %q\n  tag: %q\n  pullPolicy: %q\n",
		repository, tag, "IfNotPresent")
	fmt.Fprintf(&document, "collector:\n  image:\n    repository: %q\n    tag: %q\n    pullPolicy: %q\n",
		repository, tag, "IfNotPresent")
	// One applier block carries the runtime image and the donor together;
	// two top-level applier keys would be a duplicate-key YAML document.
	fmt.Fprintf(&document,
		"applier:\n  enabled: true\n  image:\n    repository: %q\n    tag: %q\n    pullPolicy: %q\n",
		repository, tag, "IfNotPresent")
	// The donor pulls from the project mirror rather than Docker Hub, and
	// keeps the overlay's digest: gcp:mirrorDonor copies the manifest list
	// byte-identically, so the mirror's digest is the upstream one the
	// overlay already carries (GH-2437). Only the repository moves.
	fmt.Fprintf(&document,
		"  cliDonor:\n    image:\n      repository: %q\n      tag: %q\n      pullPolicy: %q\n",
		config.RegistryPath()+"/cli-donor", "1.31.4", "IfNotPresent")
	for _, asset := range assets {
		fmt.Fprintf(&document, "%s:\n  uiArchiveConfigMap: %q\n  uiArchiveChecksum: %q\n",
			asset.Component, asset.ConfigMapName, asset.Checksum)
	}
	return document.String()
}
