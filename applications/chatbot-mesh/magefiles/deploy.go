// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// The catalog variants that install and remove a release host-side (srd022 R6).
const (
	applierDeployProfileRel   = "agents/applier/deploy-profile.yaml"
	applierUndeployProfileRel = "agents/applier/undeploy-profile.yaml"
)

// chatbotDemoNamespace names the namespace the demo release installs into.
//
// The imperative path it replaces passed no --namespace at all, so the release
// landed in whatever the generated kubeconfig's context pointed at, which for a
// kind kubeconfig is default. Every deploy word carries --namespace, so the
// choice has to be written down; this is the one it was already making.
const chatbotDemoNamespace = "default"

// Deploy installs or upgrades the chatbot-mesh demo release by running the
// catalog applier's deploy machine against the demo cluster.
//
// It gates where mage diagnose reports: a failure terminal exits non-zero.
func Deploy() error {
	return deployStagedRelease(nil)
}

// Undeploy removes the chatbot-mesh demo release. A release that is already
// gone reaches Absent, so running this twice exits zero.
func Undeploy() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	agent, err := chatbotDeployAgent(root, applierUndeployProfileRel)
	if err != nil {
		return err
	}
	// An undeploy removes a release: it stages no chart, externalizes no UI
	// assets, and measures no budget, because none of that is needed to delete
	// something. kindrig owns what the unread coordinates render to.
	return kindrig.Undeploy(kindrig.DeployRequest{
		Cluster:         chatbotDemoCluster,
		ApplicationRoot: root,
		CatalogRoot:     agent.catalogRoot,
		Coordinates: kindrig.UndeployCoordinates(
			chatbotDemoRelease, chatbotDemoNamespace, helmLLMInstallTimeout.String()),
		Agent: agent.agent,
	})
}

// stagedRelease is everything one chatbot-mesh deploy needs that demo:up has
// already produced. A nil value means this run stages its own.
type stagedRelease struct {
	Staged       string
	ChartArchive string
	Image        string
	Assets       []externalUIAsset
}

// deployStagedRelease measures the projected release Secret, then runs the
// deploy machine over the staged chart.
//
// The budget gate has to run before anything reaches the cluster: without it an
// over-budget release arrives at the API server as an opaque Secret size error,
// after the cluster and the images are already built (GH-1475). The machine
// writes the values document as its first transition, which is too late to
// measure, so this writes the same document to the same workspace path first
// and measures that. The machine rewriting identical bytes costs nothing.
func deployStagedRelease(prepared *stagedRelease) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	release := prepared
	if release == nil {
		staged, err := stageReleaseForDeploy(root)
		if err != nil {
			return err
		}
		defer staged.cleanup()
		release = staged.release
	}

	agent, err := chatbotDeployAgent(root, applierDeployProfileRel)
	if err != nil {
		return err
	}
	overrides := chatbotDeployOverrides(release.Image, release.Assets)
	if err := writeDeployOverrides(root, overrides); err != nil {
		return err
	}
	measured, err := measureHelmReleaseBudget(
		chatbotDemoRelease, release.Staged, release.ChartArchive,
		chatbotBudgetValueArgs(root, release.Staged))
	if err != nil {
		return err
	}
	fmt.Printf("demo: release budget PASS - %s\n", measured.String())

	return kindrig.Deploy(kindrig.DeployRequest{
		Cluster:         chatbotDemoCluster,
		ApplicationRoot: root,
		CatalogRoot:     agent.catalogRoot,
		Coordinates:     chatbotDeployCoordinates(release.Staged),
		Overrides:       overrides,
		Agent:           agent.agent,
	})
}

// chatbotBudgetValueArgs are the value sources the budget render reads. They
// are the same two the deploy words carry, so the measurement sees the release
// the machine is about to apply rather than an approximation of it.
func chatbotBudgetValueArgs(root, staged string) []string {
	return []string{
		"--values", filepath.Join(staged, "ci", chatbotDemoValuesFile),
		"-f", kindrig.DeployOverridesPath(root, chatbotDemoRelease),
	}
}

// writeDeployOverrides puts the values document where the budget render can
// read it and where the machine writes it again.
func writeDeployOverrides(root, overrides string) error {
	path := kindrig.DeployOverridesPath(root, chatbotDemoRelease)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("deploy: create workspace %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(overrides), 0o644); err != nil {
		return fmt.Errorf("deploy: write overrides %s: %w", path, err)
	}
	return nil
}

// chatbotDeployCoordinates names this application's deploy coordinates. The
// harness fills Kubeconfig and OverridesPath.
func chatbotDeployCoordinates(staged string) kindrig.DeployCoordinates {
	return kindrig.DeployCoordinates{
		Release:    chatbotDemoRelease,
		Namespace:  chatbotDemoNamespace,
		ChartPath:  staged,
		ValuesPath: filepath.Join(staged, "ci", chatbotDemoValuesFile),
		Timeout:    helmLLMInstallTimeout.String(),
	}
}

