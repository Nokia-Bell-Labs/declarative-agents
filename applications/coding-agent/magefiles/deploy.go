// Copyright (c) 2026 Nokia
// SPDX-License-Identifier: BSD-3-Clause

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Nokia-Bell-Labs/declarative-agents/magefiles/kindrig"
)

// The catalog variants that install and remove a release host-side (srd022 R6).
const (
	applierDeployProfileRel   = "agents/applier/deploy-profile.yaml"
	applierUndeployProfileRel = "agents/applier/undeploy-profile.yaml"
)

// Deploy installs or upgrades the coding-agent demo release by running the
// catalog applier's deploy machine against the demo cluster.
//
// Unlike mage diagnose, which reports and never gates, this gates: a failure
// terminal exits non-zero, because a release that did not come up is not
// something to keep building on.
func Deploy() error {
	request, err := codingDeployRequest()
	if err != nil {
		return err
	}
	return kindrig.Deploy(request)
}

// Undeploy removes the coding-agent demo release. A release that is already
// gone reaches Absent, which is a success, so running this twice exits zero.
func Undeploy() error {
	request, err := codingDeployRequest()
	if err != nil {
		return err
	}
	request.Agent.Profile = filepath.Join(request.CatalogRoot, filepath.FromSlash(applierUndeployProfileRel))
	return kindrig.Undeploy(request)
}

// codingDeployRequest resolves everything one deploy needs. A deploy that
// cannot resolve its agent has produced nothing, so unlike chatbotRigDoctor,
// which reports and returns a zero agent, this returns the error.
func codingDeployRequest() (kindrig.DeployRequest, error) {
	roots, err := resolveIntegrationRoots()
	if err != nil {
		return kindrig.DeployRequest{}, err
	}
	images, err := resolveCodingHelmImages(roots.Application)
	if err != nil {
		return kindrig.DeployRequest{}, err
	}
	chart, err := codingDeployChart(roots)
	if err != nil {
		return kindrig.DeployRequest{}, err
	}
	binary, cleanup, err := buildAgent(roots.Core)
	if err != nil {
		return kindrig.DeployRequest{}, fmt.Errorf("deploy: %w", err)
	}
	return kindrig.DeployRequest{
		Cluster:         codingDemoCluster,
		ApplicationRoot: roots.Application,
		CatalogRoot:     roots.Profiles,
		Coordinates:     codingDeployCoordinates(roots, chart),
		Overrides:       codingDeployOverrides(images),
		Agent: kindrig.DeployAgent{
			Binary:   binary,
			Profile:  filepath.Join(roots.Profiles, filepath.FromSlash(applierDeployProfileRel)),
			CoreRoot: roots.Core,
			Cleanup:  cleanup,
		},
	}, nil
}

// codingDeployCoordinates names this application's deploy coordinates. The
// harness fills Kubeconfig from the cluster and OverridesPath from the
// workspace it owns, so those two stay empty here.
func codingDeployCoordinates(roots integrationRoots, chart string) kindrig.DeployCoordinates {
	return kindrig.DeployCoordinates{
		Release:    codingDemoRelease,
		Namespace:  codingHelmNamespace,
		ChartPath:  chart,
		ValuesPath: filepath.Join(roots.Application, "helm", "ci", "kind-values.yaml"),
		Timeout:    codingHelmInstallTimeout.String(),
	}
}

// codingDeployChart packages the chart into the build tree the render writes
// beside, so a failed deploy leaves the chart that produced it in place.
func codingDeployChart(roots integrationRoots) (string, error) {
	destination := filepath.Join(
		kindrig.DeployRenderDirectory(roots.Application, codingDemoRelease), "chart")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return "", fmt.Errorf("deploy: create chart directory %s: %w", destination, err)
	}
	chart, err := packageHelmChart(
		filepath.Join(roots.Application, "helm"),
		filepath.Join(roots.Application, filepath.FromSlash(defaultProfileOutput)),
		destination)
	if err != nil {
		return "", fmt.Errorf("deploy: package chart: %w", err)
	}
	return chart, nil
}

// codingDeployOverrides is the values document the machine writes to its
// workspace and the apply words read with -f.
//
// The image tags are quoted because a tag is a commit-shaped string that YAML
// would otherwise read as a number: an unquoted 20260919 becomes an integer and
// the image reference stops resolving. The imperative path this replaces used
// helm's --set-string for the same reason.
func codingDeployOverrides(images codingHelmImages) string {
	repository, tag := splitCodingImageRef(images.Agent)
	collectorRepository, collectorTag := splitCodingImageRef(codingHelmCollectorImage)
	return fmt.Sprintf(`image:
  repository: %q
  tag: %q
collector:
  image:
    repository: %q
    tag: %q
`, repository, tag, collectorRepository, collectorTag)
}