// chatbotDeployOverrides is the values document the machine writes to its
// workspace and the apply words read with -f.
//
// The image tag and every UI archive checksum are quoted. A commit-shaped tag
// or a hex checksum decodes as a number when bare and stops matching what the
// chart expects, which is why the imperative path used --set-string for both.
func chatbotDeployOverrides(image string, assets []externalUIAsset) string {
	repository, tag := splitImageRef(image)
	var document strings.Builder
	fmt.Fprintf(&document, "image:\n  repository: %q\n  tag: %q\n  pullPolicy: %q\n",
		repository, tag, "Never")
	fmt.Fprintf(&document, "ingress:\n  enabled: true\n  className: %q\n  host: %q\n",
		chatbotDemoIngressClass, chatbotDemoHost)
	// The shipped UIs travel out of the release; the chart mounts them by the
	// ConfigMap each component names here (GH-1475).
	for _, asset := range assets {
		fmt.Fprintf(&document, "%s:\n  uiArchiveConfigMap: %q\n  uiArchiveChecksum: %q\n",
			asset.Component, asset.ConfigMapName, asset.Checksum)
	}
	return document.String()
}

// chatbotDeployAgent resolves the agent binary and the catalog profile one
// verb runs. A deploy that cannot resolve its agent has produced nothing, so
// every failure is returned rather than reported and stepped over.
type chatbotAgent struct {
	agent       kindrig.DeployAgent
	catalogRoot string
}

func chatbotDeployAgent(root, profileRel string) (chatbotAgent, error) {
	coreRoot := demoCoreRoot(root)
	if !agentCoreAvailable(coreRoot) {
		return chatbotAgent{}, fmt.Errorf("deploy: agent-core checkout not found at %s", coreRoot)
	}
	catalogRoot, err := resolveCatalogRoot("deploy", root)
	if err != nil {
		return chatbotAgent{}, fmt.Errorf("deploy: %w", err)
	}
	binary, err := buildAgent(coreRoot)
	if err != nil {
		return chatbotAgent{}, fmt.Errorf("deploy: %w", err)
	}
	return chatbotAgent{
		agent: kindrig.DeployAgent{
			Binary:   binary,
			Profile:  filepath.Join(catalogRoot, filepath.FromSlash(profileRel)),
			CoreRoot: coreRoot,
		},
		catalogRoot: catalogRoot,
	}, nil
}

// stagedForDeploy holds a staging this run owns and must release.
type stagedForDeploy struct {
	release *stagedRelease
	cleanup func()
}

// stageReleaseForDeploy prepares everything a standalone mage deploy needs:
// the staged chart, the externalized UI assets and their ConfigMaps, and the
// packaged archive the budget measurement reads.
//
// demo:up has already done all of this by the time it reaches the Helm step
// and hands its staging over instead, so the work happens once per run.
func stageReleaseForDeploy(root string) (stagedForDeploy, error) {
	chartDir := applicationChartDir(root)
	images, err := resolveChatbotIntegrationImages(root)
	if err != nil {
		return stagedForDeploy{}, err
	}
	staged, cleanupStaged, err := stageSmokeChart(chartDir, root)
	if err != nil {
		return stagedForDeploy{}, err
	}
	// The shipped UIs travel out of the release, as they do on every kind path.
	// Left inside, the release Secret projects past the 1 MiB Kubernetes limit
	// and the install dies creating it (GH-1475).
	assets, cleanupAssets, err := externalizeUIAssets(staged, chatbotDemoRelease)
	if err != nil {
		cleanupStaged()
		return stagedForDeploy{}, err
	}
	archive, cleanupArchive, err := packageApplierChart(staged)
	if err != nil {
		cleanupAssets()
		cleanupStaged()
		return stagedForDeploy{}, err
	}
	commands, cleanupCommands, err := kindrig.ClusterCommands(kindrig.CaptureRun, chatbotDemoCluster)
	if err != nil {
		cleanupArchive()
		cleanupAssets()
		cleanupStaged()
		return stagedForDeploy{}, fmt.Errorf("deploy: %w", err)
	}
	// The ConfigMaps live outside the release and the chart renders references
	// to them, so they exist before the apply rather than after it.
	provisionErr := provisionExternalUIAssets(commands.Run, assets)
	cleanupCommands()
	if provisionErr != nil {
		cleanupArchive()
		cleanupAssets()
		cleanupStaged()
		return stagedForDeploy{}, fmt.Errorf("deploy: provision external UI assets: %w", provisionErr)
	}
	return stagedForDeploy{
		release: &stagedRelease{
			Staged:       staged,
			ChartArchive: archive,
			Image:        images.Runtime,
			Assets:       assets,
		},
		cleanup: func() {
			cleanupArchive()
			cleanupAssets()
			cleanupStaged()
		},
	}, nil
}
